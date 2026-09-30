package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSortVersions(t *testing.T) {
	vs := []string{"3.1", "2.16", "3.0.4", "2.2", "3.1-beta.1", "3.10"}
	SortVersions(vs)
	want := []string{"2.2", "2.16", "3.0.4", "3.1-beta.1", "3.1", "3.10"}
	if !reflect.DeepEqual(vs, want) {
		t.Errorf("got %v, want %v", vs, want)
	}
}

func fakeInstall(t *testing.T, s *Store, v string) {
	t.Helper()
	if err := os.MkdirAll(s.WarehouseDir(v), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.LauncherPath(v), nil, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledAndRemove(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	fakeInstall(t, s, "3.1")
	fakeInstall(t, s, "2.16")
	os.MkdirAll(filepath.Join(s.VersionsDir(), ".tmp-3.3-123"), 0o755) // in-progress install
	os.MkdirAll(filepath.Join(s.VersionsDir(), "3.2"), 0o755)          // no launcher

	got, err := s.Installed()
	if err != nil || !reflect.DeepEqual(got, []string{"2.16", "3.1"}) {
		t.Fatalf("Installed = %v, %v", got, err)
	}
	if err := s.Remove("3.1"); err != nil {
		t.Fatal(err)
	}
	if s.IsInstalled("3.1") {
		t.Error("still installed")
	}
	if err := s.Remove("9.9"); err == nil {
		t.Error("expected error removing missing version")
	}
}

func TestDefault(t *testing.T) {
	s := &Store{Root: filepath.Join(t.TempDir(), "nested")}
	if s.Default() != "" {
		t.Error("expected empty default")
	}
	if err := s.SetDefault("3.1"); err != nil {
		t.Fatal(err)
	}
	if s.Default() != "3.1" {
		t.Errorf("Default = %q", s.Default())
	}
}

func TestPinLauncher(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	wh := s.WarehouseDir("3.3")
	for _, tool := range []string{"3.3.0", "3.5.2"} {
		os.MkdirAll(filepath.Join(wh, "packages", "meteor-tool", tool), 0o755)
	}
	orig := "packages/meteor-tool/3.3.0/meteor"
	if err := os.Symlink(orig, s.LauncherPath("3.3")); err != nil {
		t.Fatal(err)
	}
	if err := RecordLauncher(s.VersionDir("3.3")); err != nil {
		t.Fatal(err)
	}

	// Simulate `meteor update` re-pointing the launcher.
	os.Remove(s.LauncherPath("3.3"))
	os.Symlink("packages/meteor-tool/3.5.2/meteor", s.LauncherPath("3.3"))

	got, err := s.PinLauncher("3.3")
	if err != nil || got != orig {
		t.Fatalf("PinLauncher = %q, %v", got, err)
	}
	if cur, _ := os.Readlink(s.LauncherPath("3.3")); cur != orig {
		t.Errorf("launcher not restored: %s", cur)
	}
}
