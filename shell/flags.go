package shell

import (
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shortFlagSpec describes a short CLI flag and its long-form equivalent.
type shortFlagSpec struct {
	long      string // long-form flag, e.g. "--null-input"
	hasArg    bool   // true if this flag consumes the next argument
	canonical bool   // short form is a well-known idiom; preserved in default tidy
	pairWith  string // each char must also be present in the same combined-flag group
	//                 for this flag's short form to be considered canonical
	//                 (e.g. curl -s needs S, f, and L → pairWith: "SfL")
}

// cmdCanonicalSpec defines canonical ordering, merging, and invariant rules for
// a command's flags, applied in default (non-strict) tidy mode.
type cmdCanonicalSpec struct {
	// order is the canonical left-to-right char ordering for short flags.
	order string

	// merge lists boolean short flags that should be combined into a single
	// arg when they appear as separate single-char args and form a valid
	// canonical group.  e.g. "fsSL" for curl: -f -s -S -L → -fsSL.
	merge string

	// priority lists chars that should appear before any --long-flag args.
	// e.g. "vq" for grep: grep --extended-regexp -v → grep -v --extended-regexp.
	priority string

	// longFirst lists long flags that must be present and must appear as the
	// first argument(s) after the command name.  If a flag is absent it is
	// inserted; if present but not first it is moved.  e.g. --batch for gpg.
	longFirst []string

	// longPairs maps each long flag to the peer that must also be present.
	// Applied in both format and tidy mode.
	// e.g. curl --show-error requires --silent (almost always a typo without it).
	longPairs map[string]string

	// longPairsTidy is like longPairs but applied only in tidy mode.
	// Used for pairings that represent larger semantic changes.
	// e.g. curl --silent → add --show-error (tidy adds stderr output, format doesn't).
	longPairsTidy map[string]string

	// required lists short-char flags that must always be present.  They are
	// injected as separate args BEFORE the pre-merge phase so the merge step
	// can fold them into canonical combined groups (e.g. adding "f" to a call
	// that has "-sSL" produces "-fsSL" after merge).
	required string

	// preMergeLong maps long-form flag strings to their canonical short char.
	// Runs in phase 3c (AFTER longPairs) so enforceLongPairs can still find
	// flags by long name for insertion-position lookup.  A second preMerge
	// pass (phase 3d) then folds the results into the canonical combined group.
	//
	// preMergeLongFormat applies in both format and tidy mode — these are
	// correctness-level normalizations (e.g. --silent↔--show-error always
	// paired and always short).
	//
	// preMergeLongTidy applies only in tidy mode — these are opinionated
	// style choices (e.g. --fail → -f, --location → -L).
	preMergeLongFormat map[string]string
	preMergeLongTidy   map[string]string
}

// cmdFlagTables maps command names to their short→long flag tables.
//
// Flags marked canonical:true are kept in their short form by default tidy.
// A strict pass (normalizeShortFlags with strict=true) expands even canonical flags.
//
// pairWith marks flags that are only canonical when all listed peers are also
// present in the same combined-flag group.
var cmdFlagTables = map[string]map[string]shortFlagSpec{
	// jq: always normalize — long forms are documented style (see docs/jq-sh.md).
	"jq": {
		"n": {long: "--null-input"},
		"r": {long: "--raw-output"},
		"c": {long: "--compact-output"},
		"e": {long: "--exit-status"},
		"j": {long: "--join-output"},
		"R": {long: "--raw-input"},
		"s": {long: "--slurp"},
		"S": {long: "--sort-keys"},
		"M": {long: "--monochrome-output"},
		"C": {long: "--color-output"},
		"a": {long: "--ascii-output"},
		"f": {long: "--from-file", hasArg: true},
		"L": {long: "--library-path", hasArg: true},
	},

	// curl: -fsSL and -fL are canonical idioms.  -s and -S are only canonical as
	// part of the -fsSL mnemonic: both must be present together with -f and -L.
	// -o (output to file) is acceptable short form; -O (remote name) is not.
	"curl": {
		"f": {long: "--fail", canonical: true},
		"L": {long: "--location", canonical: true},
		"o": {long: "--output", hasArg: true, canonical: true},
		"s": {long: "--silent", canonical: true, pairWith: "S"}, // canonical when -S is present
		"S": {long: "--show-error", canonical: true, pairWith: "s"}, // canonical when -s is present
		"O": {long: "--remote-name"},
		"v": {long: "--verbose"},
		"k": {long: "--insecure"},
		"I": {long: "--head"},
		"X": {long: "--request", hasArg: true},
		"H": {long: "--header", hasArg: true},
		"d": {long: "--data", hasArg: true},
		"u": {long: "--user", hasArg: true},
		"A": {long: "--user-agent", hasArg: true},
	},

	// wget: -q and -O are universally recognised idioms.
	"wget": {
		"q": {long: "--quiet", canonical: true},
		"O": {long: "--output-document", hasArg: true, canonical: true},
	},

	// grep: the five flags below are ubiquitous short-form idioms; the rest
	// should be long for clarity.
	"grep": {
		"v": {long: "--invert-match", canonical: true},
		"E": {long: "--extended-regexp", canonical: true},
		"q": {long: "--quiet", canonical: true},
		"F": {long: "--fixed-strings", canonical: true},
		"o": {long: "--only-matching", canonical: true},
		"P": {long: "--perl-regexp"},
		"r": {long: "--recursive"},
		"R": {long: "--dereference-recursive"},
		"n": {long: "--line-number"},
		"i": {long: "--ignore-case"},
		"l": {long: "--files-with-matches"},
		"L": {long: "--files-without-match"},
		"h": {long: "--no-filename"},
		"H": {long: "--with-filename"},
		"w": {long: "--word-regexp"},
		"x": {long: "--line-regexp"},
		"z": {long: "--null-data"},
		"c": {long: "--count"},
		"e": {long: "--regexp", hasArg: true},
		"f": {long: "--file", hasArg: true},
		"A": {long: "--after-context", hasArg: true},
		"B": {long: "--before-context", hasArg: true},
		"C": {long: "--context", hasArg: true},
		"m": {long: "--max-count", hasArg: true},
	},

	// sed: -r (or -E), -i, and -e are idiomatic short forms; others are long.
	"sed": {
		"r": {long: "--regexp-extended", canonical: true}, // GNU sed
		"i": {long: "--in-place", canonical: true},
		"e": {long: "--expression", hasArg: true, canonical: true},
		"n": {long: "--quiet"},
		"E": {long: "--regexp-extended"}, // POSIX alias; -r preferred
		"u": {long: "--unbuffered"},
		"s": {long: "--separate"},
		"z": {long: "--null-data"},
		"f": {long: "--file", hasArg: true},
	},

	// sort: -u, -r, -V, -n are universally recognised; the rest are long.
	"sort": {
		"u": {long: "--unique", canonical: true},
		"r": {long: "--reverse", canonical: true},
		"V": {long: "--version-sort", canonical: true},
		"n": {long: "--numeric-sort", canonical: true},
		"d": {long: "--dictionary-order"},
		"f": {long: "--ignore-case"},
		"b": {long: "--ignore-leading-blanks"},
		"k": {long: "--key", hasArg: true},
		"t": {long: "--field-separator", hasArg: true},
	},

	// gpg/gpg2: corpus uses long forms throughout; normalise all short flags.
	"gpg": {
		"a": {long: "--armor"},
		"b": {long: "--detach-sign"},
		"q": {long: "--quiet"},
		"v": {long: "--verbose"},
		"k": {long: "--list-keys"},
		"K": {long: "--list-secret-keys"},
		"s": {long: "--sign"},
		"e": {long: "--encrypt"},
		"d": {long: "--decrypt"},
		"u": {long: "--local-user", hasArg: true},
		"o": {long: "--output", hasArg: true},
	},

	// dpkg/dpkg-query: corpus uses long forms; normalise all short flags.
	"dpkg": {
		"i": {long: "--install", hasArg: true},
		"r": {long: "--remove", hasArg: true},
		"P": {long: "--purge", hasArg: true},
		"l": {long: "--list"},
		"s": {long: "--status", hasArg: true},
		"L": {long: "--listfiles", hasArg: true},
		"S": {long: "--search", hasArg: true},
	},
	"dpkg-query": {
		"l": {long: "--list"},
		"s": {long: "--status", hasArg: true},
		"L": {long: "--listfiles", hasArg: true},
		"S": {long: "--search", hasArg: true},
		"W": {long: "--show"},
	},

	// apt-get: -y is the canonical short form (the long --yes is unusual in the
	// corpus); see also longToShortTables below for the --yes → -y reverse pass.
	"apt-get": {
		"y": {long: "--yes", canonical: true},
		"q": {long: "--quiet"},
	},
}

// cmdCanonicalSpecs defines per-command ordering, merging, and invariant rules.
var cmdCanonicalSpecs = map[string]cmdCanonicalSpec{
	// curl: canonical order f,s,S,L,o; canonical flags merged.
	//
	// Format-mode (correctness): --silent and --show-error are always paired
	// and always short; using --silent without --show-error silently hides
	// errors, which is dangerous.
	//
	// Tidy-mode (idiomatic): -f (--fail) is always added; --fail, --location,
	// and --output are normalised to their short forms and folded in.
	"curl": {
		order:    "fsSLo",
		merge:    "fsSL",
		required: "f",
		// Format: -S without -s is almost always a typo; add -s.
		longPairs: map[string]string{
			"--show-error": "--silent",
		},
		// Tidy: -s without -S adds stderr noise; add -S (larger semantic change).
		longPairsTidy: map[string]string{
			"--silent": "--show-error",
		},
		preMergeLongFormat: map[string]string{
			"--silent":     "s",
			"--show-error": "S",
		},
		preMergeLongTidy: map[string]string{
			"--fail":     "f",
			"--location": "L",
			"--output":   "o",
		},
	},

	// grep: -v (invert) is highest priority (changes meaning), then -q (silent,
	// footguns); all canonical boolean flags should be merged; canonical order: v,q,o,E,F,P.
	"grep": {order: "vqoEFP", merge: "vqoEFP", priority: "vq"},

	// gpg/gpg2: --batch must always be present and must be the first argument.
	"gpg": {longFirst: []string{"--batch"}},
}

// longToShortTables maps command names to long-flag → preferred-short-flag
// substitutions applied in non-strict mode.
var longToShortTables = map[string]map[string]string{
	"apt-get": {
		"--yes":        "-y",
		"--assume-yes": "-y",
	},
}

func init() {
	cmdFlagTables["gpg1"] = cmdFlagTables["gpg"]
	cmdFlagTables["gpg2"] = cmdFlagTables["gpg"]
	cmdCanonicalSpecs["gpg1"] = cmdCanonicalSpecs["gpg"]
	cmdCanonicalSpecs["gpg2"] = cmdCanonicalSpecs["gpg"]
}

// normalizeShortFlags rewrites CLI flags in call according to the per-command
// tables, in three phases:
//
//  1. Pre-merge: separate single-char boolean flags that form a canonical group
//     are merged into a combined arg in canonical order (e.g. -f -s -S -L → -fsSL).
//  2. Per-arg: non-canonical short flags are expanded to long form; long flags in
//     longToShortTables are converted to their preferred short form.
//  3. Post-normalize: combined groups are reordered by canonical order; priority
//     flags are moved before any --long-flag args.
//
// In strict mode only phase 2 runs (no merging or reordering).
// Processing stops at a bare "--".
func normalizeShortFlags(call *syntax.CallExpr, strict, tidy bool) {
	if len(call.Args) == 0 {
		return
	}
	cmd := wordLit(call.Args[0])
	table, ok := cmdFlagTables[cmd]
	if !ok {
		return
	}
	longTable := longToShortTables[cmd]

	spec := cmdCanonicalSpecs[cmd] // zero value if not present

	// Phase 0 (tidy-only): inject required short-char flags before the merge
	// phase so they are folded into canonical combined groups.
	// e.g. "f" for curl: a call missing -f gets it added here, then
	// preMergeFlags folds it into -fsSL if the other merge chars are present.
	if tidy && !strict && spec.required != "" {
		for i := 0; i < len(spec.required); i++ {
			char := string(spec.required[i])
			entry := table[char]
			if !longFlagPresent(call.Args, entry.long, char) {
				call.Args = append(call.Args[:1],
					append([]*syntax.Word{makeLitWord("-" + char)}, call.Args[1:]...)...)
			}
		}
	}

	// Phase 1: merge separate canonical boolean flags into combined form (tidy only).
	// In format mode, only the second merge pass (Phase 3d) runs, folding flags
	// newly inserted/shortened by the longPairs + preMergeLong phases.  Phase 1
	// is suppressed in format mode to avoid merging flags the user wrote as
	// separate args (e.g. grep -v -E), which is a layout-level change.
	if tidy && !strict && spec.merge != "" {
		preMergeFlags(call, table, spec)
	}

	// Phase 2: per-arg normalization (tidy only — format mode preserves short flags).
	if tidy {
		newArgs := make([]*syntax.Word, 0, len(call.Args)+4)
		newArgs = append(newArgs, call.Args[0])

		for i := 1; i < len(call.Args); i++ {
			word := call.Args[i]
			val := wordLit(word)

			if val == "--" {
				newArgs = append(newArgs, call.Args[i:]...)
				break
			}

			// Reverse normalization: long → preferred short (non-strict only).
			if !strict && longTable != nil {
				if preferred, ok := longTable[val]; ok {
					word.Parts[0].(*syntax.Lit).Value = preferred
					newArgs = append(newArgs, word)
					continue
				}
			}

			if !isShortFlag(val) {
				newArgs = append(newArgs, word)
				continue
			}

			flags := val[1:]

			if len(flags) == 1 {
				spec2, ok := table[flags]
				if !ok {
					newArgs = append(newArgs, word)
					continue
				}
				if !strict && spec2.canonical && spec2.pairWith == "" {
					newArgs = append(newArgs, word)
					continue
				}
				word.Parts[0].(*syntax.Lit).Value = spec2.long
				newArgs = append(newArgs, word)
				continue
			}

			expanded := expandCombinedFlags(flags, table, strict)
			if expanded == nil {
				newArgs = append(newArgs, word)
				continue
			}
			newArgs = append(newArgs, expanded...)
		}
		call.Args = newArgs
	}

	// Phase 3: reorder within combined groups + invariant enforcement.
	if !strict {
		if spec.order != "" {
			reorderCombinedGroups(call, table, spec)
		}
		if spec.priority != "" {
			movePriorityFlags(call, spec)
		}
		if tidy && len(spec.longFirst) > 0 {
			ensureLongFirstFlags(call, spec)
		}
		if len(spec.longPairs) > 0 {
			enforceLongPairs(call, table, spec)
		}
		if tidy && len(spec.longPairsTidy) > 0 {
			enforceLongPairs(call, table, cmdCanonicalSpec{longPairs: spec.longPairsTidy})
		}
		// Phase 3c: convert canonical long flags to short form, including any
		// long-form peers inserted by longPairs.  Runs after longPairs so that
		// enforceLongPairs can still locate flags by their long name.
		for val, shortChar := range spec.preMergeLongFormat {
			for i := 1; i < len(call.Args); i++ {
				if wordLit(call.Args[i]) == val {
					call.Args[i].Parts[0].(*syntax.Lit).Value = "-" + shortChar
				}
			}
		}
		if tidy {
			for val, shortChar := range spec.preMergeLongTidy {
				for i := 1; i < len(call.Args); i++ {
					if wordLit(call.Args[i]) == val {
						call.Args[i].Parts[0].(*syntax.Lit).Value = "-" + shortChar
					}
				}
			}
		}
		// Phase 3d: second pre-merge — fold newly-shortened flags (from phase 3c)
		// into the canonical combined group.  At this point longPairs have been
		// enforced so pairWith constraints are expected to be satisfied.
		// In format mode, only run when preMergeLongFormat is non-empty — otherwise
		// there is nothing to fold and the merge would combine flags the user wrote
		// as separate args (e.g. grep -v -E → grep -vE is a tidy-only change).
		if spec.merge != "" && (tidy || len(spec.preMergeLongFormat) > 0) {
			preMergeFlags(call, table, spec)
		}
	}
}

// preMergeFlags collects all canonical boolean merge-group chars from both
// single-char flag args AND combined flag args, then merges them into a single
// combined arg in canonical order.  This runs before per-arg expansion so that
// pairWith constraints are evaluated on the full merged set.
//
// Example: grep -oE -v → grep -voE (extracts o,E from combined -oE and merges with -v).
// Example: curl -f -s -S -L url → curl -fsSL url (four separate → one combined).
//
// When a combined arg contributes some chars to the merge (but has other chars
// that are not in the merge group), those non-merge chars are left as a residual
// arg; they will be expanded to long form in the per-arg phase.
func preMergeFlags(call *syntax.CallExpr, table map[string]shortFlagSpec, spec cmdCanonicalSpec) {
	// Collect (char, argIdx) for each merge-group boolean char, deduplicating by char
	// so each char is claimed by exactly the first arg that contains it.
	type source struct{ char byte; argIdx int }
	var sources []source
	seen := make(map[byte]bool)

	for i := 1; i < len(call.Args); i++ {
		val := wordLit(call.Args[i])
		if val == "--" {
			break
		}
		if !isShortFlag(val) {
			continue
		}
		for j := 1; j < len(val); j++ {
			c := val[j]
			if seen[c] || !strings.ContainsRune(spec.merge, rune(c)) {
				continue
			}
			fs, ok := table[string(c)]
			if !ok || fs.hasArg {
				continue
			}
			seen[c] = true
			sources = append(sources, source{c, i})
		}
	}

	if len(sources) < 2 {
		return
	}

	// Check canonical validity of the collected set.
	chars := make([]byte, len(sources))
	for i, s := range sources {
		chars[i] = s.char
	}
	if !isCombinedGroupCanonical(string(chars), table) {
		return
	}

	sorted := sortFlagsByOrder(string(chars), spec.order)

	// mergedSet: all chars being absorbed into the merged group.
	mergedSet := make(map[byte]bool, len(chars))
	for _, c := range chars {
		mergedSet[c] = true
	}

	// Insert position: before the first contributing arg.
	insertAt := sources[0].argIdx
	for _, s := range sources[1:] {
		if s.argIdx < insertAt {
			insertAt = s.argIdx
		}
	}

	// Collect contributing arg indices (descending) for safe in-place removal.
	contribSet := make(map[int]bool)
	for _, s := range sources {
		contribSet[s.argIdx] = true
	}
	descIdxs := make([]int, 0, len(contribSet))
	for idx := range contribSet {
		descIdxs = append(descIdxs, idx)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(descIdxs)))

	// Strip merged chars from every contributing arg.  If an arg has nothing
	// left, remove it entirely; otherwise update it to the residual chars.
	for _, idx := range descIdxs {
		val := wordLit(call.Args[idx])
		flags := val[1:]
		var remaining []byte
		for j := 0; j < len(flags); j++ {
			if !mergedSet[flags[j]] {
				remaining = append(remaining, flags[j])
			}
		}
		if len(remaining) == 0 {
			call.Args = append(call.Args[:idx], call.Args[idx+1:]...)
		} else {
			call.Args[idx].Parts[0].(*syntax.Lit).Value = "-" + string(remaining)
		}
	}

	// Insert the merged word at the insertion point.
	merged := makeLitWord("-" + sorted)
	call.Args = append(call.Args[:insertAt], append([]*syntax.Word{merged}, call.Args[insertAt:]...)...)
}

