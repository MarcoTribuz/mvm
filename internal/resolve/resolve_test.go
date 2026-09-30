package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRelease(t *testing.T) {
	tests := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "METEOR@3.1.2\n", want: "3.1.2"},
		{in: "# comment\n\nMETEOR@2.16\n", want: "2.16"},
		{in: "METEOR@3.0-rc.4", want: "3.0-rc.4"},
		{in: "none\n", wantErr: true},
		{in: "", wantErr: true},
		{in: "METEOR@", wantErr: true},
		{in: "OTHER@1.0", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseRelease(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseRelease(%q) = %q, %v; want %q, err=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{"3.1": "3.1", "METEOR@3.1": "3.1", " v2.16 ": "2.16"} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOrder(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	sub := filepath.Join(app, "server", "lib")
	def := filepath.Join(root, "default")
	write(t, filepath.Join(app, ".meteor", "release"), "METEOR@3.1.2\n")
	write(t, filepath.Join(root, ".mvmrc"), "2.16\n")
	write(t, def, "3.0.4\n")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv(EnvVersion, "")
	check := func(o Options, wantV string, wantS Source) {
		t.Helper()
		got, err := Resolve(o)
		if err != nil {
			t.Fatal(err)
		}
		if got.Version != wantV || got.Source != wantS {
			t.Errorf("Resolve(%+v) = %s from %s; want %s from %s", o, got.Version, got.Source, wantV, wantS)
		}
	}

	check(Options{Flag: "METEOR@2.14", Dir: sub, DefaultFile: def}, "2.14", SourceFlag)
	check(Options{Dir: sub, DefaultFile: def}, "3.1.2", SourceRelease) // found walking up
	check(Options{Dir: root, DefaultFile: def}, "2.16", SourceRC)
	check(Options{Dir: t.TempDir(), DefaultFile: def}, "3.0.4", SourceDefault)

	t.Setenv(EnvVersion, "3.3")
	check(Options{Dir: sub, DefaultFile: def}, "3.3", SourceEnv)

	t.Setenv(EnvVersion, "")
	if _, err := Resolve(Options{Dir: t.TempDir()}); err != ErrNoVersion {
		t.Errorf("want ErrNoVersion, got %v", err)
	}
}
