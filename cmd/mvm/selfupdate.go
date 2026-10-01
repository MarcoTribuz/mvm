package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/MarcoTribuz/mvm/internal/resolve"
)

// Where mvm releases live; variables so tests can point them at a fake server.
var (
	mvmLatestAPI   = "https://api.github.com/repos/MarcoTribuz/mvm/releases/latest"
	mvmDownloadURL = "https://github.com/MarcoTribuz/mvm/releases/download"
)

func selfUpdateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "self-update [version]",
		Short: "Update mvm itself to the latest (or given) release",
		Example: "  mvm self-update\n" +
			"  mvm self-update 0.2.0   # also downgrades",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := cmd.ErrOrStderr()
			want := resolve.Normalize(firstArg(args))
			if want == "" {
				latest, err := latestMVM(cmd.Context())
				if err != nil {
					return err
				}
				want = latest
			}
			if want == version && !force {
				fmt.Fprintf(cmd.OutOrStdout(), "mvm %s is already installed\n", version)
				return nil
			}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if exe, err = filepath.EvalSymlinks(exe); err != nil {
				return err
			}
			fmt.Fprintf(log, "mvm: updating %s from %s to %s\n", exe, version, want)
			if err := selfUpdate(cmd.Context(), exe, want); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "mvm %s installed to %s\n", want, exe)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "reinstall even if already on that version")
	return cmd
}

// latestMVM returns the version of the newest published mvm release.
func latestMVM(ctx context.Context) (string, error) {
	body, err := httpGet(ctx, mvmLatestAPI, 1<<20)
	if err != nil {
		return "", fmt.Errorf("finding latest mvm release: %w", err)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil || rel.TagName == "" {
		return "", fmt.Errorf("finding latest mvm release: unexpected response from %s", mvmLatestAPI)
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// selfUpdate downloads release v for this platform, checks it against the
// release's checksums.txt and atomically replaces exe with it.
func selfUpdate(ctx context.Context, exe, v string) error {
	file := fmt.Sprintf("mvm_%s_%s_%s.tar.gz", v, runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("%s/v%s", mvmDownloadURL, v)

	sums, err := httpGet(ctx, base+"/checksums.txt", 1<<20)
	if err != nil {
		return fmt.Errorf("mvm %s: %w", v, err)
	}
	want := ""
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if sum, name, ok := strings.Cut(sc.Text(), "  "); ok && name == file {
			want = sum
		}
	}
	if want == "" {
		return fmt.Errorf("mvm %s: no %s in checksums.txt (not published for this platform?)", v, file)
	}

	archive, err := httpGet(ctx, base+"/"+file, 100<<20)
	if err != nil {
		return fmt.Errorf("mvm %s: %w", v, err)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("mvm %s: checksum mismatch for %s", v, file)
	}
	bin, err := extractBinary(archive)
	if err != nil {
		return fmt.Errorf("mvm %s: %w", v, err)
	}

	// Write next to exe so the rename is atomic; a running mvm keeps its old inode.
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".mvm-update-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (reinstall with install.sh or fix permissions): %w", filepath.Dir(exe), err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), exe)
}

// extractBinary returns the "mvm" file from a release tarball.
func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("no mvm binary in release archive")
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && hdr.Name == "mvm" {
			return io.ReadAll(io.LimitReader(tr, 100<<20))
		}
	}
}

func httpGet(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