// reorderCombinedGroups sorts the chars within each canonical combined-flag word
// according to spec.order.  This is a fallback for commands that define an order
// but no merge group; for commands with a merge group, preMergeFlags already
// handles ordering as part of the merge step.
func reorderCombinedGroups(call *syntax.CallExpr, table map[string]shortFlagSpec, spec cmdCanonicalSpec) {
	for _, word := range call.Args[1:] {
		val := wordLit(word)
		if !isShortFlag(val) || len(val) < 3 {
			continue
		}
		flags := val[1:]
		if !isCombinedGroupCanonical(flags, table) {
			continue
		}
		sorted := sortFlagsByOrder(flags, spec.order)
		if sorted != flags {
			word.Parts[0].(*syntax.Lit).Value = "-" + sorted
		}
	}
}

// movePriorityFlags bubbles single-char priority flags (spec.priority) forward
// past any --long-flag args that precede them.  e.g.:
//
//	grep --extended-regexp -v → grep -v --extended-regexp
func movePriorityFlags(call *syntax.CallExpr, spec cmdCanonicalSpec) {
	// Collect positions of single-char priority flags that follow a long flag.
	// Repeat until stable (handles multiple priority flags out of order).
	changed := true
	for changed {
		changed = false
		for i := 2; i < len(call.Args); i++ {
			val := wordLit(call.Args[i])
			if len(val) != 2 || !isShortFlag(val) {
				continue
			}
			if !strings.ContainsRune(spec.priority, rune(val[1])) {
				continue
			}
			// Find the leftmost long flag at a lower index.
			firstLong := -1
			for j := 1; j < i; j++ {
				if v := wordLit(call.Args[j]); strings.HasPrefix(v, "--") {
					firstLong = j
					break
				}
			}
			if firstLong < 0 {
				continue
			}
			// Bubble the priority flag left to before firstLong.
			flag := call.Args[i]
			copy(call.Args[firstLong+1:i+1], call.Args[firstLong:i])
			call.Args[firstLong] = flag
			changed = true
			break
		}
	}
}

