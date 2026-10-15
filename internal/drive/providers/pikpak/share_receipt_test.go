package pikpak

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func TestShareTaskReceiptKeepsAllTaskIDs(testContext *testing.T) {
	var receipt shareTaskReceipt
	if err := json.Unmarshal([]byte(`{"id":"untyped-file-id","task_id":"one","task":{"id":"two"},"tasks":[{"task_id":"one"},{"id":"three"}]}`), &receipt); err != nil {
		testContext.Fatal(err)
	}
	if !reflect.DeepEqual(receipt.taskIDs(), []string{"one", "three", "two"}) {
		testContext.Fatalf("任务标识丢失或把不明 ID 当任务: %+v", receipt.taskIDs())
	}
}

func TestShareTaskVerifierPreservesPendingAndRejectsWrongIdentity(testContext *testing.T) {
	previous := netx.TestTransportHook
	testContext.Cleanup(func() { netx.TestTransportHook = previous })
	for _, sample := range []struct {
		body, status string
		invalid      bool
	}{
		{`{"id":"task","phase":"PHASE_TYPE_RUNNING"}`, "submitted", false},
		{`{"task":{"id":"task","phase":"PHASE_TYPE_COMPLETE"}}`, "completed", false},
		{`{"id":"task","phase":"PHASE_TYPE_ERROR"}`, "failed", false},
		{`{"id":"other","phase":"PHASE_TYPE_COMPLETE"}`, "", true},
		{`{"phase":"PHASE_TYPE_COMPLETE"}`, "", true},
		{`{}`, "", true},
	} {
		netx.TestTransportHook = pikpakRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method != http.MethodGet || request.URL.Path != "/drive/v1/tasks/task" {
				testContext.Fatalf("对账发生写请求或查询错误任务: %s %s", request.Method, request.URL)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(sample.body)), Request: request}, nil
		})
		items, err := (&Driver{}).VerifyShareReceipt(testContext.Context(), drive.Context{Token: &model.TokenInfo{AccessToken: "token", DeviceID: "device"}}, []model.ShareImportItem{{SourceID: "source", TaskIDs: []string{"task"}, Status: "submitted"}})
		if sample.invalid {
			if err == nil {
				testContext.Fatalf("错误身份被当成最终回执: %s", sample.body)
			}
		} else if err != nil || items[0].Status != sample.status {
			testContext.Fatalf("任务状态映射错误: %+v %v", items, err)
		}
	}
}
