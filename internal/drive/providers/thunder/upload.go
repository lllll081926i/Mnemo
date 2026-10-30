package thunder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

type uploadCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
	Bucket          string `json:"bucket"`
	Endpoint        string `json:"endpoint"`
	Key             string `json:"key"`
}

type uploadReceipt struct {
	UploadType string `json:"upload_type"`
	File       file   `json:"file"`
	Resumable  struct {
		Params uploadCredentials `json:"params"`
	} `json:"resumable"`
}

func (*Driver) ManagesUploadAttempt() bool { return true }

func (d *Driver) UploadOneFile(ctx context.Context, account drive.Context, ui *model.UploadingUI) error {
	if ui == nil || ui.Info.LocalFilePath == "" {
		return errors.New("迅雷上传缺少本地文件")
	}
	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return errors.New("迅雷上传仅支持普通文件")
	}
	ui.Info.Size = before.Size()
	if strings.TrimSpace(ui.Info.Name) == "" {
		ui.Info.Name = filepath.Base(ui.Info.LocalFilePath)
	}
	return withClientMode(ctx, account, func(cl *client) error {
		proceed, err := d.prepareUpload(ctx, cl, ui)
		if err != nil || !proceed {
			return err
		}
		hash, err := driveutil.ComputeGCID(ctx, ui.Info.LocalFilePath, ui)
		if err != nil {
			return err
		}
		after, err := os.Stat(ui.Info.LocalFilePath)
		if err != nil {
			return err
		}
		if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return errors.New("本地文件在计算指纹时变化，请重新选择文件")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if ui.IsUploadStopRequested() {
			return context.Canceled
		}
		if err := ui.BeginUploadAttempt(); err != nil {
			return err
		}
		var receipt uploadReceipt
		if err := cl.request(ctx, http.MethodPost, "/files", map[string]any{
			"kind": "drive#file", "parent_id": parentID(ui.Info.ParentFileID), "name": ui.Info.Name,
			"size": before.Size(), "hash": hash, "upload_type": "UPLOAD_TYPE_RESUMABLE", "space": "",
		}, &receipt); err != nil {
			return err
		}
		if receipt.File.ID == "" {
			return errors.New("迅雷上传回执缺少文件 ID，请先核对目标目录")
		}
		ui.Upload.FileID = receipt.File.ID
		if receipt.File.Name != "" {
			ui.Info.Name = receipt.File.Name
		}
		if receipt.File.Phase == "PHASE_TYPE_COMPLETE" {
			ui.Info.IsMiaoChuan = true
		} else if receipt.UploadType == "UPLOAD_TYPE_RESUMABLE" {
			if err := uploadS3(ctx, f, before.Size(), receipt.Resumable.Params, ui); err != nil {
				return err
			}
		} else {
			// 缺少上传参数并不能证明秒传成功，必须读取服务端完成状态。
			ui.Info.IsMiaoChuan = true
		}
		if receipt.File.Phase != "PHASE_TYPE_COMPLETE" {
			if err := waitUploadComplete(ctx, cl, ui.Upload.FileID, hash, before.Size()); err != nil {
				return err
			}
		}
		ui.ReportUploadProgress(before.Size(), before.Size())
		ui.Upload.IsCompleted = true
		return nil
	}, true)
}

// prepareUpload 在任何写入之前检查名称冲突，覆盖不先删除原文件。
func (d *Driver) prepareUpload(ctx context.Context, cl *client, ui *model.UploadingUI) (bool, error) {
	names := make(map[string]string)
	seen := make(map[string]bool)
	marker := ""
	for page := 0; ; page++ {
		if page >= 200 || seen[marker] {
			return false, errors.New("迅雷目录分页异常，请刷新目标目录")
		}
		seen[marker] = true
		result, err := cl.filePage(ctx, ui.Info.ParentFileID, marker, false)
		if err != nil {
			return false, err
		}
		for _, entry := range result.Files {
			if entry.ID == "" || entry.Name == "" {
				return false, errors.New("迅雷目录包含不完整文件信息")
			}
			names[entry.Name] = entry.ID
		}
		marker = result.Next
		if marker == "" {
			break
		}
	}
	id := names[ui.Info.Name]
	if id == "" {
		return true, nil
	}
	switch driveutil.ResolveConflictPolicy(ui.Info.ConflictPolicy) {
	case driveutil.ConflictSkip:
		ui.Upload.FileID = id
		return false, nil
	case driveutil.ConflictRename:
		original := ui.Info.Name
		ext := filepath.Ext(original)
		for index := 1; index <= 9999; index++ {
			candidate := fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(original, ext), index, ext)
			if names[candidate] == "" {
				ui.Info.Name = candidate
				return true, nil
			}
		}
		return false, errors.New("迅雷无法生成不重复的文件名")
	case driveutil.ConflictRefuse:
		return false, errors.New("迅雷目标目录已存在同名文件")
	default:
		return false, errors.New("迅雷暂不支持安全覆盖同名文件，请选择自动重命名或跳过")
	}
}

