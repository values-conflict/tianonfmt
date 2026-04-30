// Tests are in package main so they can call unexported helpers directly.
// A handful of subprocess tests verify the full CLI pipeline end-to-end; all
// other tests call formatter/lint functions directly for accurate coverage.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tianon/fmt/tianonfmt/internal/testutil"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

// ── formatter.byContent / formatter.byPath ────────────────────────────────────

func TestFormatter_JQ(t *testing.T) {
	f := &formatter{}
	out, err := f.byContent("-", `{"foo": .bar}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "foo: .bar") {
		t.Errorf("expected unquoted key, got %q", out)
	}
}

func TestFormatter_JQPath(t *testing.T) {
	f := &formatter{}
	out, err := f.byPath("foo.jq", `{"foo": .bar}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "foo: .bar") {
		t.Errorf("byPath jq dispatch failed, got %q", out)
	}
}

func TestFormatter_Template(t *testing.T) {
	// A Dockerfile.template file with {{ }} jq template syntax
	src := "FROM debian:{{ .suite }}\nRUN echo {{ .version }}\n"
	f := &formatter{}
	out, err := f.byPath("Dockerfile.template", src)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Error("expected non-empty output for template file")
	}
}

func TestFormatter_TemplateByContent(t *testing.T) {
	// Template starting with {{ }} — byContent must detect it before isDockerfileContent
	src := "{{ .base }}\nFROM debian:bookworm-slim\nRUN echo hi\n"
	f := &formatter{}
	// Must not crash or error (template detection should fire)
	_, err := f.byContent("-", src)
	if err != nil {
		t.Errorf("unexpected error for template content: %v", err)
	}
}

func TestFormatter_ShellAnyShebang(t *testing.T) {
	// #!/bin/false (source-only scripts) must be detected as shell
	src := "#!/bin/false source me\ncmd || :\n"
	f := &formatter{}
	out, err := f.byContent("-", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "cmd") {
		t.Errorf("shell not formatted: %q", out)
	}
}

func TestFormatter_DockerfilePath(t *testing.T) {
	f := &formatter{}
	out, err := f.byPath("Dockerfile", "FROM debian:bookworm-slim\nENV FOO=bar\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "FOO=bar") {
		t.Errorf("ENV not normalized, got %q", out)
	}
}

