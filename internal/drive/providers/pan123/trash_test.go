package pan123

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestRecycleListUsesRecycleParametersAndReadsAllPages(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	pages := 0
	netx.TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		pages++
		query := req.URL.Query()
		if req.URL.Path != "/b/api/file/list/new" || query.Get("event") != "recycleListFile" || query.Get("operateType") != "1" || query.Get("trashed") != "true" || query.Get("orderBy") != "trashed_at" {
			t.Errorf("回收站请求参数无效: %s", req.URL)
		}
		body := `{"code":0,"data":{"InfoList":[{"FileId":9007199254740993,"FileName":"删除的文件.txt","Type":0}],"Next":"cursor"}}`
		if pages == 2 {
			if query.Get("Page") != "2" || query.Get("next") != "cursor" {
				t.Error("未读取第二页")
			}
			body = `{"code":200,"data":{"InfoList":[{"FileId":12,"FileName":"删除的目录","Type":1}],"Next":"-1"}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	items, err := (&Driver{}).ListTrash(t.Context(), drive.Context{UserID: "pan123_test", DriveID: "test", Token: &model.TokenInfo{AccessToken: "token"}}, nil)
	if err != nil || len(items) != 2 || items[0].FileID != "9007199254740993" || !items[1].IsDir || pages != 2 {
		t.Fatalf("回收站列表 = %+v, err=%v, pages=%d", items, err, pages)
	}
}

func TestRecycleListOmitsJSONEncodedFilters(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	netx.TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":200,"data":{"InfoList":[],"Next":"-1"}}`
		for _, key := range []string{"fileAttributes", "fileTypes"} {
			if req.URL.Query().Has(key) {
				body = `{"code":400,"message":"strconv.ParseInt: parsing \"[]\": invalid syntax"}`
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	items, err := (&Driver{}).ListTrash(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "token"}}, nil)
	if err != nil || len(items) != 0 {
		t.Fatalf("无筛选的空回收站应正常加载: items=%+v, err=%v", items, err)
	}
}

func TestRecycleListRejectsMissingOrRepeatedPages(t *testing.T) {
	previous := netx.TestTransportHook
	t.Cleanup(func() { netx.TestTransportHook = previous })
	for _, body := range []string{
		`{"code":0,"data":{}}`,
		`{"code":0,"data":{"InfoList":[{"FileId":1,"FileName":"重复文件"}],"Next":"same"}}`,
	} {
		calls := 0
		netx.TestTransportHook = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})
		if _, err := (&Driver{}).ListTrash(t.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "token"}}, nil); err == nil || calls > 2 {
			t.Fatalf("异常分页必须明确失败且及时停止: calls=%d, err=%v", calls, err)
		}
	}
}