// ensureLongFirstFlags ensures each flag listed in spec.longFirst is present
// in the invocation and appears as the first argument(s).  If a flag is absent
// it is inserted at position 1; if present but not first it is moved there.
// Flags are processed in reverse order so the first entry ends up at position 1.
func ensureLongFirstFlags(call *syntax.CallExpr, spec cmdCanonicalSpec) {
	for i := len(spec.longFirst) - 1; i >= 0; i-- {
		flag := spec.longFirst[i]
		found := -1
		for j := 1; j < len(call.Args); j++ {
			if wordLit(call.Args[j]) == flag {
				found = j
				break
			}
		}
		switch {
		case found == -1:
			// Absent: insert at position 1.
			call.Args = append(call.Args[:1], append([]*syntax.Word{makeLitWord(flag)}, call.Args[1:]...)...)
		case found != 1:
			// Present but not first: bubble to position 1.
			w := call.Args[found]
			copy(call.Args[2:found+1], call.Args[1:found])
			call.Args[1] = w
		}
	}
}

// enforceLongPairs ensures that whenever a long flag from spec.longPairs is
// present, its mapped peer is also present.  If the peer is missing it is
// inserted immediately after the flag that triggered the requirement.
// Presence is checked against both standalone long-form args and short-form
// chars within canonical combined groups.
func enforceLongPairs(call *syntax.CallExpr, table map[string]shortFlagSpec, spec cmdCanonicalSpec) {
	// Build a reverse lookup: long form → short char (for combined-group detection).
	longToShort := make(map[string]string, len(table))
	for short, fs := range table {
		if fs.long != "" {
			longToShort[fs.long] = short
		}
	}

	for flag, peer := range spec.longPairs {
		if !longFlagPresent(call.Args, flag, longToShort[flag]) {
			continue
		}
		if longFlagPresent(call.Args, peer, longToShort[peer]) {
			continue
		}
		// Insert peer after the flag.  The flag may appear as its long form or
		// as a short char within a combined group (in format mode, Phase 2 does
		// not run, so short forms are not yet expanded to long).
		shortChar := longToShort[flag]
		for j := 1; j < len(call.Args); j++ {
			val := wordLit(call.Args[j])
			if val == flag || (shortChar != "" && isShortFlag(val) && strings.ContainsRune(val[1:], rune(shortChar[0]))) {
				call.Args = append(call.Args[:j+1], append([]*syntax.Word{makeLitWord(peer)}, call.Args[j+1:]...)...)
				break
			}
		}
	}
}

