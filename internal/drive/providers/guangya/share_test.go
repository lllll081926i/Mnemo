package guangya

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type guangyaRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn guangyaRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func guangyaResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

type guangyaMemoryUploadStore struct {
	sessionID string
	parts     []int
	cleared   bool
}

func (s *guangyaMemoryUploadStore) SaveUploadSession(_ string, parts []int) error {
	return s.SaveUploadSessionState("", "", parts)
}

func (s *guangyaMemoryUploadStore) LoadUploadSession(string) []int {
	_, parts := s.LoadUploadSessionState("")
	return parts
}

func (s *guangyaMemoryUploadStore) ClearUploadSession(string) {
	s.sessionID = ""
	s.parts = nil
	s.cleared = true
}

func (s *guangyaMemoryUploadStore) SaveUploadSessionState(_ string, sessionID string, parts []int) error {
	s.sessionID = sessionID
	s.parts = append([]int(nil), parts...)
	return nil
}

func (s *guangyaMemoryUploadStore) LoadUploadSessionState(string) (string, []int) {
	return s.sessionID, append([]int(nil), s.parts...)
}

func TestGuangyaDeclaresMD5RapidUpload(t *testing.T) {
	caps := drive.RegistryCaps(providerID)
	if strings.Join(caps.ProvideHashes, ",") != "md5" || strings.Join(caps.RapidUploadHashes, ",") != "md5" {
		t.Fatalf("Guangya hashes = provide:%v rapid:%v", caps.ProvideHashes, caps.RapidUploadHashes)
	}
	if strings.Join(caps.UploadConflictPolicies, ",") != "rename" {
		t.Fatalf("Guangya conflict policies = %v", caps.UploadConflictPolicies)
	}
	if _, ok := normalizeGuangyaMD5("ABC"); ok {
		t.Fatal("short MD5 must be rejected")
	}
}

func TestGuangyaRapidUploadMissPersistsCreatedTask(t *testing.T) {
	store := &guangyaMemoryUploadStore{}
	drive.SetUploadSessionStore(store)
	t.Cleanup(func() { drive.SetUploadSessionStore(nil) })
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	var paths []string
	netx.TestTransportHook = guangyaRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		switch req.URL.Path {
		case "/nd.bizuserres.s/v1/get_res_center_token":
			return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"taskId":"task-miss","uploadUrl":"https://upload.example/object"}}`), nil
		case "/userres/v1/check_can_flash_upload":
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			if body["taskId"] != "task-miss" || body["md5"] != strings.Repeat("a", 32) {
				return nil, fmt.Errorf("unexpected flash body: %#v", body)
			}
			return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"canFlashUpload":false}}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL)
		}
	})

	result, err := (&Driver{}).RapidUploadByHash(context.Background(), drive.Context{
		UserID: "guangya:user", DriveID: "guangya:user",
		Token: &model.TokenInfo{AccessToken: "access-token"},
	}, drive.RapidUploadRequest{
		ParentID: RootID, FileName: "movie.mkv", Size: 123, Method: "MD5", Hash: strings.Repeat("A", 32),
	})
	if err != nil {
		t.Fatalf("RapidUploadByHash() error = %v", err)
	}
	if result == nil || result.Reuse {
		t.Fatalf("rapid result = %+v, want explicit miss", result)
	}
	if strings.Join(paths, ",") != "/nd.bizuserres.s/v1/get_res_center_token,/userres/v1/check_can_flash_upload" {
		t.Fatalf("request paths = %v", paths)
	}
	saved, ok := decodeGuangyaUploadSession(store.sessionID)
	if !ok || saved.Data.TaskID != "task-miss" || saved.Data.UploadURL != "https://upload.example/object" {
		t.Fatalf("saved upload session = %#v", saved)
	}
}

func TestGuangyaRapidUploadHitWaitsForFile(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	var paths []string
	netx.TestTransportHook = guangyaRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		switch req.URL.Path {
		case "/nd.bizuserres.s/v1/get_res_center_token":
			return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"taskId":"task-hit"}}`), nil
		case "/userres/v1/check_can_flash_upload":
			return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"canFlashUpload":true}}`), nil
		case "/nd.bizuserres.s/v1/file/get_info_by_task_id":
			return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"fileId":"file-hit","status":0}}`), nil
		default:
			return nil, fmt.Errorf("unexpected request %s", req.URL)
		}
	})

	result, err := (&Driver{}).RapidUploadByHash(context.Background(), drive.Context{
		UserID: "guangya:user", DriveID: "guangya:user",
		Token: &model.TokenInfo{AccessToken: "access-token"},
	}, drive.RapidUploadRequest{
		ParentID: "folder-1", FileName: "movie.mkv", Size: 123, Method: "md5", Hash: strings.Repeat("a", 32),
	})
	if err != nil {
		t.Fatalf("RapidUploadByHash() error = %v", err)
	}
	if result == nil || !result.Reuse || result.FileID != "file-hit" {
		t.Fatalf("rapid result = %+v", result)
	}
	if len(paths) != 3 {
		t.Fatalf("request paths = %v", paths)
	}
}

