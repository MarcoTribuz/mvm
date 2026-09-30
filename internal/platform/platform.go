// Package platform maps the host to Meteor's platform identifiers and knows
// which Meteor releases exist for which architecture.
package platform

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Arch values as used by Meteor in bootstrap tarball names (os.linux.<arch>).
const (
	ArchX86_64  = "x86_64"
	ArchAarch64 = "aarch64"
)

// Arch returns the Meteor architecture for the current host.
// MVM_ARCH overrides detection (useful for tests and cross-prefetching).
func Arch() (string, error) {
	if a := os.Getenv("MVM_ARCH"); a != "" {
		switch a {
		case ArchX86_64, ArchAarch64:
			return a, nil
		}
		return "", fmt.Errorf("MVM_ARCH=%q not supported (use %s or %s)", a, ArchX86_64, ArchAarch64)
	}
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("unsupported OS %q: mvm currently supports linux only", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "amd64":
		return ArchX86_64, nil
	case "arm64":
		return ArchAarch64, nil
	}
	return "", fmt.Errorf("unsupported architecture %q", runtime.GOARCH)
}

// Supported reports whether a Meteor release is published for arch.
// Meteor ships linux/aarch64 bootstrap tarballs only from 3.0 onwards.
func Supported(version, arch string) error {
	major, err := Major(version)
	if err != nil {
		return err
	}
	if major < 2 {
		return fmt.Errorf("meteor %s: only Meteor 2.x and 3.x are supported", version)
	}
	if arch == ArchAarch64 && major < 3 {
		return fmt.Errorf("meteor %s is not published for linux/%s (arm64 builds exist from 3.0); use an x86_64 agent", version, arch)
	}
	return nil
}

// Major extracts the major version number from a Meteor release string.
func Major(version string) (int, error) {
	head, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(head)
	if err != nil {
		return 0, fmt.Errorf("invalid meteor version %q", version)
	}
	return n, nil
}
