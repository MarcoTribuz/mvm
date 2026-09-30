// Package install downloads and unpacks Meteor bootstrap tarballs into the
// store. Installs are safe to run concurrently: a per-version file lock
// serialises them and the final tree appears atomically via rename.
package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/MarcoTribuz/mvm/internal/platform"
	"github.com/MarcoTribuz/mvm/internal/source"
	"github.com/MarcoTribuz/mvm/internal/store"
)

// EnvKeepDownloads keeps tarballs in cache/downloads after extraction.
const EnvKeepDownloads = "MVM_KEEP_DOWNLOADS"

// Options tune an install.
type Options struct {
	Force bool      // reinstall even if present
	Log   io.Writer // progress output (stderr); nil discards
}

// Install makes release version available in the store. It returns quickly
// if the version is already installed.
func Install(ctx context.Context, s *store.Store, version, arch string, o Options) error {
	log := o.Log
	if log == nil {
		log = io.Discard
	}
	if err := platform.Supported(version, arch); err != nil {
		return err
	}
	if !o.Force && s.IsInstalled(version) {
		return nil
	}
	for _, d := range []string{s.VersionsDir(), s.DownloadsDir(), s.ChecksumsDir(), s.LocksDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	lock := flock.New(s.LockPath(version))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		fmt.Fprintf(log, "mvm: waiting for another process installing meteor %s...\n", version)
		if _, err := lock.TryLockContext(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	defer lock.Unlock()

	// Another process may have finished while we waited.
	if !o.Force && s.IsInstalled(version) {
		return nil
	}

	tarball, err := download(ctx, s, version, arch, log)
	if err != nil {
		return err
	}
	if os.Getenv(EnvKeepDownloads) == "" {
		defer os.Remove(tarball)
	}

	tmp := filepath.Join(s.VersionsDir(), fmt.Sprintf(".tmp-%s-%d", version, os.Getpid()))
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)

	fmt.Fprintf(log, "mvm: extracting meteor %s...\n", version)
	if err := extract(tarball, tmp); err != nil {
		return fmt.Errorf("extracting meteor %s: %w", version, err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".meteor", "meteor")); err != nil {
		return fmt.Errorf("meteor %s: tarball has no .meteor/meteor launcher", version)
	}
	if err := store.RecordLauncher(tmp); err != nil {
		return fmt.Errorf("meteor %s: %w", version, err)
	}

	if o.Force {
		if err := s.Remove(version); err != nil && s.IsInstalled(version) {
			return err
		}
	}
	if err := os.Rename(tmp, s.VersionDir(version)); err != nil {
		return err
	}
	s.TouchUsed(version)
	fmt.Fprintf(log, "mvm: installed meteor %s\n", version)
	return nil
}

// download fetches the tarball (or reuses a cached one) and checks it
// against the sha256 recorded the first time this release was downloaded.
func download(ctx context.Context, s *store.Store, version, arch string, log io.Writer) (string, error) {
	dest := filepath.Join(s.DownloadsDir(), fmt.Sprintf("meteor-%s-%s.tar.gz", version, arch))
	sumFile := s.ChecksumFile(version, arch)
	want := readSum(sumFile)

	if want != "" {
		if got, err := fileSum(dest); err == nil && got == want {
			fmt.Fprintf(log, "mvm: using cached download for meteor %s\n", version)
			return dest, nil
		}
	}

	url := source.TarballURL(version, arch)
	fmt.Fprintf(log, "mvm: downloading %s\n", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading meteor %s: %w", version, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("meteor %s is not published for linux/%s (%s)", version, arch, resp.Status)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("downloading meteor %s: %s", version, resp.Status)
	}

	part := dest + fmt.Sprintf(".part-%d", os.Getpid())
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}
	defer os.Remove(part)

	h := sha256.New()
	pw := &progress{w: log, total: resp.ContentLength, label: "meteor " + version}
	if _, err := io.Copy(io.MultiWriter(f, h, pw), resp.Body); err != nil {
		f.Close()
		return "", fmt.Errorf("downloading meteor %s: %w", version, err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if want != "" && got != want {
		return "", fmt.Errorf("meteor %s: checksum mismatch (got %s, recorded %s in %s); "+
			"delete that file if the release was legitimately republished", version, got, want, sumFile)
	}
	if want == "" {
		if err := store.WriteFileAtomic(sumFile, []byte(got+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	if err := os.Rename(part, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// extract unpacks a .tar.gz into dir, refusing entries that escape it.
func extract(tarball, dir string) error {
	f, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root := filepath.Clean(dir) + string(os.PathSeparator)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, hdr.Name)
		if !strings.HasPrefix(target+string(os.PathSeparator), root) {
			return fmt.Errorf("illegal path in archive: %q", hdr.Name)
		}
		mode := os.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("absolute symlink in archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
			resolved := filepath.Join(filepath.Dir(target), hdr.Linkname)
			if !strings.HasPrefix(resolved+string(os.PathSeparator), root) {
				return fmt.Errorf("symlink escapes archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			src := filepath.Join(dir, hdr.Linkname)
			if !strings.HasPrefix(src+string(os.PathSeparator), root) {
				return fmt.Errorf("hard link escapes archive: %q -> %q", hdr.Name, hdr.Linkname)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Link(src, target); err != nil {
				return err
			}
		}
	}
}

func readSum(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// progress prints a line every 10% — readable in CI logs, unlike \r bars.
type progress struct {
	w     io.Writer
	total int64
	done  int64
	next  int64
	label string
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.total > 0 {
		pct := p.done * 100 / p.total
		if pct >= p.next {
			fmt.Fprintf(p.w, "mvm: %s %3d%% (%d/%d MB)\n", p.label, pct, p.done>>20, p.total>>20)
			p.next = pct - pct%10 + 10
		}
	}
	return len(b), nil
}