func TestCreateShareUsesGuangyaShareAPI(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	var requests int
	netx.TestTransportHook = guangyaRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodPost || req.URL.Host != "api.guangyapan.com" || req.URL.Path != "/nd.bizuserres.s/v1/share_file" {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL.String())
		}
		if req.Header.Get("Authorization") != "Bearer access-token" || req.Header.Get("X-Device-Id") != "device-test" {
			return nil, fmt.Errorf("share request auth headers are incomplete")
		}
		var body struct {
			FileIDs          []string `json:"fileIds"`
			Title            string   `json:"title"`
			ValidateDuration int      `json:"validateDuration"`
			ShareType        int      `json:"shareType"`
			Code             string   `json:"code"`
			AutoFillCode     bool     `json:"autoFillCode"`
			DownloadType     int      `json:"downloadType"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if strings.Join(body.FileIDs, ",") != "file-1,folder-1" || body.Title != "测试分享" || body.ValidateDuration != 7 || body.ShareType != 1 || body.Code != "p4ss" || !body.AutoFillCode || body.DownloadType != 1 {
			return nil, fmt.Errorf("share body = %+v", body)
		}
		return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{"shareId":"share-guangya","shareUrl":"https://www.guangyapan.com/s/share-guangya","code":"p4ss"}}`), nil
	})

	sess := &Session{AccessToken: "access-token", RefreshToken: "refresh-token", DeviceID: "device-test", ClientID: "client-test"}
	item, err := (&Driver{}).CreateShare(context.Background(), drive.Context{
		UserID: "guangya:user", DriveID: "guangya:user",
		Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)},
	}, drive.ShareParams{
		FileIDs: []string{"file-1", "folder-1"}, ShareName: "测试分享", Expiration: "7", Password: "p4ss",
	})
	if err != nil {
		t.Fatalf("CreateShare() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
	if item.ShareID != "share-guangya" || item.ShareURL != "https://www.guangyapan.com/s/share-guangya" || item.SharePwd != "p4ss" || len(item.FileIDList) != 2 || item.SharePolicy != "public" {
		t.Fatalf("share = %+v", item)
	}
}

func TestGuangyaCancelShareDeletesRemoteShare(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })

	netx.TestTransportHook = guangyaRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Host != "api.guangyapan.com" || req.URL.Path != guangyaDeleteSharePath {
			return nil, fmt.Errorf("unexpected request %s %s", req.Method, req.URL.String())
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
		if strings.Join(body.IDs, ",") != "share-guangya" {
			return nil, fmt.Errorf("delete body = %+v", body)
		}
		return guangyaResponse(req, http.StatusOK, `{"code":0,"data":{}}`), nil
	})

	sess := &Session{AccessToken: "access-token", RefreshToken: "refresh-token", DeviceID: "device-test", ClientID: "client-test"}
	err := (&Driver{}).CancelShare(context.Background(), drive.Context{
		UserID: "guangya:user", DriveID: "guangya:user",
		Token: &model.TokenInfo{AccessToken: sess.AccessToken, RefreshToken: mustJSON(sess)},
	}, model.ShareHistoryEntry{ShareID: "share-guangya"})
	if err != nil {
		t.Fatalf("CancelShare() error = %v", err)
	}
}

func TestGuangyaShareDurationRejectsInvalidValue(t *testing.T) {
	if _, err := guangyaShareDuration("not-a-time"); err == nil {
		t.Fatal("invalid duration must be rejected")
	}
	if _, err := guangyaShareDuration("-1"); err == nil {
		t.Fatal("negative duration must be rejected")
	}
}