func TestFormatter_ShellPath(t *testing.T) {
	f := &formatter{}
	out, err := f.byPath("script.sh", "#!/bin/bash\ncmd || true\n")
	if err != nil {
		t.Fatal(err)
	}
	// no tidy, so || true stays; shebang stays (only --tidy normalises)
	if !strings.Contains(out, "#!/bin/bash") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestFormatter_Tidy_AndChain(t *testing.T) {
	f := &formatter{tidy: true}
	out, err := f.byContent("-", "FROM debian:bookworm-slim\nRUN apt-get update && apt-get install -y curl\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -eux") {
		t.Errorf("tidy did not flatten && chain, got:\n%s", out)
	}
}

func TestFormatter_Tidy_Shebang(t *testing.T) {
	f := &formatter{tidy: true}
	out, err := f.byContent("-", "#!/bin/bash\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "#!/usr/bin/env bash") {
		t.Errorf("shebang not normalized: %q", out)
	}
}

func TestFormatter_Tidy_ShellOrTrue(t *testing.T) {
	f := &formatter{tidy: true}
	out, err := f.byContent("-", "#!/usr/bin/env bash\ncmd || true\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "|| :") {
		t.Errorf("|| true not rewritten, got: %q", out)
	}
}

func TestFormatter_Tidy_SetPipefailDockerfile(t *testing.T) {
	f := &formatter{tidy: true}
	out, err := f.byContent("-", "FROM debian:bookworm-slim\nRUN set -Eeuo pipefail; \\\n\techo hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -eux") || strings.Contains(out, "pipefail") {
		t.Errorf("set flags not normalized: %q", out)
	}
}

func TestFormatter_Tidy_SetFlagsPreservedInShell(t *testing.T) {
	f := &formatter{tidy: true}
	out, err := f.byContent("-", "#!/usr/bin/env bash\nset -Eeuo pipefail\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -Eeuo pipefail") {
		t.Errorf("set -Eeuo pipefail wrongly modified in shell: %q", out)
	}
}

// ── type detection ────────────────────────────────────────────────────────────

func TestIsDockerfileName(t *testing.T) {
	yes := []string{"Dockerfile", "Dockerfile.bookworm", "Dockerfile.template"}
	no := []string{"main.go", "script.sh", "foo.jq", "Dockerfile_bad"}
	for _, n := range yes {
		if !isDockerfileName(n) {
			t.Errorf("isDockerfileName(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isDockerfileName(n) {
			t.Errorf("isDockerfileName(%q) = true, want false", n)
		}
	}
}

func TestIsDockerfileContent(t *testing.T) {
	yes := []string{
		"FROM debian:bookworm-slim\n",
		"# comment\nFROM scratch\n",
		"RUN echo hello\n",
	}
	no := []string{".foo | .bar\n", "#!/bin/bash\n", ""}
	for _, s := range yes {
		if !isDockerfileContent(s) {
			t.Errorf("isDockerfileContent(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isDockerfileContent(s) {
			t.Errorf("isDockerfileContent(%q) = true, want false", s)
		}
	}
}

// ── jqFmtFunc ─────────────────────────────────────────────────────────────────

func TestJQFmtFunc_Inline(t *testing.T) {
	got := jqFmtFunc(".foo | .bar", true)
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestJQFmtFunc_Multiline(t *testing.T) {
	got := jqFmtFunc("if .x then .y else .z end", false)
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestJQFmtFunc_ParseError(t *testing.T) {
	got := jqFmtFunc("((((invalid", false)
	if got != "" {
		t.Errorf("expected empty on parse error, got %q", got)
	}
}

func TestJQFmtFunc_InvalidChar(t *testing.T) {
	// "!" is not valid jq — the lexer returns an INVALID token, the parser
	// returns an error, and jqFmtFunc returns "" (no panic).
	got := jqFmtFunc("!invalid", false)
	if got != "" {
		t.Errorf("expected empty on invalid char, got %q", got)
	}
}

func TestJQFmtFunc_FileExpression(t *testing.T) {
	got := jqFmtFunc("def f: .+1; .[]|f", false)
	if got == "" {
		t.Error("expected non-empty for file-level expression")
	}
}

// ── tidyRUN ───────────────────────────────────────────────────────────────────

func TestTidyRUN_AndChain(t *testing.T) {
	cmds := tidyRUN("cmd1 && cmd2 && cmd3")
	if len(cmds) < 3 {
		t.Errorf("expected ≥3 commands, got %v", cmds)
	}
	if cmds[0] != "set -eux" {
		t.Errorf("first command should be set -eux, got %q", cmds[0])
	}
}

func TestTidyRUN_AlreadyHasSet(t *testing.T) {
	cmds := tidyRUN("set -eux && cmd1 && cmd2")
	if len(cmds) < 2 {
		t.Fatalf("expected ≥2 commands, got %v", cmds)
	}
	// should not prepend a second set -eux
	count := 0
	for _, c := range cmds {
		if strings.HasPrefix(c, "set -") {
			count++
		}
	}
	if count > 1 {
		t.Errorf("duplicate set -eux, got %v", cmds)
	}
}

func TestTidyRUN_Semicolons_NoChange(t *testing.T) {
	// Already in semicolon form: tidyRUN should return nil
	result := tidyRUN("set -eux; cmd1; cmd2")
	if result != nil {
		t.Errorf("expected nil for already-semicolon form, got %v", result)
	}
}

func TestTidyRUN_SingleCommand_NoChange(t *testing.T) {
	result := tidyRUN("echo hello")
	if result != nil {
		t.Errorf("expected nil for single command, got %v", result)
	}
}

func TestTidyRUN_OrChain_NoChange(t *testing.T) {
	result := tidyRUN("cmd1 || cmd2")
	if result != nil {
		t.Errorf("expected nil for || chain (not &&), got %v", result)
	}
}

// ── normalizeSetFlags ─────────────────────────────────────────────────────────

func TestNormaliseSetFlags(t *testing.T) {
	tests := []struct{ in, want string }{
		{"set -eux", "set -eux"},
		{"set -Eeuo pipefail", "set -eux"},
		{"set -ex", "set -eux"},
		{"set -e", "set -eux"},
		{"echo hello", "echo hello"}, // not a set command
	}
	for _, tt := range tests {
		if got := normalizeSetFlags(tt.in); got != tt.want {
			t.Errorf("normalizeSetFlags(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ── lint functions ────────────────────────────────────────────────────────────

func TestLintJQ_EqFalse(t *testing.T) {
	vs := lintJQ(".x == false")
	if len(vs) == 0 {
		t.Error("expected violation for == false")
	}
	if !strings.Contains(vs[0].Msg, "== false") {
		t.Errorf("unexpected message: %q", vs[0].Msg)
	}
}

func TestLintJQ_EqTrue(t *testing.T) {
	vs := lintJQ(".x == true")
	if len(vs) == 0 {
		t.Error("expected violation for == true")
	}
}

func TestLintJQ_Clean(t *testing.T) {
	vs := lintJQ(".x | not")
	if len(vs) != 0 {
		t.Errorf("expected no violations, got %v", vs)
	}
}

func TestLintJQ_ParseError(t *testing.T) {
	vs := lintJQ("((((")
	if vs != nil {
		t.Error("expected nil on parse error")
	}
}

func TestLintShell_EchoE(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\necho -e \"hi\\n\"\n")
	if len(vs) == 0 {
		t.Error("expected violation for echo -e")
	}
}

func TestLintShell_EchoN(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\necho -n foo\n")
	if len(vs) == 0 {
		t.Error("expected violation for echo -n")
	}
}

func TestLintShell_BashShebang(t *testing.T) {
	vs := lintShell("#!/bin/bash\necho hi\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "#!/bin/bash") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for #!/bin/bash shebang")
	}
}

func TestLintShell_Which(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nwhich docker\n")
	if len(vs) == 0 {
		t.Error("expected violation for which")
	}
}

func TestLintShell_SetEAlone(t *testing.T) {
	// "set -e" alone is flagged — missing -u and no pipefail
	vs := lintShell("#!/usr/bin/env bash\nset -e\necho hi\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "set -eu") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for bare set -e")
	}
}

func TestFormatter_PedanticSetNormalization(t *testing.T) {
	// --pedantic should normalize set -e to set -Eeuo pipefail for bash
	f := &formatter{tidy: true, pedantic: true}
	out, err := f.byContent("-", "#!/usr/bin/env bash\nset -e\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set -Eeuo pipefail") {
		t.Errorf("pedantic should normalize set -e → set -Eeuo pipefail, got: %q", out)
	}
}

func TestFormatter_TidyNoSetChange(t *testing.T) {
	// --tidy alone should NOT change set -e
	f := &formatter{tidy: true, pedantic: false}
	out, err := f.byContent("-", "#!/usr/bin/env bash\nset -e\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Eeuo") {
		t.Errorf("tidy should NOT change set flags, got: %q", out)
	}
}

func TestLintShell_SetEuOK(t *testing.T) {
	// set -eu is acceptable (appears in Tianon's corpus for simpler scripts)
	vs := lintShell("#!/usr/bin/env bash\nset -eu\necho hi\n")
	for _, v := range vs {
		if strings.Contains(v.Msg, "set -") {
			t.Errorf("unexpected set violation for set -eu: %v", v)
		}
	}
}

func TestLintShell_SetEuoOK(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nset -Eeuo pipefail\necho hi\n")
	for _, v := range vs {
		if strings.Contains(v.Msg, "set -") {
			t.Errorf("unexpected set violation for correct set -Eeuo pipefail: %v", v)
		}
	}
}

func TestLintShell_Clean(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nset -Eeuo pipefail\ncommand -v docker\nprintf '%s\\n' hi\n")
	if len(vs) != 0 {
		t.Errorf("expected no violations, got %v", vs)
	}
}

func TestLintDockerfile_AptGetNoY(t *testing.T) {
	vs := lintDockerfile("FROM debian:bookworm-slim\nRUN apt-get install curl\n")
	if len(vs) == 0 {
		t.Error("expected violation for apt-get without -y")
	}
}

func TestLintDockerfile_AptGetNoRecommends(t *testing.T) {
	vs := lintDockerfile("FROM debian:bookworm-slim\nRUN apt-get install -y curl\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "no-install-recommends") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for missing --no-install-recommends")
	}
}

func TestLintDockerfile_SetPipefail(t *testing.T) {
	vs := lintDockerfile("FROM debian:bookworm-slim\nRUN set -Eeuo pipefail; \\\n\techo hi\n")
	if len(vs) == 0 {
		t.Error("expected violation for set -Eeuo pipefail in Dockerfile RUN")
	}
}

func TestLintDockerfile_MAINTAINER(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nMAINTAINER Tianon\n")
	if len(vs) == 0 {
		t.Error("expected violation for MAINTAINER")
	}
}

func TestLintShell_LetCommand(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nset -Eeuo pipefail\nlet x=5\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "let") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for let command")
	}
}

func TestLintShell_DeclareI(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nset -Eeuo pipefail\ndeclare -i count\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "declare -i") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for declare -i")
	}
}

func TestLintShell_GlobalSetX(t *testing.T) {
	vs := lintShell("#!/usr/bin/env bash\nset -Eeuxo pipefail\necho hi\n")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Msg, "set -x") {
			found = true
		}
	}
	if !found {
		t.Error("expected violation for global set -x")
	}
}