// longFlagPresent reports whether longFlag (e.g. "--silent") or its short-form
// equivalent (shortChar, e.g. "s") appears anywhere in args.  Short-form
// presence is detected within combined flag groups (e.g. -fsSL contains s).
func longFlagPresent(args []*syntax.Word, longFlag, shortChar string) bool {
	for _, w := range args[1:] { // skip command name
		val := wordLit(w)
		if val == longFlag {
			return true
		}
		if shortChar != "" && isShortFlag(val) && strings.ContainsRune(val[1:], rune(shortChar[0])) {
			return true
		}
	}
	return false
}

// isCombinedGroupCanonical reports whether all flags in the group are canonical
// and all pairWith constraints are satisfied within the group.
func isCombinedGroupCanonical(flags string, table map[string]shortFlagSpec) bool {
	for i := 0; i < len(flags); i++ {
		spec, ok := table[string(flags[i])]
		if !ok || !spec.canonical {
			return false
		}
		for _, peer := range spec.pairWith {
			found := false
			for j := 0; j < len(flags); j++ {
				if j != i && flags[j] == byte(peer) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}


// sortFlagsByOrder returns flags sorted by their position in order.
// Flags not in order are appended at the end in their original relative order.
func sortFlagsByOrder(flags, order string) string {
	type cp struct{ c byte; pri, pos int }
	chars := make([]cp, len(flags))
	for i := 0; i < len(flags); i++ {
		pri := strings.IndexByte(order, flags[i])
		if pri < 0 {
			pri = len(order)
		}
		chars[i] = cp{flags[i], pri, i}
	}
	sort.SliceStable(chars, func(i, j int) bool {
		if chars[i].pri != chars[j].pri {
			return chars[i].pri < chars[j].pri
		}
		return chars[i].pos < chars[j].pos
	})
	result := make([]byte, len(flags))
	for i, c := range chars {
		result[i] = c.c
	}
	return string(result)
}

// expandCombinedFlags returns expanded long-form words for a combined flag group,
// or nil if expansion should not happen (unknown flag, hasArg not last, or the
// group is already canonical in non-strict mode).
func expandCombinedFlags(flags string, table map[string]shortFlagSpec, strict bool) []*syntax.Word {
	for i := 0; i < len(flags); i++ {
		spec, ok := table[string(flags[i])]
		if !ok {
			return nil
		}
		if spec.hasArg && i != len(flags)-1 {
			return nil
		}
	}

	if !strict && isCombinedGroupCanonical(flags, table) {
		return nil // keep canonical short form (reorder happens separately)
	}

	words := make([]*syntax.Word, len(flags))
	for i := 0; i < len(flags); i++ {
		spec := table[string(flags[i])]
		words[i] = makeLitWord(spec.long)
	}
	return words
}

// isShortFlag reports whether s is a short flag: starts with "-", not "--",
// not a bare "-", and not a negative number.
func isShortFlag(s string) bool {
	return len(s) >= 2 && s[0] == '-' && s[1] != '-' && (s[1] < '0' || s[1] > '9')
}

// makeLitWord constructs a *syntax.Word containing a single *syntax.Lit.
func makeLitWord(s string) *syntax.Word {
	return &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: s}}}
}
