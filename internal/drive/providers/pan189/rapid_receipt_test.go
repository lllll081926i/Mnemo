package pan189

import (
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestRapidUploadCommitsAndRequiresTargetID(t *testing.T) {
	for _, mode := range []string{"complete", "missing_session", "missing_id", "miss"} {
		t.Run(mode, func(t *testing.T) {
			previous := netx.TestTransportHook
			t.Cleanup(func() { netx.TestTransportHook = previous })
			commits := 0
			netx.TestTransportHook = pan189AuthRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := `{"data":{"fileDataExists":1,"uploadFileId":"session","fileCommitUrl":"https://upload.test/commit"}}`
				if strings.HasSuffix(r.URL.Path, "/commit") {
					commits++
					body = `{"id":"file"}`
					if mode == "missing_id" {
						body = `{}`
					}
				} else if mode == "missing_session" {
					body = `{"data":{"fileDataExists":1}}`
				} else if mode == "miss" {
					body = `{"data":{"fileDataExists":0}}`
				}
				return pan189AuthResponse(r, 200, nil, body), nil
			})
			sess := &Session{SessionKey: "key", SessionSecret: "secret", CloudType: CloudPersonal}
			result, err := (&Driver{}).RapidUploadByHash(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "key", RefreshToken: mustJSON(sess)}}, drive.RapidUploadRequest{Method: "md5", Hash: "8d777f385d3dfec8815d20f7496026dc", FileName: "file.txt", Size: 4, ParentID: PAN189Root})
			if mode == "complete" {
				if err != nil || result == nil || !result.Reuse || result.FileID == "" || commits != 1 {
					t.Fatalf("未提交秒传: %+v %v %d", result, err, commits)
				}
			} else if mode == "miss" {
				if err != nil || result.Reuse || commits != 0 {
					t.Fatalf("未命中仍提交: %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("不完整秒传回执被接受")
			}
		})
	}
}