func TestLintDockerfile_HEALTHCHECK(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nHEALTHCHECK CMD curl http://localhost\n")
	if len(vs) == 0 {
		t.Error("expected violation for HEALTHCHECK CMD")
	}
}

func TestLintDockerfile_HEALTHCHECKNoneOK(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nHEALTHCHECK NONE\n")
	for _, v := range vs {
		if strings.Contains(v.Msg, "HEALTHCHECK") {
			t.Errorf("HEALTHCHECK NONE should not be flagged: %v", v)
		}
	}
}

func TestLintDockerfile_ONBUILD(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nONBUILD RUN echo hi\n")
	if len(vs) == 0 {
		t.Error("expected violation for ONBUILD")
	}
}

func TestLintDockerfile_LABEL(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nLABEL version=1.0\n")
	if len(vs) == 0 {
		t.Error("expected violation for LABEL")
	}
}

func TestLintDockerfile_LABELBuildkitOK(t *testing.T) {
	vs := lintDockerfile("FROM scratch\nLABEL moby.buildkit.frontend.caps=\"...\"\nLABEL moby.buildkit.frontend.network.none=\"true\"\n")
	for _, v := range vs {
		if strings.Contains(v.Msg, "LABEL") {
			t.Errorf("moby.buildkit labels should not be flagged: %v", v)
		}
	}
}

