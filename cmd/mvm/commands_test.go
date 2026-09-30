package main

import (
	"reflect"
	"testing"
)

func TestSplitExecArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		dash     int
		wantV    string
		wantArgv []string
		wantErr  bool
	}{
		{"dash first", []string{"meteor", "build"}, 0, "", []string{"meteor", "build"}, false},
		{"version then literal dash", []string{"2.16", "--", "meteor", "--version"}, -1, "2.16", []string{"meteor", "--version"}, false},
		{"version with cobra dash", []string{"2.16", "meteor"}, 1, "2.16", []string{"meteor"}, false},
		{"no dash", []string{"meteor", "build", "--server-only"}, -1, "", []string{"meteor", "build", "--server-only"}, false},
		{"two versions", []string{"2.16", "3.1", "--", "meteor"}, -1, "", nil, true},
		{"empty", nil, -1, "", nil, true},
		{"nothing after dash", []string{"3.1", "--"}, -1, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, argv, err := splitExecArgs(tt.args, tt.dash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if v != tt.wantV || !reflect.DeepEqual(argv, tt.wantArgv) {
				t.Errorf("got %q %v, want %q %v", v, argv, tt.wantV, tt.wantArgv)
			}
		})
	}
}
