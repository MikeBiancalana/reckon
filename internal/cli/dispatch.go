package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/pflag"
)

// errExtNotFound is returned by findExternal when no rk-<name> binary exists on PATH.
var errExtNotFound = errors.New("external rk subcommand not found")

// findExternal searches PATH for an executable file named "rk-<name>".
// Returns the absolute path on success.
// Returns errExtNotFound if no such file exists on PATH.
// Returns a distinct error (not errExtNotFound) if the file is found but not executable.
// Does NOT use exec.LookPath because that hides the non-executable case (EC-3).
func findExternal(name string) (string, error) {
	target := "rk-" + name
	pathEnv := os.Getenv("PATH")
	dirs := filepath.SplitList(pathEnv)
	for _, dir := range dirs {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, target)
		info, err := os.Stat(candidate)
		if err != nil {
			continue // not present in this dir
		}
		if info.Mode()&0111 == 0 {
			return "", fmt.Errorf("found %q but it is not executable (mode %s)", candidate, info.Mode())
		}
		return candidate, nil
	}
	return "", errExtNotFound
}

// firstNonFlag returns the first token in args that is not a flag name or a
// value consumed by a preceding value-taking flag. It consults the given
// FlagSet to distinguish bool flags (which do not consume the next token) from
// string/int/etc. flags (which do). Unknown flags are treated conservatively
// as bool (no value consumed). "--" terminates flag scanning.
func firstNonFlag(args []string, flags *pflag.FlagSet) string {
	skipNext := false
	pastDoubleDash := false
	for _, arg := range args {
		if pastDoubleDash {
			// Everything after "--" is positional, even if it looks like a flag.
			return arg
		}
		if skipNext {
			skipNext = false
			continue
		}
		// "--" ends flag parsing; everything after is positional.
		if arg == "--" {
			pastDoubleDash = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := arg[2:]
			if idx := strings.Index(name, "="); idx != -1 {
				// --flag=value form: value is embedded, no next token consumed
				continue
			}
			// --flag form: check whether it takes a value
			f := flags.Lookup(name)
			if f != nil && f.Value.Type() != "bool" {
				skipNext = true
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			// Short flag(s), e.g. -q or -qv
			// Inspect only the first character as the flag name
			shortName := string(arg[1])
			f := flags.ShorthandLookup(shortName)
			if f != nil && f.Value.Type() != "bool" {
				// -f VALUE form (only if no value glued: "-fVAL" is length > 2)
				if len(arg) == 2 {
					skipNext = true
				}
			}
			continue
		}
		// Positional argument
		return arg
	}
	return ""
}

// maybeDispatch checks whether args refers to an external rk-<verb> binary on
// PATH. It returns (true, nil) only via syscall.Exec (process replacement), so
// a successful dispatch never actually returns. It returns (false, nil) when
// args resolves to a builtin cobra command or when no external binary is found,
// allowing the caller to fall through to RootCmd.Execute(). It returns
// (true, err) when an external binary exists but cannot be executed.
func maybeDispatch(args []string) (bool, error) {
	// If cobra can find a matching builtin, let cobra handle it.
	target, _, _ := RootCmd.Find(args)
	if target != RootCmd {
		return false, nil
	}

	verb := firstNonFlag(args, RootCmd.Flags())
	if verb == "" || strings.HasPrefix(verb, "__") {
		return false, nil
	}

	path, lerr := findExternal(verb)
	switch {
	case errors.Is(lerr, errExtNotFound):
		// Not found → fall through to cobra (will print "unknown command")
		return false, nil
	case lerr != nil:
		return true, lerr
	}
	return true, execExternal(path, verb, args)
}

// execExternal replaces the current process with the external binary at path,
// forwarding all args (minus the first occurrence of verb). Returns only on
// failure (exec syscall error), wrapping with %w.
func execExternal(path, verb string, args []string) error {
	// Remove first occurrence of verb from the arg list so the child
	// receives the same args minus the dispatch token.
	filtered := make([]string, 0, len(args))
	removed := false
	for _, arg := range args {
		if !removed && arg == verb {
			removed = true
			continue
		}
		filtered = append(filtered, arg)
	}
	argv := append([]string{path}, filtered...)
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		return fmt.Errorf("exec %q: %w", path, err)
	}
	return nil // unreachable on success
}
