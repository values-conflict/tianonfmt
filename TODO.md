<!-- remove from file when complete; keep a double space between TODO entries so they're more readable / digestible -->
<!-- sub-bullets (2-space indent, `-`, cuddled -- no blank line between parent and sub-bullets, nor between sibling sub-bullets) are for related side notes subordinate to the main item but distinct enough to stand alone -- use a semicolon continuation for the same thought, a sub-bullet for a related angle, and a new top-level entry for a separate concern -->

- jq formatter bug (changes program meaning): trailing comments on `elif COND then  # comment` lines get displaced into nested conditions.  The comment is parsed as a leading comment on the branch body (an inner `if` expression), and the inner `if`'s condition-leading-comment extraction code in `writeIfClause` embeds it into the multi-line condition block rather than emitting it before the `if` keyword.  Same bug causes `## ── SECTION ──` header comments between `elif` groups to appear inside the body of the *preceding* `elif` rather than before the next one.  Affects any file that uses the `elif $ph == "X" then  # phase-label` pattern (notably gzip.jq).
  - real impact (jq-unhinged's `gzip.jq`): `if .rem == 0 then` under `elif $ph == "dy1" then  # HCLEN+4 code-length alphabet entries (3 bits each)` comes out as `.rem # HCLEN+4 code-length alphabet entries (3 bits each) == 0`, so `== 0` silently becomes comment text and the condition degrades to `if .rem`; 2 of the 14 `gzip.jqtest` tests fail after formatting
  - minimal repro: an `elif COND then  # c` whose body is `if . then 1 else 2 end` comes out as `if . # c then 1 else 2 end`, which isn't even valid jq

- jq formatter bug: `import "module" as name;` (bare identifier alias) fails to parse -- the parser expects `as $var` (a `Var` token) but jq allows a plain identifier; this blocks formatting of any file that uses module imports with aliases (e.g. `sha256.jq`, `sha512.jq`, `blake3.jq` all use `import "bits" as bits;`)

- jq formatter bug: blank lines intentionally separating two comment blocks (e.g. a trailing blank line between a performance note and a section header, or a blank line between a table and its caption) get collapsed -- the formatter only tracks `consecutiveBlanks > 1` for inter-def spacing, but has no concept of "blank line inside a comment sequence"; it should preserve any blank line that appears between two consecutive `#`-comment lines

- jq formatter bug: a `BinOp` whose rightmost operand carries a trailing comment (e.g. `. - 65 # A–Z → 0–25`) incorrectly splits to two lines: `. \n- 65 # A–Z → 0–25`.  Root cause: `inlineSafe` → `anyNodeHasTrailingComment` returns true for a `CommentedExpr` with a trailing comment before even checking length, so the short-inline path is never taken.  Fix: when the full inline form `"EXPR COMMENT"` fits within the threshold, allow it — trailing comments don't prevent inlining, they prevent something *after* the expression from landing inside the comment.

- jq formatter bug: dense numeric lookup tables (multi-line arrays with multiple items per row) get fully expanded to one item per line.  Two separate mechanisms both force one-per-line: (1) `commaChainHasMultiLine` fires on the row-boundary commas and forces `commaExprMultiLine`, which renders every part on its own line regardless; (2) even if that were fixed, `commaChainIsLongFlat` fires on the inline rows.  Adding a trailing `empty` does not help because `commaChainHasMultiLine` fires first.  The only current workaround is not running the formatter on these arrays.  Real fix needs a way to preserve "N items per line" grouping intentionally written by the author (maybe strictly limited to literals? something like `"--foo", "bar",\n"--baz", "buzz"` would be sane too).

- jq formatter: arithmetic chains where an array lookup is multiplied by a scale factor (`$t[key] * 16`) split awkwardly when `key` itself is long -- the `* 16` ends up on its own line and visually looks like it multiplies the whole preceding sum rather than just `$t[key]`.  this shows up in `band` in bits.jq.  ideal behavior: treat `$t[key] * factor` as one unit and keep it on one line when it fits; only break the array-index expression itself if the key alone exceeds the threshold
  - confirmed on the real `band` in bits.jq: it also breaks a `+` term in the middle of an index expression (`$t[(($a / 65536 | floor) % 16) * 16` / `+ (($b / 65536 | floor) % 16)] * 65536`)

- jq formatter: `reduce EXPR as $v (INIT; BODY)` currently puts `INIT` on the same line as the opening `(`, which is fine for short inits (e.g. `reduce ... as $i (0; ...)`), but awkward for multi-field object inits like `{ c: 0, fc: {} }` where a separate line would be cleaner.  `foreach` has the same layout today, so neither has the short-init-inline / long-init-newline split yet; both should get it
  - `docs/jq.md`'s `reduce` example already puts the init on its own line

- jq formatter: `if COND then (EXPR) elif/else/end` -- in standalone and shell-embedded jq, the parens around an if/elif/else branch body are *always* syntactically unnecessary: the parser terminates the branch at the next keyword regardless of what EXPR contains (arithmetic, pipeline, comma expression, anything).  `--tidy` should unwrap these: `if COND then (X) elif` → `if COND then\n\tX\nelif`.  The formatter currently preserves the `*Paren` node and renders it in cuddled form, which both retains the redundant parens and forces a specific layout.
  - never unwrap in jq-template (`Dockerfile.template` and friends): there the parens are what let a block continue across template markers (`{{ if ... then ( -}} ... {{ ) else "" end -}}`)
  - `docs/jq.md`'s "Cuddled-paren branch form" is template-only in practice: 0 occurrences in `.jq` or `.sh` files across `corpus` and `anticorpus` (checked 2026-10-19), and every hit is in a jq-template file; move that section to `jq-template.md` or scope it there

- regular expression formatting (applies wherever regexes appear: jq `test()`/`match()`/`gsub()`/etc., shell `=~`, grep patterns, etc.):
  - `\\.` should be written as `[.]` (perl-idiomatic: character class is clearer and avoids the double-backslash visual noise from string escaping fighting with regex escaping)
  - long regexes should use `(?x)` extended mode so they can be broken across lines with inline comments explaining each clause; the formatter should detect regexes that exceed some length threshold and suggest/apply `(?x)` expansion

- multi-line string literal formatting: when a string literal spans multiple lines, the closing `"` should be on its own line (like a closing `}` or `]`), making the string boundaries unambiguous.  `(?x)` must be at the very start of the string (position 0) -- a leading newline before `(?x)` causes it to be treated as a literal match character, not a mode switch.  `(` / `)` grouping inside `(?x)` patterns should use block indentation like code blocks, with alternatives on their own indented lines and `|` leading each:
  ```
  test("(?x)
      some[.]host/path/[^/]+/[^/]+/
      (
          endpoint-a/[0-9]+/resource
          | endpoint-b/[0-9]+/resource
          | endpoint-c/[a-f0-9]+/resource
      )
  ")
  ```
  tianonfmt currently preserves multi-line string content verbatim (the `Raw` field is emitted as-is), so it doesn't enforce or reformat the internal layout at all -- this would need to be a new formatting pass

- we should support `.jqtest` files too (maybe we can even cleverly detect these by their format?)
  - hard constraint: `jq --run-tests` requires each test expression, input, and expected output to be on exactly one line each, so any formatting pass must guarantee zero newlines in the output for those lines -- probably needs a dedicated single-line mode that only normalizes within-line spacing (spacing around operators, `[` / `]`, etc.) but never inserts newlines

- `foo=$(...)` should always become `foo="$(...)"` -- maybe even *without* `--tidy` because it doesn't change behavior in any meaningful way?
  - corpus: 3951 quoted `foo="$(...)"` assignments vs 4 unquoted (one is `$(( ))` arithmetic, three are in `doi/official-images` test scripts)
  - that only covers plain assignments; `local`/`export`/`declare foo=$(...)` needs checking separately per shell

- are there any cases of things like `if [ $(foo) = 1 ]` that we can assume should *always* be quoted in `--tidy` mode because anything else would've been a typo?
  - corpus: 0 occurrences of an unquoted `[ $(` (checked 2026-10-19)

- unnecessary use of `[[` should be normalized to `[` (probably in `--tidy`)

- `echo [something] | ...` should upgrade to `... <<<[something]` in `--tidy`

- unnecessary use of `"` which should be `'` - we could even normalize these in `--tidy` (but we definitely need to document the pattern that Tianon aggressively avoids `"` unless he needs interpolation *or* it makes the quoting/escaping cleaner, like when he wants `'` inside a string)
  - `docs/bash.md` doesn't document this yet (it only says to double-quote every variable reference)

- we can't act on it in an automated way very well, but we should note in `docs/bash.md` that in bash scripts, long `\`-escaped chains of arguments usually get replaced with a bash array like `args=()` so the whitespace/newlines can be cleaner (`docker run "${args[@]}"` as what's probably the most common expression of this pattern)
  - the Arrays section of `docs/bash.md` is the natural home; it doesn't mention this yet