func waitUploadComplete(ctx context.Context, cl *client, id, hash string, size int64) error {
	for attempt := 0; attempt < 20; attempt++ {
		entry, err := detail(ctx, cl, id)
		if err != nil {
			return err
		}
		if entry.Trashed || entry.Phase == "PHASE_TYPE_ERROR" {
			return errors.New("迅雷未能完成上传，请核对目标文件状态")
		}
		if entry.Phase == "PHASE_TYPE_COMPLETE" {
			if int64(entry.Size) != size || entry.Hash != "" && !strings.EqualFold(entry.Hash, hash) {
				return errors.New("迅雷完成回执的文件大小或指纹不一致")
			}
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return errors.New("迅雷尚未确认上传完成，请稍后核对目标文件，避免重复上传")
}

func uploadS3(ctx context.Context, f *os.File, size int64, params uploadCredentials, ui *model.UploadingUI) error {
	endpoint := strings.TrimRight(strings.TrimSpace(params.Endpoint), "/")
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return errors.New("迅雷返回了无效的上传地址")
	}
	parsed.Host = strings.TrimPrefix(parsed.Host, params.Bucket+".")
	if params.Bucket == "" || params.Key == "" || params.AccessKeyID == "" || params.AccessKeySecret == "" || params.SecurityToken == "" {
		return errors.New("迅雷返回的上传凭据不完整")
	}
	hc := netx.NewClientWithSystemProxy(600 * time.Second)
	sc := s3.New(s3.Options{Region: "xunlei", BaseEndpoint: aws.String(parsed.String()),
		Credentials: credentials.NewStaticCredentialsProvider(params.AccessKeyID, params.AccessKeySecret, params.SecurityToken),
		HTTPClient:  netx.UploadHTTPClient{Client: hc.HTTP}, RetryMaxAttempts: 1,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired})
	if size < 16<<20 {
		body := driveutil.NewProgressOnlyReader(io.NewSectionReader(f, 0, size), size, func(done int64) { ui.ReportUploadProgress(done, size) })
		_, err := sc.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(params.Bucket), Key: aws.String(params.Key), Body: body, ContentLength: aws.Int64(size)})
		return err
	}
	created, err := sc.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{Bucket: aws.String(params.Bucket), Key: aws.String(params.Key)})
	if err != nil {
		return err
	}
	if aws.ToString(created.UploadId) == "" {
		return errors.New("迅雷对象存储未返回分片会话 ID")
	}
	ui.Upload.UploadID = *created.UploadId
	partSize := max(int64(16<<20), (size+9998)/9999)
	parts := make([]types.CompletedPart, 0)
	for offset, number := int64(0), int32(1); offset < size; number++ {
		if ui.IsUploadStopRequested() {
			return context.Canceled
		}
		length := min(partSize, size-offset)
		base := offset
		body := driveutil.NewProgressOnlyReader(io.NewSectionReader(f, offset, length), length, func(done int64) { ui.ReportUploadProgress(base+done, size) })
		part, err := sc.UploadPart(ctx, &s3.UploadPartInput{Bucket: aws.String(params.Bucket), Key: aws.String(params.Key), UploadId: created.UploadId, PartNumber: aws.Int32(number), Body: body, ContentLength: aws.Int64(length)})
		if err != nil {
			return err
		}
		if aws.ToString(part.ETag) == "" {
			return fmt.Errorf("迅雷分片 %d 缺少校验回执", number)
		}
		parts = append(parts, types.CompletedPart{PartNumber: aws.Int32(number), ETag: part.ETag})
		offset += length
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	if after.Size() != size {
		return errors.New("本地文件在上传期间变化，未提交远端分片")
	}
	_, err = sc.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{Bucket: aws.String(params.Bucket), Key: aws.String(params.Key), UploadId: created.UploadId, MultipartUpload: &types.CompletedMultipartUpload{Parts: parts}})
	return err
}
