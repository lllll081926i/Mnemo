package sync

import (
	"testing"

	"mnemo-go/internal/drive"
)

func TestCollapseTrackedRenamesRemoteAndLocal(t *testing.T) {
	caps := drive.Capabilities{Rename: true}
	changes := []Change{
		{
			Path:   "docs/old.pdf",
			Action: "delete-remote",
			Remote: &Entry{RemoteID: "rid-1", RemoteName: "docs/old.pdf", Size: 1024, Hash: "abc123"},
		},
		{
			Path:   "docs/new.pdf",
			Action: "upload",
			Local:  &Entry{LocalPath: "/tmp/docs/new.pdf", RemoteName: "docs/new.pdf", Size: 1024, Hash: "ABC123"},
		},
		{
			Path:   "img/a.png",
			Action: "delete-local",
			Local:  &Entry{LocalPath: "/tmp/img/a.png", RemoteName: "img/a.png", Size: 2048, Hash: "def456"},
		},
		{
			Path:   "img/b.png",
			Action: "download",
			Remote: &Entry{RemoteID: "rid-2", RemoteName: "img/b.png", Size: 2048, Hash: "def456"},
		},
	}

	got := collapseTrackedRenames(Config{}, changes, caps)
	if len(got) != 2 {
		t.Fatalf("expected 2 collapsed rename actions, got %d: %+v", len(got), got)
	}
	if got[0].Action != "rename-remote" || got[0].FromPath != "docs/old.pdf" || got[0].Path != "docs/new.pdf" {
		t.Fatalf("unexpected remote rename: %+v", got[0])
	}
	if got[1].Action != "rename-local" || got[1].FromPath != "img/a.png" || got[1].Path != "img/b.png" {
		t.Fatalf("unexpected local rename: %+v", got[1])
	}
}

func TestCollapseTrackedRenamesSkipsAmbiguousOrHashless(t *testing.T) {
	caps := drive.Capabilities{Rename: true}
	changes := []Change{
		{Path: "a.bin", Action: "delete-remote", Remote: &Entry{RemoteID: "1", Size: 500, Hash: ""}},
		{Path: "b.bin", Action: "upload", Local: &Entry{Size: 500, Hash: ""}},
	}
	got := collapseTrackedRenames(Config{}, changes, caps)
	if len(got) != 2 || got[0].Action != "delete-remote" || got[1].Action != "upload" {
		t.Fatalf("expected hashless pair untouched, got %+v", got)
	}
}
