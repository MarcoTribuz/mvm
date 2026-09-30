package platform

import "testing"

func TestSupported(t *testing.T) {
	tests := []struct {
		version, arch string
		ok            bool
	}{
		{"3.1.2", ArchX86_64, true},
		{"3.0-rc.4", ArchAarch64, true},
		{"2.16", ArchX86_64, true},
		{"2.16", ArchAarch64, false},
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
	t.Setenv("MVM_ARCH", ArchAarch64)
	if a, err := Arch(); err != nil || a != ArchAarch64 {
		t.Errorf("Arch() = %q, %v", a, err)
	}
	t.Setenv("MVM_ARCH", "sparc")
	if _, err := Arch(); err == nil {
		t.Error("expected error for unsupported MVM_ARCH")
	}
}
