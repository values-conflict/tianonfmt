#!/usr/bin/env perl
# Reformat markdown files to use two-spaces-after-sentence-terminator convention.
# Joins wrapped prose paragraphs into single long lines, normalizing sentence
# spacing to exactly two spaces after each ., !, or ?.
#
# Derived from reformat-md.pl (one-sentence-per-line), which sits next to this script.
# The sentence-detection logic is identical; only the output strategy differs.
#
# Rules:
#   - Inside fenced code blocks (``` or ~~~): preserve verbatim.
#     Closing fence must use >= the same number of backticks as the opener.
#   - Lines that are: blank, headings (#), blockquotes (>), list item bullets,
#     table rows (|), or key-value front-matter (**Name:**) -> preserve verbatim.
#   - Indented continuation lines under list items are gathered into groups.
#     Groups already correctly formatted (every line ends with terminal punctuation)
#     are joined with two-space separators. Groups with mid-sentence wrapping are
#     joined greedily (join where the preceding line lacks terminal punctuation)
#     and re-emitted as a single line with two-space sentence separators.
#   - Plain prose lines form paragraphs that are joined into a single long line
#     with two spaces between sentences.
#
# Sentence boundary = ., !, or ? followed by a space or end-of-input,
#   with optional closing quote/bracket after, EXCEPT:
#     - preceded by a known abbreviation (e.g., "e.g.", "i.e.", "etc.")
#     - preceded by a single-letter initial like "U."
#     - part of a multi-period token like "v2.7.1"
#   - Inside `inline code` backticks and [link text](url) URLs, don't split.

use strict;
use warnings;

my @ABBREVS = (
    "e.g.", "i.e.", "etc.", "cf.", "vs.", "viz.",
    "Mr.", "Mrs.", "Ms.", "Dr.", "St.", "Jr.", "Sr.",
    "No.", "Nos.", "Vol.", "Vols.", "Ch.", "Chs.",
    "Fig.", "Figs.", "Eq.", "Eqs.", "Ref.", "Refs.",
    "Sec.", "Secs.",
    "a.m.", "p.m.", "A.M.", "P.M.",
    "approx.", "Inc.", "Co.", "Ltd.", "Corp.",
    "RFC.",
);
my %ABBREV = map { lc($_) => 1 } @ABBREVS;
my %NAME_ABBREV = map { lc($_) => 1 } qw(Mr. Mrs. Ms. Dr. St. Jr. Sr.);

sub parse_continuation {
    my ($line) = @_;
    return undef unless $line =~ /^( {2,})(.+)$/;
    my ($indent, $content) = ($1, $2);
    return undef if $content =~ /^[-*+] /;
    return undef if $content =~ /^\d+\. /;
    return undef if $content =~ /^[a-zA-Z]\. /;
    return undef if $content =~ /^>/;
    return undef if $content =~ /^(```+|~~~+)/;
    return undef if $content =~ /^\|/;
    return ($indent, $content);
}

sub is_prose_line {
    my ($line) = @_;
    return 0 if $line =~ /^\s/;
    return 0 if $line eq "";
    return 0 if $line =~ /^#/;
    return 0 if $line =~ /^>/;
    return 0 if $line =~ /^\|/;
    return 0 if $line =~ /^(```|~~~)/;
    return 0 if $line =~ /^([-*+]\s|\d+\.\s|[a-zA-Z]\.\s)/;
    return 0 if $line =~ /^\[[^\]]+\]:/;
    return 0 if $line =~ /^[-*_]{3,}\s*$/;
    return 0 if $line =~ /^!\[/;
    return 1;
}

sub looks_like_abbrev {
    my ($tok) = @_;
    return 1 if $tok =~ /^[A-Za-z]\.$/;
    return 1 if exists $ABBREV{lc($tok)};
    return 0;
}