- when a Dockerfile does `{ echo ...; echo ...; echo ...; }`, `--tidy` should upgrade that to `printf '%s\n' \n '...' \n '...'` instead because that's clever as hell (and a trick I just learned just now)

- we need to figure out what to do about markdown -- how can we reasonably format code blocks that might be invalid code snippets and might even be formatted incorrectly on purpose?
  - open question: lean is warn and skip (which is what `docs/markdown.md` already says happens)
  - today `--tidy` instead aborts the whole file with exit 1 on any unparseable fenced block (and double-prefixes the error: `tianonfmt: bad.md: tianonfmt: bad.md:3: ...`), so either the behavior or the doc has to change; keep the no-silent-pass-through rule in `CLAUDE.md` in mind when deciding

- fold `reformat-md.pl` (one-sentence-per-line) and `reformat-md-2sp.pl` (two-spaces-after-sentence-terminator) into tianonfmt; the real value of doing so is extending the same sentence-boundary detection and reformatting to code comments (Go `//`, bash `#`, etc.) across all supported languages, not just standalone markdown files; the Perl scripts live in `contrib/` and only handle markdown for now
  - `reformat-md-2sp` (a goldmark-based Go port of `reformat-md-2sp.pl`, in `contrib/`) already exists as a separate module and supersedes that Perl script; it still needs folding into `markdown/`, which is currently a line-based formatter with no sentence logic
  - `reformat-md.pl` (one-sentence-per-line) has no Go port yet; see the sentence-per-line open question below
  - the sentence detection lives in the port's `sentenceSpacing` plus its abbreviation list
  - mdreflow (v0.3.0 as of 2026-08-14) is inspiration, not a dependency: its default segmenter is `internal` (only `Format` and a `Segmenter` interface are public), it picked identical sentence boundaries to the port on a tricky sample (same `Jan. 5th.` miss), and it uses the same regex-plus-abbreviation-list approach
  - worth borrowing from mdreflow: its golden-rules-style segmenter test list, its terminal-punctuation-through-inline-markup and sentence-opening-with-inline-code/emphasis cases, and the `Segmenter.Breaks(text) []Span` shape (find the gaps, the caller decides what replaces them) so markdown and comments could share one segmenter
  - if the sentence-per-line question resolves toward sentence-per-line, mdreflow's `sentence` mode becomes a candidate to use directly (its library's only third-party dependency would be goldmark)

