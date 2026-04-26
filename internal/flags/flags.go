// Package flags implements GNU-style command-line flag parsing.
//
// Rules:
//   - Long flags always require two dashes: --flag, --flag=value
//   - Short flags use one dash and one character: -f
//   - Anything after a bare -- is treated as a positional argument
//   - Single-dash long flags (-flag instead of --flag) are an error
//
// TODO: flesh this out more.  Currently only boolean flags are supported.
// Planned extensions:
//
//   - Optional-value string flags (OptString): --ast (use default) vs
//     --ast=format (use specified value) vs absent (not set).  This allows a
//     single flag to be both a boolean trigger and a mode selector, avoiding
//     an explosion of mutually-exclusive flags.  For example:
//
//       ast := fs.OptString("ast", 0, "input",
//           "dump AST as JSON; --ast=format to dump post-format AST instead")
//       // *ast == ""       → flag not given
//       // *ast == "input"  → --ast (no value; uses the default "input")
//       // *ast == "format" → --ast=format
//
//   - Required string arguments: --output=FILE or --output FILE.
//
//   - Enum validation, integer flags.
//
//   - Combined short flags: -wq equivalent to -w -q.
package flags

import (
	"fmt"
	"strings"
)

// FlagSet is a set of defined flags.
type FlagSet struct {
	name   string
	flags  []*boolFlag          // in registration order, for help text
	byLong map[string]*boolFlag // long name → flag
	byShort map[byte]*boolFlag  // short char → flag
}

type boolFlag struct {
	long  string
	short byte // 0 = no short form
	val   *bool
	usage string
}

// New returns a new FlagSet with the given program name (used in error messages).
func New(name string) *FlagSet {
	return &FlagSet{
		name:    name,
		byLong:  make(map[string]*boolFlag),
		byShort: make(map[byte]*boolFlag),
	}
}

// Bool registers a boolean flag.  long is the long name (without --); short is
// the single-character short name (without -), or 0 for no short form.
func (fs *FlagSet) Bool(long string, short byte, usage string) *bool {
	v := false
	f := &boolFlag{long: long, short: short, val: &v, usage: usage}
	fs.flags = append(fs.flags, f)
	fs.byLong[long] = f
	if short != 0 {
		fs.byShort[short] = f
	}
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
			// Everything after -- is positional.
			positional = append(positional, args[i+1:]...)
			return positional, nil

		case arg == "-" || !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)

		case strings.HasPrefix(arg, "--"):
			// Long flag: --name or --name=value
			rest := arg[2:]
			name, val, hasVal := strings.Cut(rest, "=")
			f, ok := fs.byLong[name]
			if !ok {
				return nil, fmt.Errorf("%s: unknown flag: %s", fs.name, arg)
			}
			if err := f.setbool(val, hasVal); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", fs.name, arg, err)
			}

		default:
			// Single-dash argument: must be a single character (-f) or an
			// attempt to use a long flag with one dash (-flag → error).
			rest := arg[1:]
			if len(rest) > 1 {
				// Could be -abc (stacked shorts, not yet supported) or -longflag.
				// Check if it's a known long flag name first so we give a good error.
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
			*f.val = true
		}
	}
	return positional, nil
}

func (f *boolFlag) setbool(val string, hasVal bool) error {
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
