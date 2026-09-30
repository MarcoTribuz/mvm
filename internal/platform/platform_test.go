package platform

import (
	"runtime"
	"testing"
)

func TestSupported(t *testing.T) {
	tests := []struct {
		version, arch string
		ok            bool
	}{
		{"3.1.2", ArchX86_64, true},
		{"3.0-rc.4", ArchAarch64, true},
		{"2.16", ArchX86_64, true},
		{"2.16", ArchAarch64, false},
		{"2.16", ArchArm64, true},
		{"1.12.1", ArchX86_64, false},
		{"garbage", ArchX86_64, false},
	}
	for _, tt := range tests {
		if err := Supported(tt.version, tt.arch); (err == nil) != tt.ok {
			t.Errorf("Supported(%s, %s) = %v, want ok=%v", tt.version, tt.arch, err, tt.ok)
		}
	}
}

func TestArchOverride(t *testing.T) {
	arm, other := ArchAarch64, ArchArm64
	if runtime.GOOS == "darwin" {
		arm, other = ArchArm64, ArchAarch64
	}
	t.Setenv("MVM_ARCH", arm)
	if a, err := Arch(); err != nil || a != arm {
		t.Errorf("Arch() = %q, %v", a, err)
	}
	t.Setenv("MVM_ARCH", other)
	if _, err := Arch(); err == nil {
		t.Errorf("expected error for MVM_ARCH=%s on %s", other, runtime.GOOS)
	}
	t.Setenv("MVM_ARCH", "sparc")
	if _, err := Arch(); err == nil {
		t.Error("expected error for unsupported MVM_ARCH")
	}
}

func TestOS(t *testing.T) {
	want := map[string]string{"linux": OSLinux, "darwin": OSMac}[runtime.GOOS]
	if got, err := OS(); err != nil || got != want {
		t.Errorf("OS() = %q, %v, want %q", got, err, want)
	}
}
