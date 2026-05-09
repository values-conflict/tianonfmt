package flags_test

import (
	"testing"

	"github.com/values-conflict/tianonfmt/internal/flags"
)

func newTestFS() (*flags.FlagSet, *bool, *bool, *string) {
	fs := flags.New("test")
	write := fs.Bool("write", 'w', "write files")
	verbose := fs.Bool("verbose", 0, "verbose")
	out := fs.OptString("output", 'o', "stdout", "output dest")
	return fs, write, verbose, out
}

func TestParse_LongFlag(t *testing.T) {
	fs, write, _, _ := newTestFS()
	rest, err := fs.Parse([]string{"--write"})
	if err != nil || !*write || len(rest) != 0 {
		t.Errorf("--write: err=%v write=%v rest=%v", err, *write, rest)
	}
}

func TestParse_LongFlagEqualsTrue(t *testing.T) {
	fs, write, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"--write=true"}); err != nil || !*write {
		t.Errorf("--write=true: err=%v write=%v", err, *write)
	}
}

func TestParse_LongFlagEqualsFalse(t *testing.T) {
	fs, write, _, _ := newTestFS()
	*write = true
	if _, err := fs.Parse([]string{"--write=false"}); err != nil || *write {
		t.Errorf("--write=false: err=%v write=%v", err, *write)
	}
}

func TestParse_LongFlagEqualsYes(t *testing.T) {
	fs, write, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"--write=yes"}); err != nil || !*write {
		t.Errorf("--write=yes: err=%v write=%v", err, *write)
	}
}

func TestParse_LongFlagEqualsNo(t *testing.T) {
	fs, write, _, _ := newTestFS()
	*write = true
	if _, err := fs.Parse([]string{"--write=no"}); err != nil || *write {
		t.Errorf("--write=no: err=%v write=%v", err, *write)
	}
}

func TestParse_LongFlagEqualsInvalid(t *testing.T) {
	fs, _, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"--write=maybe"}); err == nil {
		t.Error("expected error for --write=maybe")
	}
}

func TestParse_ShortFlag(t *testing.T) {
	fs, write, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"-w"}); err != nil || !*write {
		t.Errorf("-w: err=%v write=%v", err, *write)
	}
}

func TestParse_OptStringWithValue(t *testing.T) {
	fs, _, _, out := newTestFS()
	if _, err := fs.Parse([]string{"--output=file.txt"}); err != nil || *out != "file.txt" {
		t.Errorf("--output=file.txt: err=%v out=%v", err, *out)
	}
}

func TestParse_OptStringWithoutValue(t *testing.T) {
	fs, _, _, out := newTestFS()
	if _, err := fs.Parse([]string{"--output"}); err != nil || *out != "stdout" {
		t.Errorf("--output (no value): err=%v out=%v", err, *out)
	}
}

func TestParse_Positional(t *testing.T) {
	fs, _, _, _ := newTestFS()
	rest, err := fs.Parse([]string{"file.jq", "other.jq"})
	if err != nil || len(rest) != 2 {
		t.Errorf("positionals: err=%v rest=%v", err, rest)
	}
}

func TestParse_BareDash(t *testing.T) {
	fs, _, _, _ := newTestFS()
	rest, err := fs.Parse([]string{"-"})
	if err != nil || len(rest) != 1 || rest[0] != "-" {
		t.Errorf("bare dash: err=%v rest=%v", err, rest)
	}
}

func TestParse_EndOfFlags(t *testing.T) {
	fs, write, _, _ := newTestFS()
	rest, err := fs.Parse([]string{"--", "--write", "file.jq"})
	if err != nil || *write || len(rest) != 2 {
		t.Errorf("--: err=%v write=%v rest=%v", err, *write, rest)
	}
}

func TestParse_UnknownLongFlag(t *testing.T) {
	fs, _, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"--unknown"}); err == nil {
		t.Error("expected error for unknown long flag")
	}
}

func TestParse_UnknownShortFlag(t *testing.T) {
	fs, _, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"-z"}); err == nil {
		t.Error("expected error for unknown short flag")
	}
}

func TestParse_SingleDashLongName(t *testing.T) {
	// -write (single dash + multi-char) should suggest using --write
	fs, _, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"-write"}); err == nil {
		t.Error("expected error for -write (use --write)")
	}
}

func TestParse_SingleDashUnknownMultiChar(t *testing.T) {
	// -xyz (single dash + unknown multi-char) should error
	fs, _, _, _ := newTestFS()
	if _, err := fs.Parse([]string{"-xyz"}); err == nil {
		t.Error("expected error for -xyz")
	}
}

func TestParse_OptStringShort(t *testing.T) {
	// -o exercises optStringFlag.setShort(), setting the value to the default.
	fs, _, _, out := newTestFS()
	if _, err := fs.Parse([]string{"-o"}); err != nil || *out != "stdout" {
		t.Errorf("-o (optstring short): err=%v out=%v", err, *out)
	}
}

func TestParse_FlagThenPositional(t *testing.T) {
	fs, write, _, _ := newTestFS()
	rest, err := fs.Parse([]string{"--write", "file.jq"})
	if err != nil || !*write || len(rest) != 1 || rest[0] != "file.jq" {
		t.Errorf("flag then positional: err=%v write=%v rest=%v", err, *write, rest)
	}
}
