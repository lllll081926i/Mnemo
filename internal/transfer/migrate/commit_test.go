package migrate

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

type uncertainStreamDriver struct {
	drive.Driver
	url             string
	streams, spools int
}

var currentUncertainStreamDriver *uncertainStreamDriver

func (driver *uncertainStreamDriver) GetFile(context.Context, drive.Context, string) (*model.File, error) {
	return &model.File{FileID: "file", Name: "file", Size: 4}, nil
}
func (driver *uncertainStreamDriver) GetDownloadURL(context.Context, drive.Context, string, int) (*model.DownloadURL, error) {
	return &model.DownloadURL{URL: driver.url, Size: 4}, nil
}
func (*uncertainStreamDriver) Capabilities() drive.Capabilities {
	return drive.Capabilities{Upload: true, Download: true}
}
func (driver *uncertainStreamDriver) UploadStream(_ context.Context, _ drive.Context, _, _ string, _ int64, reader io.Reader) error {
	driver.streams++
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}
	return errors.New("目标已读取正文，但最终响应丢失")
}
func (driver *uncertainStreamDriver) UploadOneFile(context.Context, drive.Context, *model.UploadingUI) error {
	driver.spools++
	return nil
}

func TestUnknownStreamCommitDoesNotFallbackOrRetry(testContext *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { _, _ = io.WriteString(response, "data") }))
	defer server.Close()
	driver := &uncertainStreamDriver{url: server.URL}
	const providerID = model.ProviderS3
	currentUncertainStreamDriver = driver
	if !drive.IsRegistered(providerID) {
		drive.Register(drive.Registration{ID: providerID, Factory: func() drive.Driver { return currentUncertainStreamDriver }, Caps: driver.Capabilities()})
	}
	drive.SetTokenResolver(func(string, string) (*model.TokenInfo, error) { return &model.TokenInfo{TokenFrom: providerID}, nil })
	testContext.Cleanup(func() { drive.SetTokenResolver(nil) })
	job := &Job{ID: "unknown", SrcUser: providerID + ":source", SrcDrive: "source", DstUser: providerID + ":target", DstDrive: "target", DstParent: "parent", FileIDs: []string{"file"}}
	engine := NewEngine(nil, nil)
	for repeat := 0; repeat < 2; repeat++ {
		if err := engine.Run(testContext.Context(), job); err == nil {
			testContext.Fatal("未知回执被视为成功")
		}
		if driver.streams != 1 || driver.spools != 0 || job.Items["file"].CommitState != "unknown" {
			testContext.Fatalf("重复上传或降级: %+v %+v", driver, job.Items)
		}
	}
}

func TestSpoolSpaceFailsBeforeDownload(testContext *testing.T) {
	if err := requireSpoolSpace(testContext.TempDir(), math.MaxInt64); err == nil {
		testContext.Fatal("临时盘不足没有阻止中转")
	}
	if err := requireSpoolSpace(testContext.TempDir(), -1); err == nil {
		testContext.Fatal("未知大小被视为零大小")
	}
}