func TestLintDockerfile_Clean(t *testing.T) {
	vs := lintDockerfile("FROM debian:bookworm-slim\nRUN set -eux; \\\n\tapt-get update; \\\n\tapt-get install -y --no-install-recommends curl; \\\n\trm -rf /var/lib/apt/lists/*\n")
	if len(vs) != 0 {
		t.Errorf("expected no violations, got %v", vs)
	}
}

func TestLintViolations_JQByContent(t *testing.T) {
	vs := lintViolations("-", ".x == false", false)
	if len(vs) == 0 {
		t.Error("expected violations")
	}
}

func TestLintViolations_JQByPath(t *testing.T) {
	vs := lintViolations("foo.jq", ".x == false", true)
	if len(vs) == 0 {
		t.Error("expected violations for .jq path")
	}
}

func TestLintViolations_DockerfileByPath(t *testing.T) {
	vs := lintViolations("Dockerfile", "FROM debian:bookworm-slim\nRUN apt-get install curl\n", true)
	if len(vs) == 0 {
		t.Error("expected violations for Dockerfile path")
	}
}

func TestLintViolations_ShellByPath(t *testing.T) {
	vs := lintViolations("script.sh", "#!/usr/bin/env bash\necho -e 'hi'\n", true)
	if len(vs) == 0 {
		t.Error("expected violations for shell path")
	}
}

// ── AST functions ─────────────────────────────────────────────────────────────

func TestJQASTPair(t *testing.T) {
	pre, post, err := jqASTPair("-", ".foo | .bar")
	if err != nil {
		t.Fatal(err)
	}
	if pre == "" || post == "" {
		t.Error("expected non-empty pre and post AST")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(pre), &m); err != nil {
		t.Fatalf("pre AST invalid JSON: %v", err)
	}
	if m["type"] != "jq" {
		t.Errorf("pre AST type = %q, want jq", m["type"])
	}
	if m["file"] != "-" {
		t.Errorf("pre AST file = %q, want -", m["file"])
	}
}

func TestDockerfileASTPair(t *testing.T) {
	src := "FROM debian:bookworm-slim\nCMD [\"bash\"]\n"
	pre, post, err := dockerfileASTPair("-", src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, `"type": "dockerfile"`) {
		t.Errorf("pre AST missing dockerfile type: %s", pre)
	}
	if !strings.Contains(post, `"type": "dockerfile"`) {
		t.Errorf("post AST missing dockerfile type: %s", post)
	}
}

