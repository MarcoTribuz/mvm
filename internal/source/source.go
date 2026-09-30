// Package source knows where Meteor releases come from: the list of
// available versions and the URL of each bootstrap tarball.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MarcoTribuz/mvm/internal/platform"
	"github.com/MarcoTribuz/mvm/internal/store"
)

const (
	// DefaultBaseURL is where the official installer downloads from.
	DefaultBaseURL = "https://static.meteor.com/packages-bootstrap"
	// EnvMirror overrides DefaultBaseURL (Artifactory, Nexus, S3...). The
	// mirror must keep the layout <base>/<version>/meteor-bootstrap-os.<os>.<arch>.tar.gz.
	EnvMirror = "MVM_MIRROR"

	tagsURL        = "https://api.github.com/repos/meteor/meteor/git/matching-refs/tags/release/METEOR@"
	tagPrefix      = "refs/tags/release/METEOR@"
	remoteCacheTTL = time.Hour
)

// Only final releases and alpha/beta/rc prereleases; the tag list also holds
// experiments such as "new-version-solver-1".
var releaseRe = regexp.MustCompile(`^\d+\.\d+(\.\d+)*(-(alpha|beta|rc)\.\d+)?$`)

// BaseURL returns the configured download base URL.
func BaseURL() string {
	if m := os.Getenv(EnvMirror); m != "" {
		return strings.TrimRight(m, "/")
	}
	return DefaultBaseURL
}

// TarballURL returns the bootstrap tarball URL for a release, OS and arch.
func TarballURL(version, osName, arch string) string {
	return fmt.Sprintf("%s/%s/meteor-bootstrap-os.%s.%s.tar.gz", BaseURL(), version, osName, arch)
}

// Remote lists published Meteor 2.x/3.x releases, using a cached copy
// younger than an hour unless refresh is set.
func Remote(ctx context.Context, s *store.Store, refresh bool) ([]string, error) {
	if !refresh {
		if vs, ok := readCache(s.RemoteCacheFile()); ok {
			return vs, nil
		}
	}
	vs, err := fetchTags(ctx)
	if err != nil {
		// Stale cache beats no answer when GitHub is rate limiting us.
		if cached, ok := readCacheAnyAge(s.RemoteCacheFile()); ok {
			return cached, nil
		}
		return nil, err
	}
	if b, err := json.Marshal(vs); err == nil {
		if os.MkdirAll(filepath.Dir(s.RemoteCacheFile()), 0o755) == nil {
			_ = store.WriteFileAtomic(s.RemoteCacheFile(), b, 0o644)
		}
	}
	return vs, nil
}

// Filter keeps stable releases unless prerelease is set.
func Filter(vs []string, prerelease bool) []string {
	var out []string
	for _, v := range vs {
		if !prerelease && strings.Contains(v, "-") {
			continue
		}
		out = append(out, v)
	}
	return out
}

func fetchTags(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing releases: GitHub API returned %s (set GITHUB_TOKEN if rate limited)", resp.Status)
	}
	var refs []struct {
		Ref string `json:"ref"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refs); err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Ref
	}
	return parseRefs(names), nil
}

// parseRefs turns git refs into supported, sorted release versions.
func parseRefs(refs []string) []string {
	var out []string
	for _, ref := range refs {
		v, ok := strings.CutPrefix(ref, tagPrefix)
		if !ok || !releaseRe.MatchString(v) {
			continue
		}
		if major, err := platform.Major(v); err != nil || major < 2 {
			continue
		}
		out = append(out, v)
	}
	store.SortVersions(out)
	return out
}

func readCache(path string) ([]string, bool) {
	fi, err := os.Stat(path)
	if err != nil || time.Since(fi.ModTime()) > remoteCacheTTL {
		return nil, false
	}
	return readCacheAnyAge(path)
}

func readCacheAnyAge(path string) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var vs []string
	if json.Unmarshal(b, &vs) != nil || len(vs) == 0 {
		return nil, false
	}
	return vs, true
}