sub ends_with_terminal {
    my ($s) = @_;
    $s =~ s/\s+$//;
    return 1 if $s eq '';
    return 1 if $s =~ /\*\*$/;
    return 1 if $s =~ /[!?)\]]$/;
    if ($s =~ /\.$/) {
        my $tail = $s;
        $tail =~ s/.*\s//;
        my $tok = $tail;
        $tok =~ s/^["'(\[]+//;
        return 0 if looks_like_abbrev($tok);
        return 1;
    }
    return 1 if $s =~ /[.!?]["'\)\]]+$/;
    return 0;
}

sub split_into_sentences {
    my ($text) = @_;
    $text =~ s/\s+/ /g;
    $text =~ s/^\s+//;
    $text =~ s/\s+$//;
    return () if $text eq "";

    my @sentences;
    my @chars = split //, $text;
    my $n = scalar @chars;
    my $i = 0;
    my $start = 0;
    my $in_code = 0;
    my $code_delim = 0;
    my $bracket_depth = 0;
    my $in_link_url = 0;
    my $paren_depth = 0;
    my $star_depth = 0;

    while ($i < $n) {
        my $c = $chars[$i];

        if ($c eq '`' && !$in_link_url) {
            if (!$in_code) {
                my $j = $i;
                $j++ while $j < $n && $chars[$j] eq '`';
                $code_delim = $j - $i;
                $in_code = 1;
                $i = $j;
            } else {
                my $j = $i;
                $j++ while $j < $n && $chars[$j] eq '`';
                if (($j - $i) == $code_delim) { $in_code = 0; $code_delim = 0; }
                $i = $j;
            }
            next;
        }
        if ($in_code) { $i++; next; }

        if ($c eq '[') { $bracket_depth++; $i++; next; }
        if ($c eq ']') {
            $bracket_depth-- if $bracket_depth > 0;
            if ($i + 1 < $n && $chars[$i+1] eq '(') {
                $in_link_url = 1; $paren_depth = 1; $i += 2; next;
            }
            $i++; next;
        }
        if ($in_link_url) {
            if    ($c eq '(') { $paren_depth++; }
            elsif ($c eq ')') { $paren_depth--; $in_link_url = 0 if $paren_depth == 0; }
            $i++; next;
        }

        if ($c eq '*') { $star_depth++; $i++; next; }

        if ($c eq '.' || $c eq '!' || $c eq '?') {
            if ($star_depth % 2 == 1) { $i++; next; }
            my $j = $i + 1;
            while ($j < $n && $chars[$j] =~ /["')\]]/) { $j++; }
            my $at_end        = ($j >= $n);
            my $next_is_space = ($j < $n && $chars[$j] eq ' ');
            if (!$at_end && !$next_is_space) { $i++; next; }

            if (($c eq '?' || $c eq '!') && $next_is_space && $j > $i + 1) {
                my $k = $j + 1;
                while ($k < $n && $chars[$k] eq ' ') { $k++; }
                if ($k < $n && $chars[$k] =~ /[a-z]/) { $i++; next; }
            }

            if ($c eq '.') {
                my $k = $i;
                while ($k > $start && $chars[$k-1] ne ' ') { $k--; }
                my $prev_token       = join('', @chars[$k..$i]);
                my $token_for_abbrev = $prev_token;
                $token_for_abbrev =~ s/^["'(\[]+//;
                my $body = $prev_token; chop $body;
                if ($body =~ /\./) {
                    my $plain = $body;
                    $plain =~ s/`[^`]*`//g;
                    $plain =~ s/\[[^\]]*\]//g;
                    $plain =~ s/<[^>]*>//g;
                    $plain =~ s/\([^)]*\)//g;
                    if ($plain =~ /\./) { $i = $j; next; }
                }
                if (looks_like_abbrev($token_for_abbrev)) {
                    my $is_abbrev = 1;
                    if ($token_for_abbrev =~ /^[A-Za-z]\.$/) {
                        my $prev_ch = ($k > $start + 1) ? $chars[$k - 2] : '.';
                        $is_abbrev = ($prev_ch eq '.');
                    }
                    elsif (!exists $NAME_ABBREV{lc($token_for_abbrev)}) {
                        my $after = $j;
                        $after++ while $after < $n && $chars[$after] eq ' ';
                        if ($after < $n && $chars[$after] =~ /[A-Z]/) {
                            my $end = $after + 1;
                            $end++ while $end < $n && $chars[$end] =~ /[A-Za-z]/;
                            my $next_word = join('', @chars[$after .. $end - 1]);
                            $is_abbrev = 0 if $next_word !~ /^[A-Z]{2,}$/;
                        }
                    }
                    $i = $j; next if $is_abbrev;
                }
            }

            my $sentence = join('', @chars[$start..$j-1]);
            $sentence =~ s/^\s+|\s+$//g;
            push @sentences, $sentence if length $sentence;
            while ($j < $n && $chars[$j] eq ' ') { $j++; }
            $start = $j; $i = $j;
            next;
        }
        $i++;
    }

    if ($start < $n) {
        my $tail = join('', @chars[$start..$n-1]);
        $tail =~ s/^\s+|\s+$//g;
        push @sentences, $tail if length $tail;
    }
    return @sentences;
}

# Join sentences with two spaces between them.
sub join_with_two_spaces {
    my (@sents) = @_;
    return join('  ', @sents);
}

sub reformat {
    my ($text) = @_;
    my $had_newline = ($text =~ /\n\z/);
    my @lines = split /\n/, $text, -1;
    if ($had_newline && @lines && $lines[-1] eq '') { pop @lines; }

    my @out;
    my $in_fence   = 0;
    my $fence_char = '';
    my $fence_min  = 0;
    my $i = 0;
    my $n = scalar @lines;

    # Pass YAML front matter through verbatim
    if ($i < $n && $lines[$i] =~ /^---\s*$/) {
        push @out, $lines[$i]; $i++;
        while ($i < $n) {
            push @out, $lines[$i];
            last if $lines[$i] =~ /^---\s*$/;
            $i++;
        }
        $i++;
    }

    OUTER: while ($i < $n) {
        my $line = $lines[$i];

        # ── Fence handling ───────────────────────────────────────────────────
        if (!$in_fence) {
            if ($line =~ /^\s*((`{3,}|~{3,}))/) {
                my $marker = $1;
                $fence_char = substr($marker, 0, 1);
                $fence_min  = length($marker);
                $in_fence = 1;
                push @out, $line; $i++; next;
            }
        } else {
            push @out, $line;
            if ($line =~ /^\s*(\Q$fence_char\E+)\s*$/ && length($1) >= $fence_min) {
                $in_fence = 0; $fence_char = ''; $fence_min = 0;
            }
            $i++; next;
        }

        # ── Plain prose lines ─────────────────────────────────────────────────
        if (is_prose_line($line)) {
            if ($line =~ /  $/) { push @out, $line; $i++; next; }
            my @para = ($line);
            my $j = $i + 1;
            while ($j < $n) {
                my $nl = $lines[$j];
                last if $nl =~ /^\s*$/;
                last if !is_prose_line($nl);
                last if $nl =~ /^\s*(```+|~~~+)/;
                push @para, $nl;
                $j++;
            }
            my $joined = join(' ', map { (my $s = $_) =~ s/^\s+|\s+$//g; $s } @para);
            my @sents  = split_into_sentences($joined);
            if (@sents) {
                push @out, join_with_two_spaces(@sents);
            } else {
                push @out, join(' ', @para);
            }
            $i = $j; next;
        }

        # ── Indented continuation lines ───────────────────────────────────────
        {
            my ($indent, $content) = parse_continuation($line);
            if (defined $indent) {
                my @para = ($content);
                my $j = $i + 1;
                unless ($line =~ /  $/) {
                    while ($j < $n) {
                        my $nl = $lines[$j];
                        last if $nl =~ /^\s*$/;
                        my ($ni, $nc) = parse_continuation($nl);
                        last unless defined $ni && $ni eq $indent;
                        push @para, $nc;
                        $j++;
                    }
                }

                if (@para == 1) {
                    push @out, $line;
                    $i++; next OUTER;
                }

                # Check if any line (except the last) lacks terminal punctuation.
                my $any_wrapped = 0;
                for my $k (0 .. $#para - 1) {
                    if (!ends_with_terminal($para[$k])) { $any_wrapped = 1; last; }
                }

                if (!$any_wrapped) {
                    # Already one-sentence-per-line -- join them with two spaces.
                    my $joined = join(' ', @para);
                    my @sents = split_into_sentences($joined);
                    if (@sents) {
                        push @out, $indent . join_with_two_spaces(@sents);
                    } else {
                        push @out, map { "$indent$_" } @para;
                    }
                    $i = $j; next OUTER;
                }

                # Greedy join then sentence-split.
                my @blocks;
                my $cur = $para[0];
                for my $k (1 .. $#para) {
                    if (!ends_with_terminal($cur)) {
                        $cur = $cur . ' ' . $para[$k];
                    } else {
                        push @blocks, $cur;
                        $cur = $para[$k];
                    }
                }
                push @blocks, $cur;

                my @result;
                for my $block (@blocks) {
                    my @sents = split_into_sentences($block);
                    push @result, (@sents ? @sents : $block);
                }
                push @out, $indent . join_with_two_spaces(@result);
                $i = $j; next OUTER;
            }
        }

        # ── List-item header overflow ─────────────────────────────────────────
        # Handles both top-level (^[-*+] ) and indented sub-list items.
        # Continuation threshold: any indent strictly deeper than the leading
        # whitespace before the marker (length($leading_ws) + 1), which is
        # looser than requiring full prefix-width alignment and correctly handles
        # files where continuation is indented by one level (2 spaces) rather
        # than aligned to the content column.
        if ($line =~ /^((\s*)(?:[-*+]|\d+\.)\s+)(.+)$/) {
            my ($prefix, $leading_ws, $content) = ($1, $2, $3);
            my $min_cont = length($leading_ws) + 1;
            {
                my @cont;
                my $j = $i + 1;
                unless ($line =~ /  $/) {
                    while ($j < $n) {
                        my $nl = $lines[$j];
                        last if $nl =~ /^\s*$/;
                        my ($ni, $nc) = parse_continuation($nl);
                        last unless defined $ni && length($ni) >= $min_cont;
                        push @cont, $nc;
                        $j++;
                    }
                }
                if (@cont) {
                    my $full = join(' ', $content, @cont);
                    $full =~ s/\s+/ /g;
                    my @sents = split_into_sentences($full);
                    push @out, $prefix . (@sents ? join_with_two_spaces(@sents) : $full);
                    $i = $j; next OUTER;
                }
            }
        }

        # ── Verbatim fallthrough ──────────────────────────────────────────────
        push @out, $line;
        $i++;
    }

    # Post-processing: structural whitespace normalization.
    # (a) Blank line before each top-level code fence (```/~~~).
    # (b) Blank line after each closing code fence.
    # (c) Blank line before each top-level unordered/ordered list item
    #     that directly follows a non-blank, non-list line.
    # (d) Collapse runs of 2+ consecutive blank lines to one.
    {
        my @p;
        my $fd = 0;   # fence depth (0 = not in fence)
        my $fc = '';  # fence character (` or ~)

        for my $i (0 .. $#out) {
            my $line = $out[$i];
            my $prev = @p ? $p[-1] : undef;

            if ($fd) {
                push @p, $line;
                if ($line =~ /^\s*(\Q$fc\E+)\s*$/ && length($1) >= $fd) {
                    $fd = 0; $fc = '';
                    my $next = ($i + 1 <= $#out) ? $out[$i + 1] : undef;
                    push @p, '' if defined($next) && $next ne '';
                }
                next;
            }

            if ($line =~ /^\s*(`{3,}|~{3,})/) {
                my $marker = $1;
                $fd = length($marker);
                $fc = substr($marker, 0, 1);
                push @p, '' if defined($prev) && $prev ne '';
                push @p, $line;
                next;
            }

            # Blank after a heading when the next line is non-blank.
            if (defined($prev) && $prev =~ /^#+/ && $line ne '') {
                push @p, '';
                $prev = '';
            }

            # Blank before top-level list item following a non-blank non-list line.
            # Adjacent list siblings separated only by an indented continuation note
            # must NOT have a blank between them -- remove one if already present.
            if ($line =~ /^[-*+] / || $line =~ /^\d+\. /) {
                if (@p >= 2 && $p[-1] eq '' && $p[-2] =~ /^\s/) {
                    pop @p;
                }
                $prev = @p ? $p[-1] : undef;
                if (defined($prev) && $prev ne ''
                        && $prev !~ /^[-*+] /
                        && $prev !~ /^\d+\. /
                        && $prev !~ /^\s/) {
                    push @p, '';
                }
            }

            push @p, $line;
        }

        # Collapse 2+ consecutive blank lines to 1.
        @out = ();
        my $nb = 0;
        for my $line (@p) {
            if ($line eq '') {
                push @out, '' if ++$nb == 1;
            } else {
                $nb = 0;
                push @out, $line;
            }
        }
    }

    my $result = join("\n", @out);
    $result .= "\n" if $had_newline;
    return $result;
}

for my $path (@ARGV) {
    open(my $fh, '<', $path) or die "cannot read $path: $!";
    local $/;
    my $content = <$fh>;
    close $fh;
    my $new = reformat($content);
    if ($new ne $content) {
        open(my $out, '>', $path) or die "cannot write $path: $!";
        print $out $new;
        close $out;
        print "Updated: $path\n";
    } else {
        print "No change: $path\n";
    }
}