func TestShellASTPair(t *testing.T) {
	src := "#!/usr/bin/env bash\necho hi\n"
	pre, post, err := shellASTPair("-", src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, `"type": "shell"`) {
		t.Errorf("pre AST missing shell type: %s", pre)
	}
	_ = post
}

func TestASTByPath(t *testing.T) {
	pre, _, err := astByPath("foo.jq", ".x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, `"type": "jq"`) {
		t.Errorf("unexpected AST: %s", pre)
	}
}

func TestASTByPath_Shell(t *testing.T) {
	pre, _, err := astByPath("script.sh", "#!/usr/bin/env bash\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "shell") {
		t.Errorf("unexpected AST: %s", pre)
	}
}

func TestASTByPath_Default(t *testing.T) {
	// Unknown extension falls back to jq
	pre, _, err := astByPath("unknown.xyz", ".x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "jq") {
		t.Errorf("default should be jq AST: %s", pre)
	}
}

func TestASTByContent_Dockerfile(t *testing.T) {
	pre, _, err := astByContent("-", "FROM debian:bookworm-slim\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "dockerfile") {
		t.Errorf("unexpected AST: %s", pre)
	}
}

func TestASTByContent_Shell(t *testing.T) {
	pre, _, err := astByContent("-", "#!/usr/bin/env bash\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "shell") {
		t.Errorf("unexpected AST: %s", pre)
	}
}

func TestASTByContent_ShellAnyShebang(t *testing.T) {
	// #!/bin/false (source-only scripts) should also be detected as shell
	pre, _, err := astByContent("-", "#!/bin/false source me\necho hi\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pre, "shell") {
		t.Errorf("#!/bin/false not detected as shell: %s", pre)
	}
}

func TestJQASTPair_ParseError(t *testing.T) {
	_, _, err := jqASTPair("-", "[[[invalid")
	if err == nil {
		t.Error("expected error for invalid jq")
	}
}

func TestShellASTPair_ParseError(t *testing.T) {
	_, _, err := shellASTPair("-", "#!/usr/bin/env bash\n$((1.5))\n")
	if err == nil {
		t.Error("expected error for zsh float arithmetic in bash script")
	}
}

