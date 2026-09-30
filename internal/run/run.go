// Package run builds the environment for a Meteor version and executes
// commands inside it.
package run

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/MarcoTribuz/mvm/internal/resolve"
	"github.com/MarcoTribuz/mvm/internal/store"
)

// Env describes the environment of one installed Meteor version.
type Env struct {
	Version   string
	Warehouse string // $MVM_HOME/versions/<v>/.meteor
	ToolDir   string // .../packages/meteor-tool/<tool>/mt-os.linux.<arch>
}

// Load resolves the paths of an installed version.
func Load(s *store.Store, version string) (*Env, error) {
	if !s.IsInstalled(version) {
		return nil, fmt.Errorf("meteor %s is not installed (run `mvm install %s`)", version, version)
	}
	target, err := s.PinLauncher(version)
	if err != nil {
		return nil, fmt.Errorf("meteor %s: broken launcher: %w", version, err)
	}
	toolDir := filepath.Dir(filepath.Join(s.WarehouseDir(version), target))
	if _, err := os.Stat(toolDir); err != nil {
		return nil, fmt.Errorf("meteor %s: broken launcher: %w", version, err)
	}
	return &Env{
		Version:   version,
		Warehouse: s.WarehouseDir(version),
		ToolDir:   toolDir,
	}, nil
}

// Meteor is the launcher to execute for this version.
func (e *Env) Meteor() string { return filepath.Join(e.Warehouse, "meteor") }

// NodeBin is the dev_bundle bin dir holding the node/npm bundled with this release.
func (e *Env) NodeBin() string { return filepath.Join(e.ToolDir, "dev_bundle", "bin") }

// Vars returns the variables to set for this version.
//
// METEOR_WAREHOUSE_DIR points Meteor at the isolated warehouse instead of
// ~/.meteor. This only works while the app release matches the warehouse
// tool; otherwise Meteor springboards forever (meteor/meteor#14797), which
// CheckRelease guards against.
func (e *Env) Vars(withNode bool) map[string]string {
	path := []string{e.Warehouse}
	if withNode {
		path = append(path, e.NodeBin())
	}
	return map[string]string{
		"METEOR_WAREHOUSE_DIR": e.Warehouse,
		"PATH":                 strings.Join(append(path, os.Getenv("PATH")), string(os.PathListSeparator)),
	}
}

// CheckRelease refuses meteor invocations that would make Meteor springboard
// to another release, which loops forever with an isolated warehouse
// (meteor/meteor#14797):
//   - running this version inside a project pinned to a different release;
//   - `meteor update` without --release, which jumps to the newest release.
//
// Passing --release stops the springboard (e.g. `meteor update --release X`).
func (e *Env) CheckRelease(argv []string, project string, projectFile string) error {
	if filepath.Base(argv[0]) != "meteor" {
		return nil
	}
	args := argv[1:]
	for _, a := range args {
		if a == "--release" || strings.HasPrefix(a, "--release=") {
			return nil
		}
	}
	if len(args) > 0 && args[0] == "update" && updatesRelease(args[1:]) {
		return fmt.Errorf("`meteor update` without --release would switch to the newest Meteor release and loop " +
			"(meteor/meteor#14797). Pick the target explicitly: `mvm exec <version> -- meteor update --release <version>` " +
			"(see `mvm ls-remote`), or use `meteor update --packages-only`")
	}
	if project == "" || project == e.Version {
		return nil
	}
	return fmt.Errorf("%s pins METEOR@%s but meteor %s was requested; running it would loop forever "+
		"(meteor/meteor#14797). Use `mvm exec %s -- ...`, or upgrade the app with `mvm exec %s -- meteor update --release %s`",
		projectFile, project, e.Version, project, e.Version, e.Version)
}

// Environ returns os.Environ() with Vars applied.
func (e *Env) Environ(withNode bool) []string {
	vars := e.Vars(withNode)
	out := make([]string, 0, len(os.Environ())+len(vars))
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := vars[k]; !ok {
			out = append(out, kv)
		}
	}
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	return out
}

// Exec replaces the current process with argv run in this environment.
// argv[0] is looked up in the new PATH, so "meteor" resolves to this version.
func (e *Env) Exec(argv []string, withNode bool) error {
	if cwd, err := os.Getwd(); err == nil {
		if proj, ok, err := resolve.ProjectRelease(cwd); err == nil && ok {
			if err := e.CheckRelease(argv, proj.Version, proj.Path); err != nil {
				return err
			}
		}
	}
	env := e.Environ(withNode)
	bin := argv[0]
	if bin == "meteor" {
		bin = e.Meteor()
	} else if !strings.Contains(bin, "/") {
		p, err := lookPath(bin, env)
		if err != nil {
			return err
		}
		bin = p
	}
	return syscall.Exec(bin, argv, env)
}

// updatesRelease reports whether `meteor update <args>` changes the release:
// it does unless given --packages-only or explicit package names.
func updatesRelease(args []string) bool {
	for _, a := range args {
		if a == "--packages-only" || !strings.HasPrefix(a, "-") {
			return false
		}
	}
	return true
}

func lookPath(name string, env []string) (string, error) {
	for _, kv := range env {
		if p, ok := strings.CutPrefix(kv, "PATH="); ok {
			old := os.Getenv("PATH")
			os.Setenv("PATH", p)
			defer os.Setenv("PATH", old)
			break
		}
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s: command not found", name)
	}
	return p, nil
}
