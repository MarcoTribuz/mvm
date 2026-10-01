package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/MarcoTribuz/mvm/internal/resolve"
	"github.com/MarcoTribuz/mvm/internal/store"
)

func aliasStore(t *testing.T) *store.Store {
	t.Helper()
	s := &store.Store{Root: t.TempDir()}
	// A fresh remote cache keeps "latest" offline.
	os.MkdirAll(filepath.Dir(s.RemoteCacheFile()), 0o755)
	os.WriteFile(s.RemoteCacheFile(), []byte(`["2.16","3.2","3.3","3.4-rc.1"]`), 0o644)
	s.SetAlias("legacy", "2.16")
	s.SetAlias("venus", "3.2")
	return s
}

func TestExpandVersion(t *testing.T) {
	s := aliasStore(t)
	t.Setenv("MVM_ARCH", "x86_64")
	ctx := context.Background()
	for in, want := range map[string]string{
		"legacy":       "2.16",
		"venus":        "3.2", // not mangled by Normalize's "v" stripping
		"latest":       "3.3", // prereleases skipped
		"METEOR@3.1.2": "3.1.2",
		"v3.1":         "3.1",
	} {
		if got, err := expandVersion(ctx, s, in); err != nil || got != want {
			t.Errorf("expandVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := expandVersion(ctx, s, "default"); err == nil {
		t.Error("default without `mvm use` should fail")
	}
	s.SetDefault("3.2")
	if got, _ := expandVersion(ctx, s, "default"); got != "3.2" {
		t.Errorf("default = %q", got)
	}
}

func TestValidAliasName(t *testing.T) {
	for _, ok := range []string{"legacy", "work-app", "a.b_c", "venus"} {
		if err := validAliasName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"latest", "default", "3.1", "v3", "-x", "a/b", ""} {
		if validAliasName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestResolveVersionAliases(t *testing.T) {
	s := aliasStore(t)
	t.Setenv(resolve.EnvVersion, "")
	ctx := context.Background()
	dir := t.TempDir()
	t.Chdir(dir)

	os.WriteFile(filepath.Join(dir, ".mvmrc"), []byte("legacy\n"), 0o644)
	res, err := resolveVersion(ctx, s, "")
	if err != nil || res.Version != "2.16" || res.Alias != "legacy" {
		t.Errorf(".mvmrc alias: %+v, %v", res, err)
	}

	t.Setenv(resolve.EnvVersion, "venus")
	if res, _ := resolveVersion(ctx, s, ""); res.Version != "3.2" {
		t.Errorf("env alias: %+v", res)
	}

	if res, _ := resolveVersion(ctx, s, "legacy"); res.Version != "2.16" || res.Source != resolve.SourceFlag {
		t.Errorf("flag alias: %+v", res)
	}

	// .meteor/release is a real release, never an alias.
	os.MkdirAll(filepath.Join(dir, ".meteor"), 0o755)
	os.WriteFile(filepath.Join(dir, ".meteor", "release"), []byte("METEOR@3.1.2\n"), 0o644)
	t.Setenv(resolve.EnvVersion, "")
	if res, _ := resolveVersion(ctx, s, ""); res.Version != "3.1.2" || res.Alias != "" {
		t.Errorf("release: %+v", res)
	}
}