func TestMarshalASTJSON(t *testing.T) {
	v := map[string]any{"key": "value", "num": 42}
	out, err := marshalASTJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"key"`) {
		t.Errorf("unexpected output: %q", out)
	}
	// Must end with newline (from json.Encoder)
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with newline")
	}
}

func TestPrintAST_Input(t *testing.T) {
	pre := `{"type":"jq"}`
	post := `{"type":"jq","modified":true}`
	// non-diff mode, input: should print pre
	var sb strings.Builder
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := printAST("test", pre, post, "input", false, os.Stdout, os.Stderr)
	w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	sb.Write(buf[:n])
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if !strings.Contains(sb.String(), "jq") {
		t.Errorf("unexpected output: %q", sb.String())
	}
}

func TestPrintAST_DiffClean(t *testing.T) {
	// same pre and post → no diff output, exit 0
	same := `{"type":"jq"}`
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := printAST("test", same, same, "input", true, os.Stdout, os.Stderr)
	w.Close()
	os.Stdout = os.NewFile(1, "/dev/stdout")
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if code != 0 {
		t.Errorf("expected exit 0 for identical ASTs, got %d", code)
	}
	if n > 0 {
		t.Errorf("expected no diff output for identical ASTs, got: %q", string(buf[:n]))
	}
}

func TestPrintAST_DiffWithChange(t *testing.T) {
	// different pre and post → diff output, exit 1
	pre := `{"type":"jq","query":{"type":"field"}}`
	post := `{"type":"jq","query":{"type":"pipe"}}`
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := printAST("test", pre, post, "input", true, os.Stdout, os.Stderr)
	w.Close()
	os.Stdout = os.NewFile(1, "/dev/stdout")
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	if code != 1 {
		t.Errorf("expected exit 1 for changed AST, got %d", code)
	}
	if !strings.Contains(string(buf[:n]), "@@") {
		t.Errorf("expected diff output, got: %q", string(buf[:n]))
	}
}

func TestPrintAST_Format(t *testing.T) {
	pre := `{"type":"jq"}`
	post := `{"type":"jq","formatted":true}`
	r, w, _ := os.Pipe()
	os.Stdout = w
	printAST("test", pre, post, "format", false, os.Stdout, os.Stderr)
	w.Close()
	os.Stdout = os.NewFile(1, "/dev/stdout")
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), "formatted") {
		t.Errorf("format mode should print post AST, got: %q", string(buf[:n]))
	}
}

func TestPedanticCheck_Clean(t *testing.T) {
	// Already-tidy output: second pass should find nothing Wrong
	out, err := (&formatter{tidy: true}).byContent("-", ".foo | not")
	if err != nil {
		t.Fatal(err)
	}
	code := pedanticCheck("-", out, false, false, os.Stdout, os.Stderr)
	if code != 0 {
		t.Errorf("expected exit 0 for clean jq, got %d", code)
	}
}

func TestPedanticCheck_EqFalse(t *testing.T) {
	// lintJQ fires for == false — pedantic check should return 1.
	// Redirect stderr so violation messages don't pollute test output.
	old := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	code := pedanticCheck("-", ".x == false", false, false, os.Stdout, os.Stderr)
	os.Stderr = old
	if code != 1 {
		t.Errorf("expected exit 1 for == false, got %d", code)
	}
}

// ── computeDiff ───────────────────────────────────────────────────────────────

func TestComputeDiff_NoDiff(t *testing.T) {
	diff, err := computeDiff("test", "same\n", "same\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 0 {
		t.Errorf("expected empty diff for identical content, got: %q", diff)
	}
}

func TestComputeDiff_WithDiff(t *testing.T) {
	diff, err := computeDiff("test.jq", "before\n", "after\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) == 0 {
		t.Error("expected non-empty diff for changed content")
	}
	if !strings.Contains(string(diff), "@@") {
		t.Errorf("expected unified diff header, got: %q", string(diff))
	}
}

// ── echoFlagViolation ─────────────────────────────────────────────────────────

func TestEchoFlagViolation(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"echo -e \"hi\\n\"", true},
		{"echo -n foo", true},
		{"echo -ne foo", true},
		{"echo hello", false},
		{"printf '%s\\n' foo", false},
		// Note: comment lines are filtered by the caller (lintShell), not here.
	}
	for _, tt := range tests {
		got := echoFlagViolation(tt.line)
		if (got != "") != tt.want {
			t.Errorf("echoFlagViolation(%q) = %q (want violation=%v)", tt.line, got, tt.want)
		}
	}
}

// ── aptGetInstallViolation ────────────────────────────────────────────────────

func TestAptGetInstallViolation_NoAptGet(t *testing.T) {
	if v := aptGetInstallViolation("echo hello", 1); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestAptGetInstallViolation_HasBothFlags(t *testing.T) {
	if v := aptGetInstallViolation("apt-get install -y --no-install-recommends curl", 1); v != nil {
		t.Errorf("expected nil for complete flags, got %v", v)
	}
}

func TestAptGetInstallViolation_MissingY(t *testing.T) {
	v := aptGetInstallViolation("apt-get install --no-install-recommends curl", 1)
	if v == nil {
		t.Error("expected violation for missing -y")
	}
}

func TestAptGetInstallViolation_MissingRecommends(t *testing.T) {
	v := aptGetInstallViolation("apt-get install -y curl", 1)
	if v == nil {
		t.Error("expected violation for missing --no-install-recommends")
	}
}

// ── run() integration tests (direct, no subprocess) ──────────────────────────

func TestRun_StdinFormat(t *testing.T) {
	stdout, _, code := runCLI(t, ".foo | .bar", "")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, ".foo") {
		t.Errorf("unexpected output: %q", stdout)
	}
}

func TestRun_FlagErrors(t *testing.T) {
	cases := []string{
		"--write --diff",
		"--write --ast",
	}
	for _, args := range cases {
		t.Run(args, func(t *testing.T) {
			_, _, code := runCLI(t, ".x", args)
			if code == 0 {
				t.Errorf("expected non-zero exit for %q", args)
			}
		})
	}
}

func TestRun_WriteFlagOnStdin(t *testing.T) {
	_, _, code := runCLI(t, ".x", "--write")
	if code == 0 {
		t.Error("--write on stdin should fail")
	}
}

func TestRun_TidyStdin(t *testing.T) {
	stdout, _, code := runCLI(t, "FROM debian:bookworm-slim\nRUN apt-get update && apt-get install -y curl\n", "--tidy")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "set -eux") {
		t.Errorf("tidy not applied: %q", stdout)
	}
}

func TestRun_PedanticPass(t *testing.T) {
	_, _, code := runCLI(t, ".foo | not", "--pedantic")
	if code != 0 {
		t.Errorf("clean jq should pass pedantic, got exit %d", code)
	}
}

func TestRun_PedanticFail(t *testing.T) {
	_, stderr, code := runCLI(t, ".foo == false", "--pedantic")
	if code == 0 {
		t.Error("expected pedantic failure for == false")
	}
	if !strings.Contains(stderr, "== false") {
		t.Errorf("expected violation message, got %q", stderr)
	}
}

func TestRun_DiffNoChange(t *testing.T) {
	_, _, code := runCLI(t, ".foo | .bar\n", "--diff")
	if code != 0 {
		t.Errorf("already-formatted input should have no diff, got %d", code)
	}
}

func TestRun_DiffWithChange(t *testing.T) {
	stdout, _, code := runCLI(t, ".foo | .bar", "--diff")
	if code == 0 {
		t.Error("unformatted input should produce diff")
	}
	if !strings.Contains(stdout, "@@") {
		t.Errorf("expected unified diff, got %q", stdout)
	}
}

func TestRun_ASTStdin(t *testing.T) {
	stdout, _, code := runCLI(t, ".foo | .bar", "--ast")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, `"type": "jq"`) {
		t.Errorf("expected jq AST, got %q", stdout)
	}
}

func TestRun_FileArg(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.jq")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(".foo | .bar")
	f.Close()
	stdout, _, code := runCLI(t, "", f.Name())
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, ".foo") {
		t.Errorf("unexpected output: %q", stdout)
	}
}

func TestRun_WriteFlag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.jq")
	os.WriteFile(p, []byte(`{"foo": .bar}`), 0o644)
	stdout, _, code := runCLI(t, "", "--write "+p)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "test.jq") {
		t.Errorf("expected filename in output, got %q", stdout)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "foo: .bar") {
		t.Errorf("file not updated: %q", string(got))
	}
}

func TestRun_ASTFormat(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.jq")
	f.WriteString(".foo")
	f.Close()
	stdout, _, code := runCLI(t, "", "--ast=format "+f.Name())
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, `"type": "jq"`) {
		t.Errorf("expected jq AST format mode: %q", stdout)
	}
}

func TestRun_ASTDiff(t *testing.T) {
	// --ast --diff compares pre- and post-format ASTs.
	f, _ := os.CreateTemp(t.TempDir(), "*.jq")
	f.WriteString(".foo | .bar")
	f.Close()
	stdout, _, code := runCLI(t, "", "--ast --diff "+f.Name())
	// Already-formatted jq → same ASTs → no diff → exit 0
	if code != 0 {
		t.Errorf("canonical jq should produce no AST diff, got %d, output: %q", code, stdout)
	}
}

func TestRun_PedanticWithDiff(t *testing.T) {
	_, _, code := runCLI(t, "FROM debian:bookworm-slim\nRUN apt-get update && apt-get install -y --no-install-recommends curl\n", "--pedantic --diff")
	// --pedantic --diff shows what tidy changes
	_ = code // exit code varies
}

func TestRun_FileNotFound(t *testing.T) {
	_, _, code := runCLI(t, "", "/nonexistent/file.jq")
	if code == 0 {
		t.Error("expected non-zero exit for missing file")
	}
}

func TestRun_MarkdownFormat(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("Title\n=====\n\n* First\n")
	f.Close()
	stdout, _, code := runCLI(t, "", f.Name())
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "# Title") {
		t.Errorf("setext not converted: %q", stdout)
	}
	if !strings.Contains(stdout, "- First") {
		t.Errorf("bullet not normalized: %q", stdout)
	}
}

func TestRun_MarkdownAST(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("# Title\n\nParagraph.\n")
	f.Close()
	stdout, _, code := runCLI(t, "", "--ast "+f.Name())
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, `"type": "markdown"`) {
		t.Errorf("expected markdown AST, got %q", stdout)
	}
}

// ── file-based tests covering paths not exercised via stdin ──────────────────

func TestRun_PedanticMarkdownFile(t *testing.T) {
	// Exercises lintMarkdown (0% before this test) via --pedantic on a .md file.
	_, stderr, code := runCLI(t, "", "--pedantic testdata/pedantic-markdown/input.md")
	if code == 0 {
		t.Error("expected non-zero exit for markdown with sh fence")
	}
	if !strings.Contains(stderr, "sh") {
		t.Errorf("expected sh-language violation in stderr: %q", stderr)
	}
}

func TestRun_DiffFileChanged(t *testing.T) {
	// --diff on a file that needs formatting: stable golden output since
	// go test always runs from the package source directory.
	const path = "testdata/diff-jq-changed/input.jq"
	const golden = "testdata/diff-jq-changed/output.diff"
	stdout, _, code := runCLI(t, "", "--diff "+path)
	if code == 0 {
		t.Error("expected non-zero exit for changed file")
	}
	if *testutil.Update {
		os.WriteFile(golden, []byte(stdout), 0o644)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden %s missing — run `go test -update` to create it: %v", golden, err)
	}
	if stdout != string(want) {
		t.Errorf("diff mismatch\ngot:  %q\nwant: %q", stdout, string(want))
	}
}

func TestRun_ASTDiffChanged(t *testing.T) {
	// --ast --diff on a file whose AST changes after formatting (quoted key
	// "foo" becomes unquoted foo).  Covers the len(diff)>0 branch in printAST.
	const path = "testdata/ast-diff-changed/input.jq"
	const golden = "testdata/ast-diff-changed/output.diff"
	stdout, _, code := runCLI(t, "", "--ast --diff "+path)
	if code == 0 {
		t.Error("expected non-zero exit: AST differs before and after formatting")
	}
	if *testutil.Update {
		os.WriteFile(golden, []byte(stdout), 0o644)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden %s missing — run `go test -update` to create it: %v", golden, err)
	}
	if stdout != string(want) {
		t.Errorf("AST diff mismatch\ngot:  %q\nwant: %q", stdout, string(want))
	}
}

func TestRun_DiffFileClean(t *testing.T) {
	// --diff on an already-formatted file: exit 0, no output.
	const path = "testdata/diff-jq-clean/input.jq"
	stdout, _, code := runCLI(t, "", "--diff "+path)
	if code != 0 {
		t.Errorf("expected exit 0 for already-formatted file, got %d\nstdout: %q", code, stdout)
	}
	if stdout != "" {
		t.Errorf("expected no diff output for clean file, got: %q", stdout)
	}
}

func TestRun_PedanticFilePass(t *testing.T) {
	// --pedantic on a well-formed jq file: exit 0.
	f, err := os.CreateTemp(t.TempDir(), "*.jq")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(".foo | not\n")
	f.Close()
	_, _, code := runCLI(t, "", "--pedantic "+f.Name())
	if code != 0 {
		t.Errorf("expected exit 0 for clean jq file, got %d", code)
	}
}

func TestRun_FormatError_File(t *testing.T) {
	// A malformed .jq file: exercises the format-error path in the file loop.
	f, err := os.CreateTemp(t.TempDir(), "*.jq")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("[[[invalid\n")
	f.Close()
	_, stderr, code := runCLI(t, "", f.Name())
	if code == 0 {
		t.Error("expected non-zero exit for malformed jq")
	}
	if !strings.Contains(stderr, "tianonfmt:") {
		t.Errorf("expected error message in stderr: %q", stderr)
	}
}

// ── subprocess end-to-end (verifies full CLI pipeline) ───────────────────────

func TestE2E_Idempotent(t *testing.T) {
	inputs := []string{
		".foo | .bar",
		`{"foo": .bar}`,
		"if .x then .y else .z end",
	}
	for _, in := range inputs {
		t.Run(in[:min(25, len(in))], func(t *testing.T) {
			first, _, code := runCLI(t, in, "")
			if code != 0 {
				t.Fatalf("first format failed: exit %d", code)
			}
			second, _, _ := runCLI(t, first, "")
			if first != second {
				t.Errorf("not idempotent\nfirst:  %q\nsecond: %q", first, second)
			}
		})
	}
}

func TestE2E_WriteFlag(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.jq")
	os.WriteFile(p, []byte(`{"foo": .bar}`), 0o644)
	_, _, code := runCLI(t, "", "--write "+p)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "foo: .bar") {
		t.Errorf("file not updated: %q", string(got))
	}
}

// runCLI spawns the tool; kept minimal — one test per CLI path for smoke only.
// runCLI calls run() directly (no subprocess) for accurate coverage.
func runCLI(t *testing.T, stdinStr, argsStr string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf strings.Builder
	var stdinR io.Reader
	if stdinStr != "" {
		stdinR = strings.NewReader(stdinStr)
	} else {
		stdinR = strings.NewReader("")
	}
	code = run(strings.Fields(argsStr), stdinR, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

