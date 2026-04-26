// Package flags implements GNU-style command-line flag parsing.
//
// Rules:
//   - Long flags always require two dashes: --flag, --flag=value
//   - Short flags use one dash and one character: -f
//   - Anything after a bare -- is treated as a positional argument
//   - Single-dash long flags (-flag instead of --flag) are an error
//
// TODO: flesh this out more.  Planned extensions:
//
//   - Required string arguments: --output=FILE or --output FILE.
//
//   - Enum validation, integer flags.
//
//   - Combined short flags: -wq equivalent to -w -q.
//
//   - Help text formatting / usage output.
package flags

import (
	"fmt"
	"strings"
)

// FlagSet is a set of defined flags.
type FlagSet struct {
	name    string
	byLong  map[string]flag
	byShort map[byte]flag
}

// flag is the internal interface for any registered flag.
type flag interface {
	longName() string
	setLong(val string, hasVal bool) error // called for --name or --name=val
	setShort() error                       // called for -f
}

// New returns a new FlagSet with the given program name (used in error messages).
func New(name string) *FlagSet {
	return &FlagSet{
		name:    name,
		byLong:  make(map[string]flag),
		byShort: make(map[byte]flag),
	}
}

func (fs *FlagSet) register(f flag, short byte) {
	fs.byLong[f.longName()] = f
	if short != 0 {
		fs.byShort[short] = f
	}
}

// Bool registers a boolean flag.  long is the long name (without --); short is
// the single-character short name (without -), or 0 for no short form.
func (fs *FlagSet) Bool(long string, short byte, usage string) *bool {
	v := false
	f := &boolFlag{long: long, val: &v, usage: usage}
	fs.register(f, short)
	return &v
}

// OptString registers an optional-value string flag.
//
// The flag has three states:
//
//	absent:         *result == ""   (flag not given at all)
//	--flag:         *result == def  (flag given with no value; uses the default)
//	--flag=value:   *result == value
//
// This allows a single flag to act as both a boolean trigger (--ast) and a mode
// selector (--ast=format), without an explosion of mutually-exclusive flags.
// Per GNU convention, the value MUST use = syntax; --flag value treats "value"
// as a positional argument, not the flag's value.
//
// Example:
//
//	ast := fs.OptString("ast", 0, "input",
//	    "dump AST as JSON; --ast=format to dump post-format AST instead")
//	// *ast == ""       → flag not given
//	// *ast == "input"  → --ast (no =value; uses the default "input")
//	// *ast == "format" → --ast=format
func (fs *FlagSet) OptString(long string, short byte, def, usage string) *string {
	v := ""
	f := &optStringFlag{long: long, val: &v, def: def, usage: usage}
	fs.register(f, short)
	return &v
}

// Parse processes args (typically os.Args[1:]) and returns the remaining
// positional arguments.  Unknown flags and misused single-dash long forms
// return an error.
func (fs *FlagSet) Parse(args []string) ([]string, error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			return positional, nil

		case arg == "-" || !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)

		case strings.HasPrefix(arg, "--"):
			rest := arg[2:]
			name, val, hasVal := strings.Cut(rest, "=")
			f, ok := fs.byLong[name]
			if !ok {
				return nil, fmt.Errorf("%s: unknown flag: %s", fs.name, arg)
			}
			if err := f.setLong(val, hasVal); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", fs.name, arg, err)
			}

		default:
			rest := arg[1:]
			if len(rest) > 1 {
				longName, _, _ := strings.Cut(rest, "=")
				if _, ok := fs.byLong[longName]; ok {
					return nil, fmt.Errorf("%s: use --%s, not %s (long flags require --)", fs.name, longName, arg)
				}
				return nil, fmt.Errorf("%s: unknown flag: %s (use -- for long flags)", fs.name, arg)
			}
			f, ok := fs.byShort[rest[0]]
			if !ok {
				return nil, fmt.Errorf("%s: unknown flag: %s", fs.name, arg)
			}
			if err := f.setShort(); err != nil {
				return nil, fmt.Errorf("%s: -%c: %w", fs.name, rest[0], err)
			}
		}
	}
	return positional, nil
}

// ── boolFlag ─────────────────────────────────────────────────────────────────

type boolFlag struct {
	long  string
	val   *bool
	usage string
}

func (f *boolFlag) longName() string { return f.long }

func (f *boolFlag) setLong(val string, hasVal bool) error {
	if !hasVal {
		*f.val = true
		return nil
	}
	switch strings.ToLower(val) {
	case "true", "1", "yes":
		*f.val = true
	case "false", "0", "no":
		*f.val = false
	default:
		return fmt.Errorf("invalid boolean value %q", val)
	}
	return nil
}

func (f *boolFlag) setShort() error { *f.val = true; return nil }

// ── optStringFlag ─────────────────────────────────────────────────────────────

type optStringFlag struct {
	long  string
	val   *string
	def   string
	usage string
}

func (f *optStringFlag) longName() string { return f.long }

func (f *optStringFlag) setLong(val string, hasVal bool) error {
	if !hasVal {
		*f.val = f.def
		return nil
	}
	*f.val = val
	return nil
}

func (f *optStringFlag) setShort() error { *f.val = f.def; return nil }
