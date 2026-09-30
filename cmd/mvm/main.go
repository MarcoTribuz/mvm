// Command mvm is a version manager for Meteor, built for CI agents that
// build many apps pinned to different Meteor releases.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/MarcoTribuz/mvm/internal/install"
	"github.com/MarcoTribuz/mvm/internal/platform"
	"github.com/MarcoTribuz/mvm/internal/resolve"
	"github.com/MarcoTribuz/mvm/internal/run"
	"github.com/MarcoTribuz/mvm/internal/store"
)

// Set by GoReleaser via -ldflags.
var (
	version = "dev"
	commit  = "none"
)

// EnvAutoInstall makes exec and the meteor shim install missing versions.
const EnvAutoInstall = "MVM_AUTO_INSTALL"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	if filepath.Base(os.Args[0]) == "meteor" {
		err = shim(ctx, os.Args[1:])
	} else {
		err = newRootCmd().ExecuteContext(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mvm:", err)
		os.Exit(1)
	}
}

// shim runs when mvm is invoked through the $MVM_HOME/bin/meteor symlink:
// it picks the version for the current directory and becomes that meteor.
func shim(ctx context.Context, args []string) error {
	s, err := store.Open()
	if err != nil {
		return err
	}
	env, err := prepare(ctx, s, "", os.Getenv(EnvAutoInstall) != "")
	if err != nil {
		return err
	}
	return env.Exec(append([]string{"meteor"}, args...), true)
}

// prepare resolves a version (explicit or from context), optionally installs
// it, and returns its environment.
func prepare(ctx context.Context, s *store.Store, explicit string, autoInstall bool) (*run.Env, error) {
	res, err := resolveVersion(s, explicit)
	if err != nil {
		return nil, err
	}
	if !s.IsInstalled(res.Version) {
		if !autoInstall {
			return nil, fmt.Errorf("meteor %s (from %s) is not installed: run `mvm install %s` or set %s=1",
				res.Version, describe(res), res.Version, EnvAutoInstall)
		}
		arch, err := platform.Arch()
		if err != nil {
			return nil, err
		}
		if err := install.Install(ctx, s, res.Version, arch, install.Options{Log: os.Stderr}); err != nil {
			return nil, err
		}
	}
	env, err := run.Load(s, res.Version)
	if err != nil {
		return nil, err
	}
	s.TouchUsed(res.Version)
	return env, nil
}

func resolveVersion(s *store.Store, explicit string) (resolve.Result, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return resolve.Result{}, err
	}
	return resolve.Resolve(resolve.Options{Flag: explicit, Dir: cwd, DefaultFile: s.DefaultFile()})
}

func describe(r resolve.Result) string {
	if r.Path != "" {
		return r.Path
	}
	return string(r.Source)
}

// installShim links $MVM_HOME/bin/meteor to the running mvm binary.
func installShim(s *store.Store) (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.BinDir(), 0o755); err != nil {
		return "", err
	}
	link := filepath.Join(s.BinDir(), "meteor")
	if cur, err := os.Readlink(link); err == nil && cur == self {
		return link, nil
	}
	tmp := link + fmt.Sprintf(".tmp-%d", os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(self, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return link, nil
}
