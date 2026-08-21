package drive_test

import (
	"testing"

	"mnemo-go/internal/drive"
	_ "mnemo-go/internal/drive/providers"
	"mnemo-go/internal/model"
)

func TestProviderDeleteCapabilitiesAreExplicit(t *testing.T) {
	expected := map[string]struct {
		recycle   bool
		permanent bool
	}{
		model.ProviderPikpak:   {recycle: true, permanent: true},
		model.ProviderOnedrive: {recycle: true, permanent: false},
		model.ProviderDropbox:  {recycle: true, permanent: false},
		model.ProviderPan123:   {recycle: true, permanent: true},
		model.ProviderLanzou:   {recycle: false, permanent: true},
		model.ProviderIlanzou:  {recycle: false, permanent: true},
		model.ProviderPan139:   {recycle: true, permanent: true},
		model.ProviderPan189:   {recycle: true, permanent: true},
		model.ProviderYike:     {recycle: false, permanent: true},
		model.ProviderAliopen:  {recycle: true, permanent: true},
		model.ProviderGuangya:  {recycle: false, permanent: true},
		model.ProviderWebdav:   {recycle: false, permanent: true},
		model.ProviderS3:       {recycle: false, permanent: true},
	}
	if got := drive.Count(); got != len(expected) {
		t.Fatalf("registered provider count = %d, want %d", got, len(expected))
	}
	for provider, want := range expected {
		caps := drive.RegistryCaps(provider)
		if caps.RecycleBin != want.recycle || caps.PermanentDelete != want.permanent {
			t.Errorf("%s delete caps = recycle:%v permanent:%v, want recycle:%v permanent:%v", provider, caps.RecycleBin, caps.PermanentDelete, want.recycle, want.permanent)
		}
	}
}

func TestRenameBatchRejectsMismatchedInput(t *testing.T) {
	_, err := drive.RenameBatch("", "", []drive.FileRef{{ID: "file-1"}}, nil)
	if err == nil {
		t.Fatal("RenameBatch should reject mismatched refs and names")
	}
}

func TestShareExpirationOptionsAreDeclaredExplicitly(t *testing.T) {
	caps := drive.NewCapabilities("test", nil, func(c *drive.Capabilities) {
		c.SetShareExpirationOptions(0, 1, 7)
	})
	if got := caps.ShareExpirationOptions; len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 7 {
		t.Fatalf("share expiration options = %v, want [0 1 7]", got)
	}
}
