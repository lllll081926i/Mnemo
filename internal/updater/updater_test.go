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
