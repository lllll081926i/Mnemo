package app

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestPR1ReviewMigrationDetectsNestedConflict(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() {
		netx.TestTransportHook = previous
		drive.SetTokenResolver(nil)
	})
	drive.SetTokenResolver(func(userID, driveID string) (*model.TokenInfo, error) {
		return &model.TokenInfo{TokenFrom: "onedrive", AccessToken: userID}, nil
	})
	netx.TestTransportHook = favoriteTestTransport(func(request *http.Request) (*http.Response, error) {
		responses := map[string]string{
			"/v1.0/me/drive/items/source-a":          `{"id":"source-a","name":"A","folder":{}}`,
			"/v1.0/me/drive/items/source-a/children": `{"value":[{"id":"source-b","name":"B","folder":{}}]}`,
			"/v1.0/me/drive/items/source-b":          `{"id":"source-b","name":"B","folder":{}}`,
			"/v1.0/me/drive/items/source-b/children": `{"value":[{"id":"source-x","name":"x.txt","size":3,"file":{}}]}`,
			"/v1.0/me/drive/items/source-x":          `{"id":"source-x","name":"x.txt","size":3,"file":{}}`,
			"/v1.0/me/drive/root/children":           `{"value":[{"id":"target-a","name":"A","folder":{}}]}`,
			"/v1.0/me/drive/items/target-a/children": `{"value":[{"id":"target-b","name":"B","folder":{}}]}`,
			"/v1.0/me/drive/items/target-b/children": `{"value":[{"id":"target-x","name":"x.txt","size":3,"file":{}}]}`,
		}
		body, found := responses[request.URL.Path]
		if !found {
			return nil, fmt.Errorf("非预期请求: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})
	application := NewApp()
	preview, err := application.PreviewMigration("onedrive_review-source", "source", "onedrive_review-target", "target", "root", []string{"source-a"})
	if err != nil {
		testContext.Fatal(err)
	}
	if preview.Files != 1 || preview.Bytes != 3 || !slices.Contains(preview.Conflicts, "A/B/x.txt") {
		testContext.Fatalf("嵌套冲突预检不完整: %+v", preview)
	}
}
