package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/marcotribuzio/mvm/internal/source"
	"github.com/marcotribuzio/mvm/internal/store"
)

type entry struct {
	name, body, link string
	typ              byte
	mode             int64
}

func makeTarball(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: mode, Linkname: e.link, Size: int64(len(e.body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// fakeRelease mimics the layout of a Meteor bootstrap tarball.
func fakeRelease(t *testing.T, tool string) []byte {
	toolDir := ".meteor/packages/meteor-tool/" + tool + "/mt-os.linux.x86_64"
	return makeTarball(t, []entry{
		{name: ".meteor/", typ: tar.TypeDir, mode: 0o755},
		{name: toolDir + "/meteor", typ: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\necho meteor " + tool + "\n"},
		{name: toolDir + "/dev_bundle/bin/node", typ: tar.TypeReg, mode: 0o755, body: "#!/bin/sh\n"},
		{name: ".meteor/meteor", typ: tar.TypeSymlink, link: "packages/meteor-tool/" + tool + "/mt-os.linux.x86_64/meteor"},
	})
}

func serve(t *testing.T, body []byte, hits *atomic.Int32) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.Contains(r.URL.Path, "aarch64") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(source.EnvMirror, srv.URL)
	t.Setenv(EnvKeepDownloads, "")
}

func TestInstall(t *testing.T) {
	var hits atomic.Int32
	serve(t, fakeRelease(t, "3.1.2"), &hits)
	s := &store.Store{Root: t.TempDir()}

	if err := Install(context.Background(), s, "3.1.2", "x86_64", Options{}); err != nil {
		t.Fatal(err)
	}
	if !s.IsInstalled("3.1.2") {
		t.Fatal("not installed")
	}
	fi, err := os.Stat(s.LauncherPath("3.1.2"))
	if err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("launcher not executable: %v %v", fi, err)
	}
	if _, err := os.Stat(s.ChecksumFile("3.1.2", "x86_64")); err != nil {
		t.Error("checksum not recorded")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(s.DownloadsDir(), "*")); len(leftovers) != 0 {
		t.Errorf("downloads not cleaned: %v", leftovers)
	}

	// Second install is a no-op.
	if err := Install(context.Background(), s, "3.1.2", "x86_64", Options{}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1", hits.Load())
	}
}

func TestInstallConcurrent(t *testing.T) {
	var hits atomic.Int32
	serve(t, fakeRelease(t, "2.16.0"), &hits)
	s := &store.Store{Root: t.TempDir()}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Install(context.Background(), s, "2.16", "x86_64", Options{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want exactly 1 download", hits.Load())
	}
}

func TestInstallChecksumMismatch(t *testing.T) {
	var hits atomic.Int32
	serve(t, fakeRelease(t, "3.0.4"), &hits)
	s := &store.Store{Root: t.TempDir()}
	os.MkdirAll(s.ChecksumsDir(), 0o755)
	os.WriteFile(s.ChecksumFile("3.0.4", "x86_64"), []byte("deadbeef\n"), 0o644)

	err := Install(context.Background(), s, "3.0.4", "x86_64", Options{})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	if s.IsInstalled("3.0.4") {
		t.Error("installed despite checksum mismatch")
	}
}

func TestInstallUnsupportedArch(t *testing.T) {
	s := &store.Store{Root: t.TempDir()}
	if err := Install(context.Background(), s, "2.16", "aarch64", Options{}); err == nil {
		t.Fatal("expected error for 2.x on aarch64")
	}
}

func TestExtractRejectsEscapes(t *testing.T) {
	cases := map[string][]entry{
		"dotdot":       {{name: "../evil", typ: tar.TypeReg, body: "x"}},
		"abs symlink":  {{name: "link", typ: tar.TypeSymlink, link: "/etc/passwd"}},
		"rel symlink":  {{name: "a/link", typ: tar.TypeSymlink, link: "../../outside"}},
		"hardlink out": {{name: "h", typ: tar.TypeLink, link: "../outside"}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "x.tar.gz")
			os.WriteFile(p, makeTarball(t, entries), 0o644)
			if err := extract(p, filepath.Join(t.TempDir(), "out")); err == nil {
				t.Error("expected extract to fail")
			}
		})
	}
}
