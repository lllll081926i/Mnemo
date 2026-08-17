package lanzou

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

const maxLanzouUploadSize = 200 * 1024 * 1024 // 蓝奏单文件上传暂限 200MB

// UploadOneFile uploads a whole file via html5up.php (the 蓝奏 interface does
// not support chunked upload; the file is read into memory like the legacy
// ProviderNet relay path).
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	if ui == nil {
		return errors.New("蓝奏: 无效的上传任务")
	}
	cookie, _, _, baseURL := sessionOf(c)
	if cookie == "" {
		return errors.New("未登录")
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	if size > maxLanzouUploadSize {
		return errors.New("蓝奏单文件上传暂限 200MB（接口不支持分片），请用网页端上传超大文件")
	}
	ui.Info.Size = size
	ui.Info.SizeStr = model.FormatBytes(size)
	buff := make([]byte, size)
	if _, err := io.ReadFull(f, buff); err != nil && err != io.EOF {
		return err
	}
	folderID := ToLanzouFolderId(ui.Info.ParentFileID)

	ui.Upload.DownState = "uploading"
	ui.Upload.IsDowning = true

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("task", "1")
	_ = mw.WriteField("vie", "2")
	_ = mw.WriteField("ve", "2")
	_ = mw.WriteField("id", "WU_FILE_0")
	_ = mw.WriteField("name", ui.Info.Name)
	_ = mw.WriteField("folder_id_bb_n", folderID)
	fw, err := mw.CreateFormFile("upload_file", ui.Info.Name)
	if err != nil {
		return err
	}
	if _, err := fw.Write(buff); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}

	contentType := mw.FormDataContentType()
	j, err := uploadLanzouRaw(ctx, cookie, baseURL, contentType, body.Bytes())
	if err != nil {
		return err
	}
	zt := numOf(j["zt"])
	if zt == 9 {
		newCookie, _, _, reloginErr := d.reloginAccount(ctx, c, baseURL)
		if reloginErr != nil {
			return reloginErr
		}
		j, err = uploadLanzouRaw(ctx, newCookie, baseURL, contentType, body.Bytes())
		if err != nil {
			return err
		}
		zt = numOf(j["zt"])
	}
	if zt != 1 && zt != 2 && zt != 4 {
		msg := strOf(j["info"])
		if msg == "" {
			msg = strOf(j["inf"])
		}
		if msg == "" {
			msg = "上传失败"
		}
		return errors.New(msg)
	}
	ui.Upload.DownSize = size
	ui.Upload.DownProcess = 100
	ui.Upload.IsDowning = false
	ui.Upload.IsCompleted = true
	ui.Upload.DownState = "completed"
	return nil
}

func uploadLanzouRaw(ctx context.Context, cookie, baseURL, contentType string, body []byte) (map[string]any, error) {
	rawURL := strings.TrimSuffix(baseURL, "/") + "/html5up.php"
	res, err := fetchText(ctx, http.MethodPost, rawURL, map[string]string{
		"referer":      "https://pc.woozooo.com",
		"user-agent":   LANZOU_DEFAULT.UserAgent,
		"content-type": contentType,
	}, body, cookie, false)
	if err != nil {
		return nil, err
	}
	if res.status >= 400 {
		return nil, fmt.Errorf("上传失败 HTTP %d", res.status)
	}
	var j map[string]any
	if err := json.Unmarshal([]byte(res.text), &j); err != nil {
		return nil, fmt.Errorf("上传失败: %s", truncate(res.text, 120))
	}
	return j, nil
}
