package source

import (
	"reflect"
	"testing"
)

func TestParseRefs(t *testing.T) {
	refs := []string{
		"refs/tags/release/METEOR@3.1",
		"refs/tags/release/METEOR@1.12.1",
		"refs/tags/release/METEOR@2.16",
		"refs/tags/release/METEOR@3.0-rc.4",
		"refs/tags/release/METEOR@3.0.4",
		"refs/tags/release/METEOR@2.2.1",
		"refs/tags/release/METEOR@new-version-solver-1",
		"refs/tags/release/METEOR@CORDOVA-4.1",
		"refs/tags/release/METEOR@3.1-beta.1",
		"refs/tags/v3.1",
	}
	want := []string{"2.2.1", "2.16", "3.0-rc.4", "3.0.4", "3.1-beta.1", "3.1"}
	if got := parseRefs(refs); !reflect.DeepEqual(got, want) {
		t.Errorf("parseRefs = %v, want %v", got, want)
	}
	if got := Filter(want, false); !reflect.DeepEqual(got, []string{"2.2.1", "2.16", "3.0.4", "3.1"}) {
		t.Errorf("Filter = %v", got)
	}
}

func TestTarballURL(t *testing.T) {
	t.Setenv(EnvMirror, "")
	if got := TarballURL("3.1", "x86_64"); got != DefaultBaseURL+"/3.1/meteor-bootstrap-os.linux.x86_64.tar.gz" {
		t.Errorf("TarballURL = %s", got)
	}
	t.Setenv(EnvMirror, "https://nexus.example/meteor/")
	if got := TarballURL("2.16", "x86_64"); got != "https://nexus.example/meteor/2.16/meteor-bootstrap-os.linux.x86_64.tar.gz" {
		t.Errorf("mirror TarballURL = %s", got)
	}
}
