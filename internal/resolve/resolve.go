// Package resolve decides which Meteor version a command should use.
//
// Resolution is per process and never mutates global state, so concurrent
// builds on the same agent can each use a different version.
package resolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvVersion is the environment variable that pins a version for a process.
const EnvVersion = "MVM_METEOR_VERSION"

// ErrNoVersion is returned when no source specifies a version.
var ErrNoVersion = errors.New("no meteor version found: pass a version, set " + EnvVersion +
	", run inside a Meteor project (.meteor/release), add a .mvmrc or set a default with `mvm use`")

// Source describes where a resolved version came from.
type Source string

const (
	SourceFlag    Source = "argument"
	SourceEnv     Source = EnvVersion
	SourceRelease Source = ".meteor/release"
	SourceRC      Source = ".mvmrc"
	SourceDefault Source = "default"
)

// Result is a resolved version plus its origin.
type Result struct {
	Version string
	Source  Source
	Path    string // file the version was read from, if any
}

// Options are the inputs to Resolve.
type Options struct {
	Flag        string // explicit version from the command line
	Dir         string // directory to start searching from
	DefaultFile string // path of the global default file
}

// Resolve applies the order: flag, env, .meteor/release, .mvmrc, default.
func Resolve(o Options) (Result, error) {
	if v := strings.TrimSpace(o.Flag); v != "" {
		return Result{Version: Normalize(v), Source: SourceFlag}, nil
	}
	if v := strings.TrimSpace(os.Getenv(EnvVersion)); v != "" {
		return Result{Version: Normalize(v), Source: SourceEnv}, nil
	}
	if r, ok, err := ProjectRelease(o.Dir); err != nil || ok {
		return r, err
	}
	if p, ok := findUp(o.Dir, ".mvmrc"); ok {
		v, err := readFirstLine(p)
		if err != nil {
			return Result{}, err
		}
		if v != "" {
			return Result{Version: Normalize(v), Source: SourceRC, Path: p}, nil
		}
	}
	if o.DefaultFile != "" {
		v, err := readFirstLine(o.DefaultFile)
		if err == nil && v != "" {
			return Result{Version: Normalize(v), Source: SourceDefault, Path: o.DefaultFile}, nil
		}
	}
	return Result{}, ErrNoVersion
}

// ProjectRelease returns the release pinned by the Meteor project containing
// dir, if any.
func ProjectRelease(dir string) (Result, bool, error) {
	p, ok := findUp(dir, filepath.Join(".meteor", "release"))
	if !ok {
		return Result{}, false, nil
	}
	v, err := readRelease(p)
	if err != nil {
		return Result{}, false, err
	}
	return Result{Version: v, Source: SourceRelease, Path: p}, true, nil
}

// ParseRelease parses the content of a .meteor/release file ("METEOR@3.1.2").
func ParseRelease(content string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "none" {
			return "", errors.New(".meteor/release is \"none\" (app runs from a Meteor checkout)")
		}
		track, v, ok := strings.Cut(line, "@")
		if !ok || v == "" {
			return "", fmt.Errorf("invalid .meteor/release content %q", line)
		}
		if track != "METEOR" {
			return "", fmt.Errorf("unsupported release track %q (only METEOR@ releases are supported)", track)
		}
		return v, nil
	}
	return "", errors.New(".meteor/release is empty")
}

// Normalize accepts "3.1", "METEOR@3.1" or "v3.1" and returns "3.1".
func Normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "METEOR@")
	v = strings.TrimPrefix(v, "v")
	return v
}

func readRelease(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	v, err := ParseRelease(string(b))
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

func readFirstLine(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimSpace(line), nil
}

// findUp looks for rel in dir and its parents.
func findUp(dir, rel string) (string, bool) {
	if dir == "" {
		return "", false
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		p := filepath.Join(dir, rel)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
