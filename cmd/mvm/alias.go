package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MarcoTribuz/mvm/internal/platform"
	"github.com/MarcoTribuz/mvm/internal/resolve"
	"github.com/MarcoTribuz/mvm/internal/source"
	"github.com/MarcoTribuz/mvm/internal/store"
)

// Built-in aliases, accepted wherever a version is.
const (
	aliasLatest  = "latest"  // newest stable release published for this platform
	aliasDefault = "default" // the release set with `mvm use`
)

// Alias names start with a letter so they never look like a version; "v1"
// style names are refused because Normalize would strip the "v".
var (
	aliasNameRe   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]*$`)
	vVersionStyle = regexp.MustCompile(`^[vV]\d`)
)

func validAliasName(name string) error {
	switch {
	case name == aliasLatest || name == aliasDefault:
		return fmt.Errorf("%q is a built-in alias", name)
	case !aliasNameRe.MatchString(name), vVersionStyle.MatchString(name):
		return fmt.Errorf("invalid alias name %q (start with a letter, then letters, digits, '.', '_' or '-')", name)
	}
	return nil
}

// expandVersion turns a version or alias into a concrete release version.
func expandVersion(ctx context.Context, s *store.Store, v string) (string, error) {
	v = strings.TrimSpace(v)
	switch v {
	case aliasLatest:
		return latestRelease(ctx, s)
	case aliasDefault:
		if d := s.Default(); d != "" {
			return d, nil
		}
		return "", fmt.Errorf("no default release set (use `mvm use <version>`)")
	}
	if a := s.Alias(v); a != "" {
		return a, nil
	}
	return resolve.Normalize(v), nil
}

// latestRelease returns the newest stable release published for this host.
func latestRelease(ctx context.Context, s *store.Store) (string, error) {
	arch, err := platform.Arch()
	if err != nil {
		return "", err
	}
	vs, err := source.Remote(ctx, s, false)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", aliasLatest, err)
	}
	vs = source.Filter(vs, false)
	for i := len(vs) - 1; i >= 0; i-- {
		if platform.Supported(vs[i], arch) == nil {
			return vs[i], nil
		}
	}
	return "", fmt.Errorf("resolving %q: no release published for %s", aliasLatest, arch)
}

func aliasCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "alias [name [version]]",
		Short: "List, show or set version aliases",
		Long: "Aliases name a release, e.g. `mvm alias work 3.1.2` then `mvm exec work -- meteor`.\n" +
			"They work wherever a version does, including .mvmrc and MVM_METEOR_VERSION.\n" +
			"Built-in: \"latest\" (newest stable release) and \"default\" (set with `mvm use`).",
		Example: "  mvm alias              # list aliases\n" +
			"  mvm alias legacy 2.16\n" +
			"  mvm alias legacy        # print its version",
		Args:              cobra.MaximumNArgs(2),
		ValidArgsFunction: completeVersions(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch len(args) {
			case 0:
				all, err := s.Aliases()
				if err != nil {
					return err
				}
				names := make([]string, 0, len(all))
				for n := range all {
					names = append(names, n)
				}
				sort.Strings(names)
				if d := s.Default(); d != "" {
					fmt.Fprintf(out, "%s -> %s%s\n", aliasDefault, d, installedNote(s, d))
				}
				for _, n := range names {
					fmt.Fprintf(out, "%s -> %s%s\n", n, all[n], installedNote(s, all[n]))
				}
				return nil
			case 1:
				v, err := expandVersion(cmd.Context(), s, args[0])
				if err != nil {
					return err
				}
				if v == resolve.Normalize(args[0]) {
					return fmt.Errorf("alias %q does not exist", args[0])
				}
				fmt.Fprintln(out, v)
				return nil
			}
			name := args[0]
			if err := validAliasName(name); err != nil {
				return err
			}
			v, err := expandVersion(cmd.Context(), s, args[1])
			if err != nil {
				return err
			}
			if v == name {
				return fmt.Errorf("alias %q cannot point to itself", name)
			}
			if err := s.SetAlias(name, v); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s -> %s%s\n", name, v, installedNote(s, v))
			return nil
		},
	}
}

func unaliasCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unalias <name...>",
		Short: "Remove version aliases",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			s, err := openStore()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			all, _ := s.Aliases()
			names := make([]string, 0, len(all))
			for n := range all {
				names = append(names, n)
			}
			sort.Strings(names)
			return names, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			for _, n := range args {
				if err := s.RemoveAlias(n); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed alias %s\n", n)
			}
			return nil
		},
	}
}

func installedNote(s *store.Store, v string) string {
	if s.IsInstalled(v) {
		return ""
	}
	return " (not installed)"
}

// aliasesFor returns the alias names pointing at v, "default" first.
func aliasesFor(s *store.Store, v string, all map[string]string) []string {
	var names []string
	for n, av := range all {
		if av == v {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if s.Default() == v {
		names = slices.Insert(names, 0, aliasDefault)
	}
	return names
}

// completeVersions completes the first argument with installed releases and
// aliases; with remote set, it offers published releases instead (from the
// cached list when fresh).
func completeVersions(remote bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 && cmd.Name() != "install" && cmd.Name() != "uninstall" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		s, err := openStore()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		out := []string{aliasLatest}
		if remote {
			if vs, err := source.Remote(cmd.Context(), s, false); err == nil {
				out = append(out, source.Filter(vs, false)...)
			}
		} else {
			vs, _ := s.Installed()
			out = append(out, vs...)
			if s.Default() != "" {
				out = append(out, aliasDefault)
			}
		}
		all, _ := s.Aliases()
		for n := range all {
			out = append(out, n)
		}
		return out, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
	}
}
