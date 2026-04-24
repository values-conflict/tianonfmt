# Perl style

Covers Perl scripts (`.pl`) and modules (`.pm`) in the corpus.  The corpus has a small but consistent set of Perl files in `corpus/doi/perl-bashbrew/`.

## Shebang

```perl
#!/usr/bin/env perl
```

Same pattern as [bash.md](bash.md) — always `env`-based, never a direct `/usr/bin/perl` path.

Corpus ref: `corpus/doi/perl-bashbrew/bin/put-multiarch.pl:1`.

## Module imports

The standard preamble for scripts:

```perl
use Mojo::Base -strict, -signatures;
```

This imports strict mode and the modern subroutine signatures feature via `Mojolicious`.

For modules (`.pm`):

```perl
package Bashbrew;
use Mojo::Base -base, -signatures;
```

`-base` sets up `Bashbrew` as a Mojolicious base class.  `-signatures` enables the signatures feature.

Additional `use` statements follow for other dependencies, one per line:

```perl
use Digest::SHA;
use Dpkg::Version;
use Getopt::Long;
use Mojo::Promise;
```

Corpus refs: `corpus/doi/perl-bashbrew/bin/put-multiarch.pl:2-11`, `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:2-6`.

## Indentation

**Hard tabs, one per nesting level.**  This matches the convention in shell and jq files.

## Subroutine definitions

The modern signatures syntax is used for named parameters:

```perl
sub arch_to_platform ($arch) {
    ...
}
```

```perl
sub get_arch_p ($targetRef, $arch, $archRef) {
    ...
}
```

For variadic subroutines (accepting any number of arguments), the `(@)` prototype syntax:

```perl
sub bashbrew (@) {
    open my $fh, '-|', 'bashbrew', @_ or die "...";
    ...
}
```

Opening brace `{` always on the same line as `sub`.

Corpus refs: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:13`, `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:48`, `corpus/doi/perl-bashbrew/bin/put-multiarch.pl:32`.

## `if` / `elsif` / `else`

```perl
if ($architecture eq 'i386') {
    $architecture = '386';
}
elsif ($architecture eq 'arm32') {
    $architecture = 'arm';
}
```

- `elsif` (not `elseif` or `else if`)
- Opening brace `{` on the same line as the keyword
- Closing brace `}` on its own line at the same indentation as `if`

## Regex matching

Extended regex (`/x` modifier) is used for complex patterns to allow whitespace and comments inside the pattern:

```perl
if ($arch =~ m{
    ^
    (?: ([^-]+) - )? # optional "os" prefix ("windows-", etc)
    ([^-]+?) # "architecture" bit ("arm64", "s390x", etc)
    (v[0-9]+)? # optional "variant" suffix ("v7", "v6", etc)
    $
}x) {
```

The `m{}` delimiter (braces) is used instead of `m//` when the pattern contains `/`.  Comments inside `/x` patterns explain what each group matches.

Corpus ref: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:14-20`.

## `defined-or` (`//=`, `//`)

The Perl 5.10+ defined-or operator is used for default values:

```perl
$os //= 'linux';
```

`//` (defined-or) is preferred over `||` when the distinction between defined and truthy matters.

Corpus ref: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:22`.

## `die` for errors

Errors are fatal and use `die`:

```perl
die "unrecognized architecture format in: $arch";
die "failed to run 'bashbrew': $!";
```

Error messages are lowercase, no trailing newline (Perl adds `at file line N` automatically).

Corpus refs: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:39`, `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:49`.

## `return` from subroutines

Explicit `return` is used for early returns:

```perl
return (
    os => $os,
    architecture => $architecture,
    ($variant ? (variant => $variant) : ()),
);
```

The last statement's value is returned implicitly only in simple cases.  For list returns, explicit parentheses are used.

## Module closing

Every `.pm` file ends with `1;`:

```perl
1;
```

Corpus ref: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:57`.

## `@EXPORT_OK` and `use Exporter`

```perl
use Exporter 'import';
our @EXPORT_OK = qw(
	arch_to_platform
	bashbrew
);
```

`qw()` is used for the export list.  Each symbol is on its own line when the list has more than one element, tab-indented.

Corpus ref: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:6-10`.

## Comments

Comments follow the `#` convention.  Multi-line comments in regexes use the `/x` modifier and inline `#` annotations.  Explanatory comments appear above complex logic blocks.

TODO comments follow the same pattern as in other languages:

```perl
# TODO create dedicated Bashbrew::Arch package?
# TODO make this promise-based and non-blocking?
```

Corpus ref: `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:12`, `corpus/doi/perl-bashbrew/lib/Bashbrew.pm:42-46`.

## Promise-based async (Mojo::Promise)

When using Mojo::Promise for async code, the `.then(sub (...) { ... })` pattern is used:

```perl
return $ua->get_manifest_p($archRef)->then(sub ($manifestData = undef) {
    ...
});
```

The sub keyword starts the callback, with signatures for parameters.  The default value `= undef` pattern is used for optional parameters.

Corpus ref: `corpus/doi/perl-bashbrew/bin/put-multiarch.pl:33`.

## Notable omissions

- `use strict; use warnings;` — replaced by `use Mojo::Base -strict`
- `use feature 'signatures';` — implied by `-signatures` in `Mojo::Base`
- Indirect object syntax (`new Foo ...`) — method call syntax (`Foo->new(...)`) is used
- `local $/` without a scope guard — used carefully in specific contexts
- `chomp` in a loop — used once at the end of captured output, not repeatedly
- Regex without `/x` for patterns longer than ~20 characters
- `die` with a trailing newline (that would suppress `at file line N` info)
