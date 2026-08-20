package updater

import (
	"reflect"
	"testing"
)

func TestParseChecksumsUsesOnlyValidSHA256Entries(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got := parseChecksums([]byte(valid + "  Mnemo-windows-x64-Setup.exe\ninvalid  ignored\n"))
	want := map[string]string{"Mnemo-windows-x64-Setup.exe": valid}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseChecksums() = %#v, want %#v", got, want)
	}
}

func TestIsNewerVersionComparesReleaseNumbers(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
	}{
		{candidate: "v0.2.2", current: "v0.2.1", want: true},
		{candidate: "0.2.1", current: "v0.2.1", want: false},
		{candidate: "v0.2.0", current: "v0.2.1", want: false},
		{candidate: "v1.0", current: "v0.9.9", want: true},
	}
	for _, tc := range cases {
		if got := isNewerVersion(tc.candidate, tc.current); got != tc.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}