- compare goldmark's parse against what our own parsing needs, and decide whether it can be the markdown parser behind the fold-in above (the `reformat-md-2sp` port and `goldmark-cmp` already lean on it)
  - the requirement (see "AST design: parser and formatter are separate concerns" in `CLAUDE.md`): the AST must capture every syntactic form so `format(parse(x))` is stable and semantically equivalent spellings stay distinguishable -- so we need full syntax parsing / a raw AST from goldmark, not just a render-equivalent tree
  - things to check that it retains (or that we can recover from its node source segments): setext vs ATX headings; `*` vs `_` emphasis delimiters; fence character (`` ` `` vs `~`) and length; ordered-list delimiter (`.` vs `)`) and bullet character; trailing-double-space vs backslash hard breaks; inline vs reference vs autolink styles; byte offsets for every node (so prose could be spliced into the original bytes instead of re-rendered, like mdreflow does)
  - compare against what `markdown.MarshalFile` records today (it emits setext and ATX headings identically, for example) and against the line-based `markdown/` formatter
  - if goldmark loses information, the options are recovering it from source segments, supplementing it with our own line-level pass, or writing our own parser; the render-preservation check (`goldmark-cmp`) stays useful either way

- `TODO: ...` in any comment (`#`, `//`, etc.) should be auto-converted to `TODO ...` (no colon) -- the colon form is a bug, not a variant; applies to every language whose comments we handle (shell, jq, Dockerfile, Go, Perl, etc.)
  - corpus: 684 `TODO ` lines vs 9 `TODO:` lines (the `TODO:` ones are jq comments duplicated across `meta-scripts` and `meta-scripts-cosine`, plus third-party Go in vendored patches and docs)
  - this repo's own source has 5 `// TODO:` lines (`cmd/tianonfmt/main.go`, `dockerfile/parser.go`)

