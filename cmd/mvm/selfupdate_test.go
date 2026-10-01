package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func releaseTarball(t *testing.T, bin string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"README.md": "readme", "mvm": bin} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// fakeReleases serves a GitHub-like latest API and release v9.9.9.
func fakeReleases(t *testing.T, archive []byte, sum string) {
	t.Helper()
	file := fmt.Sprintf("mvm_9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Write([]byte(`{"tag_name": "v9.9.9"}`))
		case "/download/v9.9.9/checksums.txt":
			fmt.Fprintf(w, "%s  mvm_9.9.9_other_os.tar.gz\n%s  %s\n", strings.Repeat("0", 64), sum, file)
		case "/download/v9.9.9/" + file:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	oldAPI, oldDL := mvmLatestAPI, mvmDownloadURL
	mvmLatestAPI, mvmDownloadURL = srv.URL+"/latest", srv.URL+"/download"
	t.Cleanup(func() { mvmLatestAPI, mvmDownloadURL = oldAPI, oldDL })
}

func TestSelfUpdate(t *testing.T) {
	archive := releaseTarball(t, "new binary")
	sum := sha256.Sum256(archive)
	fakeReleases(t, archive, hex.EncodeToString(sum[:]))

	v, err := latestMVM(context.Background())
	if err != nil || v != "9.9.9" {
		t.Fatalf("latestMVM = %q, %v", v, err)
	}
	exe := filepath.Join(t.TempDir(), "mvm")
	os.WriteFile(exe, []byte("old binary"), 0o755)
	if err := selfUpdate(context.Background(), exe, v); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	fi, _ := os.Stat(exe)
	if string(b) != "new binary" || fi.Mode().Perm() != 0o755 {
		t.Errorf("exe = %q mode %v", b, fi.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".mvm-update-*")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestSelfUpdateChecksumMismatch(t *testing.T) {
	fakeReleases(t, releaseTarball(t, "evil"), strings.Repeat("a", 64))
	exe := filepath.Join(t.TempDir(), "mvm")
	os.WriteFile(exe, []byte("old binary"), 0o755)
	err := selfUpdate(context.Background(), exe, "9.9.9")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old binary" {
		t.Errorf("exe replaced despite mismatch: %q", b)
	}
}

func TestSelfUpdateMissingVersion(t *testing.T) {
	fakeReleases(t, nil, "")
	exe := filepath.Join(t.TempDir(), "mvm")
	if err := selfUpdate(context.Background(), exe, "1.2.3"); err == nil {
		t.Fatal("expected error for unpublished version")
	}
}
