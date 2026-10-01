// Package store manages the on-disk layout of MVM_HOME.
//
//	$MVM_HOME/
//	  bin/meteor           shim (symlink to the mvm binary)
//	  versions/<v>/.meteor isolated Meteor warehouse for release <v>
//	  cache/downloads/     downloaded bootstrap tarballs
//	  checksums/           sha256 recorded on first download (trust on first use)
//	  locks/               per-version install locks
//	  default              global default version
//	  aliases/<name>       version a user-defined alias points to
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

// EnvHome overrides the store root.
const EnvHome = "MVM_HOME"

const (
	lastUsedFile = ".mvm-last-used"
	// launcherFile records where .meteor/meteor pointed right after install.
	// `meteor update` re-points that symlink to the newest release, which
	// would silently change the version (and loop, see meteor/meteor#14797).
	launcherFile = ".mvm-launcher"
)

// Store is a handle on an MVM_HOME directory.
type Store struct {
	Root string
}

// Open returns the store at $MVM_HOME or ~/.mvm.
func Open() (*Store, error) {
	root := os.Getenv(EnvHome)
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory, set %s: %w", EnvHome, err)
		}
		root = filepath.Join(home, ".mvm")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (s *Store) BinDir() string               { return filepath.Join(s.Root, "bin") }
func (s *Store) VersionsDir() string          { return filepath.Join(s.Root, "versions") }
func (s *Store) VersionDir(v string) string   { return filepath.Join(s.VersionsDir(), v) }
func (s *Store) WarehouseDir(v string) string { return filepath.Join(s.VersionDir(v), ".meteor") }
func (s *Store) LauncherPath(v string) string { return filepath.Join(s.WarehouseDir(v), "meteor") }
func (s *Store) DownloadsDir() string         { return filepath.Join(s.Root, "cache", "downloads") }
func (s *Store) ChecksumsDir() string         { return filepath.Join(s.Root, "checksums") }
func (s *Store) LocksDir() string             { return filepath.Join(s.Root, "locks") }
func (s *Store) LockPath(v string) string     { return filepath.Join(s.LocksDir(), v+".lock") }
func (s *Store) DefaultFile() string          { return filepath.Join(s.Root, "default") }
func (s *Store) AliasesDir() string           { return filepath.Join(s.Root, "aliases") }
func (s *Store) RemoteCacheFile() string {
	return filepath.Join(s.Root, "cache", "remote-versions.json")
}
func (s *Store) ChecksumFile(v, arch string) string {
	return filepath.Join(s.ChecksumsDir(), v+"-"+arch+".sha256")
}

// IsInstalled reports whether release v is fully installed.
func (s *Store) IsInstalled(v string) bool {
	_, err := os.Stat(s.LauncherPath(v))
	return err == nil
}

// Installed lists installed versions, sorted ascending.
func (s *Store) Installed() ([]string, error) {
	entries, err := os.ReadDir(s.VersionsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if s.IsInstalled(name) {
			out = append(out, name)
		}
	}
	SortVersions(out)
	return out, nil
}

// Remove deletes an installed version. It renames first so a concurrent
// reader never sees a half-deleted tree under the real name.
func (s *Store) Remove(v string) error {
	dir := s.VersionDir(v)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("meteor %s is not installed", v)
	}
	trash := filepath.Join(s.VersionsDir(), fmt.Sprintf(".trash-%s-%d", v, os.Getpid()))
	if err := os.Rename(dir, trash); err != nil {
		return err
	}
	return os.RemoveAll(trash)
}

// RecordLauncher stores the launcher target of the tree rooted at versionDir.
func RecordLauncher(versionDir string) error {
	target, err := os.Readlink(filepath.Join(versionDir, ".meteor", "meteor"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(versionDir, launcherFile), []byte(target+"\n"), 0o644)
}

// PinLauncher makes .meteor/meteor of version v point back to the tool it
// was installed with, and returns that target (relative to the warehouse).
// Versions installed without a record keep their current symlink.
func (s *Store) PinLauncher(v string) (string, error) {
	link := s.LauncherPath(v)
	cur, err := os.Readlink(link)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.VersionDir(v), launcherFile))
	if errors.Is(err, os.ErrNotExist) {
		return cur, nil
	}
	if err != nil {
		return "", err
	}
	want := strings.TrimSpace(string(b))
	if cur == want {
		return want, nil
	}
	tmp := fmt.Sprintf("%s.mvm-%d", link, os.Getpid())
	_ = os.Remove(tmp)
	if err := os.Symlink(want, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return want, nil
}

// Default returns the global default version, or "" if unset.
func (s *Store) Default() string {
	b, err := os.ReadFile(s.DefaultFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SetDefault writes the global default version atomically.
func (s *Store) SetDefault(v string) error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(s.DefaultFile(), []byte(v+"\n"), 0o644)
}

// Alias returns the version alias name points to, or "" if it is not set.
func (s *Store) Alias(name string) string {
	b, err := os.ReadFile(filepath.Join(s.AliasesDir(), name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SetAlias points alias name at version v.
func (s *Store) SetAlias(name, v string) error {
	if err := os.MkdirAll(s.AliasesDir(), 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(s.AliasesDir(), name), []byte(v+"\n"), 0o644)
}

// RemoveAlias deletes alias name.
func (s *Store) RemoveAlias(name string) error {
	err := os.Remove(filepath.Join(s.AliasesDir(), name))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("alias %q does not exist", name)
	}
	return err
}

// Aliases returns all user-defined aliases as name -> version.
func (s *Store) Aliases() (map[string]string, error) {
	entries, err := os.ReadDir(s.AliasesDir())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if v := s.Alias(e.Name()); v != "" {
			out[e.Name()] = v
		}
	}
	return out, nil
}

// TouchUsed records that version v was just used (for prune --unused-days).
func (s *Store) TouchUsed(v string) {
	p := filepath.Join(s.VersionDir(v), lastUsedFile)
	now := time.Now()
	if err := os.Chtimes(p, now, now); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(p, nil, 0o644)
	}
}

// LastUsed returns when version v was last used, falling back to install time.
func (s *Store) LastUsed(v string) time.Time {
	if fi, err := os.Stat(filepath.Join(s.VersionDir(v), lastUsedFile)); err == nil {
		return fi.ModTime()
	}
	if fi, err := os.Stat(s.VersionDir(v)); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// WriteFileAtomic writes via a temp file + rename in the same directory.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// SortVersions sorts Meteor release strings ascending by semver,
// falling back to string order for anything that does not parse.
func SortVersions(vs []string) {
	sort.SliceStable(vs, func(i, j int) bool { return CompareVersions(vs[i], vs[j]) < 0 })
}

// CompareVersions compares two Meteor release strings.
func CompareVersions(a, b string) int {
	va, ea := semver.NewVersion(a)
	vb, eb := semver.NewVersion(b)
	if ea == nil && eb == nil {
		return va.Compare(vb)
	}
	return strings.Compare(a, b)
}
