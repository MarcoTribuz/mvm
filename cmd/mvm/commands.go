package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/MarcoTribuz/mvm/internal/install"
	"github.com/MarcoTribuz/mvm/internal/platform"
	"github.com/MarcoTribuz/mvm/internal/resolve"
	"github.com/MarcoTribuz/mvm/internal/run"
	"github.com/MarcoTribuz/mvm/internal/source"
	"github.com/MarcoTribuz/mvm/internal/store"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mvm",
		Short:         "Meteor version manager",
		Long:          "mvm installs several Meteor releases side by side and runs each command with the right one.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		installCmd(), uninstallCmd(), listCmd(), lsRemoteCmd(),
		useCmd(), currentCmd(), whichCmd(), execCmd(), envCmd(),
		pruneCmd(), doctorCmd(), initCmd(), versionCmd(),
	)
	return root
}

func openStore() (*store.Store, error) { return store.Open() }

func installCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "install [version...]",
		Short: "Install Meteor releases (default: the one the current project needs)",
		Example: "  mvm install 3.1.2 2.16\n" +
			"  mvm install            # reads .meteor/release",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			arch, err := platform.Arch()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				res, err := resolveVersion(s, "")
				if err != nil {
					return err
				}
				args = []string{res.Version}
			}
			for _, v := range args {
				v = resolve.Normalize(v)
				if !force && s.IsInstalled(v) {
					fmt.Fprintf(cmd.ErrOrStderr(), "mvm: meteor %s already installed\n", v)
					continue
				}
				if err := install.Install(cmd.Context(), s, v, arch, install.Options{Force: force, Log: cmd.ErrOrStderr()}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already installed")
	return cmd
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall <version...>",
		Aliases: []string{"rm"},
		Short:   "Remove installed Meteor releases",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			for _, v := range args {
				v = resolve.Normalize(v)
				if err := s.Remove(v); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed meteor %s\n", v)
			}
			return nil
		},
	}
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List installed Meteor releases",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			vs, err := s.Installed()
			if err != nil {
				return err
			}
			if len(vs) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no meteor versions installed (try `mvm install <version>`)")
				return nil
			}
			def := s.Default()
			cur, _ := resolveVersion(s, "")
			for _, v := range vs {
				mark := "  "
				if v == cur.Version {
					mark = "->"
				}
				suffix := ""
				if v == def {
					suffix = " (default)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s%s\n", mark, v, suffix)
			}
			return nil
		},
	}
}

func lsRemoteCmd() *cobra.Command {
	var pre, refresh bool
	cmd := &cobra.Command{
		Use:     "ls-remote [prefix]",
		Short:   "List Meteor releases available for install",
		Example: "  mvm ls-remote 3.1\n  mvm ls-remote --pre",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			vs, err := source.Remote(cmd.Context(), s, refresh)
			if err != nil {
				return err
			}
			vs = source.Filter(vs, pre)
			arch, _ := platform.Arch()
			for _, v := range vs {
				if len(args) == 1 && v != args[0] && !strings.HasPrefix(v, args[0]+".") && !strings.HasPrefix(v, args[0]+"-") {
					continue
				}
				if arch != "" && platform.Supported(v, arch) != nil {
					continue
				}
				mark := "  "
				if s.IsInstalled(v) {
					mark = " *"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", mark, v)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&pre, "pre", false, "include alpha/beta/rc releases")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "ignore the cached release list")
	return cmd
}

func useCmd() *cobra.Command {
	var doInstall bool
	cmd := &cobra.Command{
		Use:   "use <version>",
		Short: "Set the default Meteor release (used outside Meteor projects)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			v := resolve.Normalize(args[0])
			if !s.IsInstalled(v) {
				if !doInstall {
					return fmt.Errorf("meteor %s is not installed (use --install)", v)
				}
				arch, err := platform.Arch()
				if err != nil {
					return err
				}
				if err := install.Install(cmd.Context(), s, v, arch, install.Options{Log: cmd.ErrOrStderr()}); err != nil {
					return err
				}
			}
			if err := s.SetDefault(v); err != nil {
				return err
			}
			if _, err := installShim(s); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "default meteor is now %s\n", v)
			return nil
		},
	}
	cmd.Flags().BoolVar(&doInstall, "install", false, "install the version if missing")
	return cmd
}

func currentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show which Meteor release applies here and why",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			res, err := resolveVersion(s, "")
			if err != nil {
				return err
			}
			state := "installed"
			if !s.IsInstalled(res.Version) {
				state = "not installed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (from %s, %s)\n", res.Version, describe(res), state)
			return nil
		},
	}
}

func whichCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "which [version]",
		Short: "Print the path of the meteor launcher for a release",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			res, err := resolveVersion(s, firstArg(args))
			if err != nil {
				return err
			}
			env, err := run.Load(s, res.Version)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), env.Meteor())
			return nil
		},
	}
}

func execCmd() *cobra.Command {
	var autoInstall, noNode bool
	cmd := &cobra.Command{
		Use:   "exec [version] -- <command> [args...]",
		Short: "Run a command with a Meteor release on PATH",
		Long: "Run a command with a Meteor release (and its bundled node/npm) on PATH.\n" +
			"Without a version, it is resolved from " + resolve.EnvVersion + ", .meteor/release, .mvmrc or the default.",
		Example: "  mvm exec --auto-install -- meteor npm ci\n" +
			"  mvm exec -- meteor build ../out --server-only\n" +
			"  mvm exec 2.16 -- meteor --version",
		RunE: func(cmd *cobra.Command, args []string) error {
			explicit, argv, err := splitExecArgs(args, cmd.ArgsLenAtDash())
			if err != nil {
				return err
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			env, err := prepare(cmd.Context(), s, explicit, autoInstall || os.Getenv(EnvAutoInstall) != "")
			if err != nil {
				return err
			}
			return env.Exec(argv, !noNode)
		},
	}
	// Flags after the command belong to the command: `mvm exec meteor build --server-only`.
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().BoolVar(&autoInstall, "auto-install", false, "install the release if missing (also "+EnvAutoInstall+"=1)")
	cmd.Flags().BoolVar(&noNode, "no-node", false, "do not put the bundled node/npm on PATH")
	return cmd
}

func envCmd() *cobra.Command {
	var noNode bool
	cmd := &cobra.Command{
		Use:     "env [version]",
		Short:   "Print shell exports for a Meteor release",
		Example: `  eval "$(mvm env 3.1.2)"`,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			res, err := resolveVersion(s, firstArg(args))
			if err != nil {
				return err
			}
			env, err := run.Load(s, res.Version)
			if err != nil {
				return err
			}
			vars := env.Vars(!noNode)
			keys := make([]string, 0, len(vars))
			for k := range vars {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "export %s=%s\n", k, shellQuote(vars[k]))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noNode, "no-node", false, "do not put the bundled node/npm on PATH")
	return cmd
}

func pruneCmd() *cobra.Command {
	var keep, unusedDays int
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove Meteor releases that are no longer used",
		Long: "Remove installed releases. --keep N keeps the N most recently used;\n" +
			"--unused-days D removes those not used for D days. The default release is never removed.",
		Example: "  mvm prune --unused-days 30\n  mvm prune --keep 5 --dry-run",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if keep <= 0 && unusedDays <= 0 {
				return fmt.Errorf("specify --keep and/or --unused-days")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			vs, err := s.Installed()
			if err != nil {
				return err
			}
			sort.SliceStable(vs, func(i, j int) bool { return s.LastUsed(vs[i]).After(s.LastUsed(vs[j])) })
			def := s.Default()
			cutoff := time.Now().AddDate(0, 0, -unusedDays)
			for i, v := range vs {
				if v == def {
					continue
				}
				used := s.LastUsed(v)
				stale := unusedDays > 0 && used.Before(cutoff)
				extra := keep > 0 && i >= keep
				if !stale && !extra {
					continue
				}
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "would remove meteor %s (last used %s)\n", v, used.Format(time.DateOnly))
					continue
				}
				if err := s.Remove(v); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed meteor %s (last used %s)\n", v, used.Format(time.DateOnly))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&keep, "keep", 0, "keep the N most recently used releases")
	cmd.Flags().IntVar(&unusedDays, "unused-days", 0, "remove releases unused for this many days")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "only print what would be removed")
	return cmd
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the mvm setup",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			problems := 0
			check := func(ok bool, msg string, args ...any) {
				mark := "ok  "
				if !ok {
					mark = "FAIL"
					problems++
				}
				fmt.Fprintf(out, "[%s] %s\n", mark, fmt.Sprintf(msg, args...))
			}

			arch, err := platform.Arch()
			check(err == nil, "platform: linux/%s %v", arch, errString(err))

			s, err := openStore()
			if err != nil {
				return err
			}
			check(os.MkdirAll(s.Root, 0o755) == nil && unix.Access(s.Root, unix.W_OK) == nil, "MVM_HOME writable: %s", s.Root)

			var st unix.Statfs_t
			if unix.Statfs(s.Root, &st) == nil {
				free := st.Bavail * uint64(st.Bsize) >> 30
				check(free >= 5, "free disk space: %d GB (each release needs 2-3 GB unpacked)", free)
			}

			vs, err := s.Installed()
			check(err == nil, "installed releases: %d", len(vs))
			for _, v := range vs {
				_, err := run.Load(s, v)
				check(err == nil, "meteor %s launcher %v", v, errString(err))
			}

			leftovers, _ := filepath.Glob(filepath.Join(s.VersionsDir(), ".tmp-*"))
			trash, _ := filepath.Glob(filepath.Join(s.VersionsDir(), ".trash-*"))
			check(len(leftovers)+len(trash) == 0, "no leftovers from interrupted installs %v", append(leftovers, trash...))

			if def := s.Default(); def != "" {
				check(s.IsInstalled(def), "default release %s is installed", def)
			}

			shimOnPath := false
			for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
				if dir == s.BinDir() {
					shimOnPath = true
				}
			}
			check(shimOnPath, "%s on PATH (only needed for the transparent `meteor` shim; see `mvm init`)", s.BinDir())

			url := source.TarballURL("3.0.4", platform.ArchX86_64)
			resp, err := http.Head(url)
			if err == nil {
				resp.Body.Close()
			}
			check(err == nil && resp.StatusCode == http.StatusOK, "download source reachable: %s", source.BaseURL())

			if problems > 0 {
				return fmt.Errorf("%d check(s) failed", problems)
			}
			return nil
		},
	}
}

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the meteor shim and print the PATH line for your shell",
		Long: "Creates $MVM_HOME/bin/meteor, a shim that runs the right Meteor release for\n" +
			"the current directory. Add the printed line to your shell profile or CI environment.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStore()
			if err != nil {
				return err
			}
			if _, err := installShim(s); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "export PATH=%s:\"$PATH\"\n", shellQuote(s.BinDir()))
			return nil
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mvm version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "mvm %s (%s)\n", version, commit)
		},
	}
}

// splitExecArgs separates the optional version from the command in
// `mvm exec [version] -- cmd...`. Interspersed flag parsing is off, so cobra
// only reports the dash position when "--" comes before any positional arg;
// after a version it arrives as a literal "--" in args.
func splitExecArgs(args []string, dash int) (version string, argv []string, err error) {
	if dash < 0 {
		dash = slices.Index(args, "--")
		if dash >= 0 {
			args = slices.Delete(slices.Clone(args), dash, dash+1)
		}
	}
	switch {
	case dash < 0:
		argv = args
	case dash > 1:
		return "", nil, fmt.Errorf("expected at most one version before --, got %v", args[:dash])
	default:
		if dash == 1 {
			version = args[0]
		}
		argv = args[dash:]
	}
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("missing command, e.g. `mvm exec -- meteor --version`")
	}
	return version, argv, nil
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return "(" + err.Error() + ")"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
