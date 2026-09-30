package run

import "testing"

func TestCheckRelease(t *testing.T) {
	e := &Env{Version: "3.3"}
	tests := []struct {
		name    string
		argv    []string
		project string
		ok      bool
	}{
		{"same release", []string{"meteor", "build"}, "3.3", true},
		{"no project", []string{"meteor", "create", "x"}, "", true},
		{"mismatch", []string{"meteor", "build"}, "3.0.4", false},
		{"mismatch via path", []string{"/x/meteor", "run"}, "3.0.4", false},
		{"update with --release", []string{"meteor", "update", "--release", "3.3"}, "3.0.4", true},
		{"--release=", []string{"meteor", "--release=3.3", "run"}, "3.0.4", true},
		{"not meteor", []string{"npm", "ci"}, "3.0.4", true},
		{"bare update", []string{"meteor", "update"}, "3.3", false},
		{"update --all-packages", []string{"meteor", "update", "--all-packages"}, "3.3", false},
		{"update packages only", []string{"meteor", "update", "--packages-only"}, "3.3", true},
		{"update one package", []string{"meteor", "update", "react-meteor-data"}, "3.3", true},
	}
	for _, tt := range tests {
		err := e.CheckRelease(tt.argv, tt.project, ".meteor/release")
		if (err == nil) != tt.ok {
			t.Errorf("%s: err = %v, want ok=%v", tt.name, err, tt.ok)
		}
	}
}
