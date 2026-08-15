package e2e

import (
	"encoding/json"
	"net/http"
	"testing"

	"mnemo-go/internal/drive"
	s3pkg "mnemo-go/internal/drive/providers/s3"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// TestYikeListMock exercises the yike driver against a mock photo.baidu.com.
func TestYikeListMock(t *testing.T) {
	mock := MockAPI(t, "photo.baidu.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/youai/user/v1/getuinfo":
			json.NewEncoder(w).Encode(map[string]any{"errno": 0, "youa_id": "12345", "uk": "12345"})
		case "/youai/album/v1/list":
			json.NewEncoder(w).Encode(map[string]any{"errno": 0, "list": []map[string]any{
				{"album_id": "a1", "title": "旅行", "ctime": 1700000000},
			}, "cursor": ""})
		case "/youai/file/v1/list":
			json.NewEncoder(w).Encode(map[string]any{"errno": 0, "list": []map[string]any{}, "cursor": ""})
		default:
			json.NewEncoder(w).Encode(map[string]any{"errno": 0})
		}
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "yike", &model.TokenInfo{
		AccessToken: "BDUSS=abc", TokenFrom: "yike", UserID: "yike_test",
		RefreshToken: `{"cookie":"BDUSS=abc","uk":"12345"}`,
	})

	names := listNames(t, uid, did, "yike_root")
	if len(names) != 1 || names[0] != "旅行" {
		t.Fatalf("yike names: %v", names)
	}
}

// TestGuangyaListMock exercises the guangya driver against a mock api.guangyapan.com.
func TestGuangyaListMock(t *testing.T) {
	mock := MockAPI(t, "api.guangyapan.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/userres/v1/file/get_file_list" {
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"list": []map[string]any{
						{"fileId": "f1", "fileName": "photo.jpg", "fileSize": 800, "resType": 1, "cTime": 1700000000, "uTime": 1700000000},
						{"fileId": "f2", "fileName": "photos", "fileSize": 0, "resType": 2, "cTime": 1700000000, "uTime": 1700000000},
					},
					"total": 2,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "guangya", &model.TokenInfo{
		AccessToken: "test-token", TokenFrom: "guangya", UserID: "guangya_test",
		RefreshToken: `{"access_token":"test-token","device_id":"dev","client_id":"aMe-8VSlkrbQXpUR"}`,
	})

	names := listNames(t, uid, did, "guangya_root")
	if len(names) != 2 || names[0] != "photo.jpg" || names[1] != "photos" {
		t.Fatalf("guangya names: %v", names)
	}
}

// TestPan139ListMock exercises the pan139 driver against a mock API host.
// The personalCloudHost is stored in the token and points at the mocked host,
// so /file/list is served by the same mock.
func TestPan139ListMock(t *testing.T) {
	// authorization = base64("user:account:tok|a|b|c|<future-ms>")
	authorization := "Basic " + base64Std("user:account:tok|a|b|c|4102444800000")

	mock := MockAPI(t, "api.mail.10086.cn", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/file/list" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"dataList": []map[string]any{
						{"fileId": "f1", "name": "data.zip", "size": 900, "updateTime": "2026-01-01 00:00:00", "contentType": "application/zip"},
						{"fileId": "f2", "name": "backup", "size": 0, "updateTime": "2026-01-01 00:00:00", "contentType": "folder"},
					},
					"nextPageCursor": "",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock

	uid, did, _ := SeedAccount(t, "pan139", &model.TokenInfo{
		AccessToken: authorization, TokenFrom: "pan139", UserID: "pan139_test",
		RefreshToken: `{"authorization":"` + authorization + `","account":"account","personalCloudHost":"https://api.mail.10086.cn"}`,
	})

	names := listNames(t, uid, did, "pan139_root")
	if len(names) != 2 || names[0] != "data.zip" {
		t.Fatalf("pan139 names: %v", names)
	}
}

// TestS3ListMock exercises the s3 driver against a mock S3-compatible server.
func TestS3ListMock(t *testing.T) {
	s3XML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>test-bucket</Name>
  <Prefix></Prefix>
  <KeyCount>2</KeyCount>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>report.pdf</Key>
    <LastModified>2026-01-01T00:00:00.000Z</LastModified>
    <ETag>&quot;abc&quot;</ETag>
    <Size>2048</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
  <CommonPrefixes>
    <Prefix>docs/</Prefix>
  </CommonPrefixes>
</ListBucketResult>`
	mock := MockAPI(t, "s3.example.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(s3XML))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	_ = mock
	// route the AWS SDK's own transport to the mock as well
	s3pkg.TransportOverride = netx.TestTransportHook
	t.Cleanup(func() { s3pkg.TransportOverride = nil })

	uid, did, _ := SeedAccount(t, "s3", &model.TokenInfo{
		TokenFrom: "s3", UserID: "s3_test",
		Conn: &model.ConnConfig{
			Endpoint: "s3.example.com", Username: "key", Password: "secret",
			Bucket: "test-bucket",
		},
	})

	names := listNames(t, uid, did, "/")
	if len(names) != 2 {
		t.Fatalf("s3 names: %v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if !seen["report.pdf"] || !seen["docs"] {
		t.Fatalf("s3 names: %v", names)
	}
}

func base64Std(s string) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	buf := []byte(s)
	var out []byte
	for i := 0; i < len(buf); i += 3 {
		var b [3]byte
		rem := 0
		for k := 0; k < 3 && i+k < len(buf); k++ {
			b[k] = buf[i+k]
			rem = k + 1
		}
		out = append(out, chars[(b[0]&0xFC)>>2])
		if rem >= 2 {
			out = append(out, chars[((b[0]&0x03)<<4)|((b[1]&0xF0)>>4)])
		} else {
			out = append(out, '=')
		}
		if rem >= 3 {
			out = append(out, chars[((b[1]&0x0F)<<2)|((b[2]&0xC0)>>6)])
		} else {
			out = append(out, '=')
		}
		if rem >= 3 {
			out = append(out, chars[b[2]&0x3F])
		} else {
			out = append(out, '=')
		}
	}
	return string(out)
}

var _ = drive.ErrNotFound