- godoc comment wrapping -- for exported doc comments, we should figure out / consider sentence wrapping (https://github.com/jbeda/mdreflow), and for inline struct comments we should hard-wrap/reflow at 80 columns (since they render verbatim in the godoc web view)
  - maybe for *all* markdown we should lean into sentence-per-line too?
  - open question: sentence-per-line (mdreflow-style) vs the current single long line with two spaces after sentence terminators -- `docs/markdown.md` and `docs/prose.md` currently say the latter, `docs/go.md` already allows wrapping godoc at sentence boundaries, and `docs/prose.md`'s universal two-space rule has no sentence-per-line case; undecided
  - mdreflow's `para` mode keeps existing double spaces inside a line but joins wrapped lines with a single space, and its `sentence` mode drops them, so neither produces the two-space style
  - `docs/go.md` says struct field comments should be hard-wrapped but doesn't give a column; 80 would need documenting there

- markdown lint: unescaped angle-bracket placeholders in prose (`<path>`, `<value>`, `<b>`) get interpreted as HTML tags by renderers -- `<b>` in particular makes everything after it render bold
  - lint-only (no safe auto-fix without knowing intent); add a new check to `Lint()`
  - pattern: `<[A-Za-z][^>]*>` on a non-fenced, non-blank line
  - needs an allowlist of intentional GFM block-level HTML: `details`, `summary`, `br`, `picture`, `source`, `a` (and probably a few more -- audit the anticorpus)
  - docs/markdown.md already says "HTML tags in markdown -- avoided" under Notable omissions, which is consistent with flagging these
  - note that `docs/markdown.md` allows only `<small>` (and lists explicit `<br>` as a notable omission), so decide whether the allowlist above is GFM-compat only or also style

- markdown lint/fix: bold-headed notes mid-paragraph (`. **Note:**`) not separated by a blank line cause the whole section to render as one wall of text
  - detection: a paragraph line ending in `.`/`)`/`:` immediately followed (no blank) by a line starting with `**...**:`
  - auto-fix requires `formatImpl` to *insert* blank lines on certain transitions -- currently it only ever removes/collapses blank lines (see the `consecutiveBlanks > 1` guard); that's a new capability
  - be careful not to flag `**Bold**` at the start of a list item, which is normal style
  - the `reformat-md-2sp` port already inserts blank lines around block elements and before paragraphs that start with a `**Bold:**` label, which is a model for that capability; but it only splits mid-paragraph at `**Bold` boundaries on lines over 400 characters, so short cases get joined rather than separated

- markdown lint/fix: bullet lists immediately followed by prose with no blank line -- CommonMark parsers absorb the prose into the last list item instead of starting a new paragraph
  - detection: a line where `isBullet` is true, followed immediately (no blank) by a non-bullet, non-blank, non-indented line
  - same blank-line insertion capability needed as the bold-note issue above
  - exception: continuation lines indented to list depth (2-space soft-break continuations) must not be flagged -- use `leadingSpaces` to check
  - confirmed with the `reformat-md-2sp` port: it merges the prose into the last item (`- bullet two Prose right after the list`) because goldmark treats it as lazy continuation, so detection has to run on raw lines before parsing, as described above

- markdown lint: multiple consecutive lines of pure inline-code spans (`foo\nbar`) that should probably be a fenced code block
  - weakest of the four -- high false-positive risk (legitimate adjacent inline-code mentions in list items, etc.)
  - probably not worth adding until we have a real corpus example that motivated the rule; park for now
  - the `reformat-md-2sp` port joins such lines into a single line of adjacent code spans, hiding the problem rather than flagging it

- for Go files, we should apply `gofmt` (with `-s` if we're doing `--tidy`)
  - stdlib `go/format` covers plain gofmt (its docs warn that the output changes between Go versions), but the `-s` simplification lives in `package main` under `cmd/gofmt` and isn't importable
  - this should apply to "go" fence blocks in markdown too

- colored diff output?

- what might it look like to update show-diffs to have some way to diff output against output.tidy?  suggested flag names?  maybe this is actually what --tidy should mean?
  - today `show-diffs.sh --tidy` diffs `input` against `output.tidy`, and plain mode diffs `input` against `output`; there's no `output` vs `output.tidy` mode

- GHA -- add CI for this repo (there's no `.github/` yet), sourcing Tianon's style from the corpus (`docs/yaml.md`), of course
  - what do we include?
  - coverage diffing (gasp, do we have clod implement it??): upload coverage as an artifact on every `main` run; on PRs, download the latest `main` coverage artifact and post a Codecov-style coverage diff as a PR comment that gets updated in place (including on force pushes) instead of a new comment each time
  - details to settle when implementing: what to do when `main` has no usable artifact (none yet, or expired), whether to diff total coverage or per package/function, and the `pull-requests: write` permission plus how the comment gets found again for updating

- would it be too much scope creep to add a minifier mode?  (using our AST parsers to then remove anything that's "fluff")
  - we could then use this to find edge cases in how we choose to render things
  - however often our choices are based on "leave it how it was" so that's not exactly perfect
