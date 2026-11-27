package app

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
)

// DownloadVerification describes the integrity check result of a completed download.
type DownloadVerification struct {
	TaskID    string `json:"taskId"`
	Status    string `json:"status"` // hash | size | mismatch | missing | unverified
	Algorithm string `json:"algorithm,omitempty"`
	Detail    string `json:"detail"`
}

func newDownloadHasher(algorithm string) (hash.Hash, string, bool) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "md5":
		return md5.New(), "MD5", true
	case "sha1":
		return sha1.New(), "SHA1", true
	case "sha256":
		return sha256.New(), "SHA256", true
	default:
		return nil, "", false
	}
}

func hashLocalFile(ctx context.Context, path, algorithm string) (string, string, error) {
	h, label, ok := newDownloadHasher(algorithm)
	if !ok {
		return "", "", fmt.Errorf("不支持的哈希算法: %s", algorithm)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	// 1 MB 固定缓冲流式哈希，降低大文件 syscall 次数并保持低内存占用
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), label, nil
}

// VerifyDownload verifies a completed local download against its recorded size
// and trusted provider hash (without re-downloading the remote file).
func (a *App) VerifyDownload(taskID string) (*DownloadVerification, error) {
	var target *model.DownloadTask
	for _, task := range a.ListDownloads() {
		if task.ID == taskID {
			copyTask := task
			target = &copyTask
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("下载记录不存在")
	}
	if target.Status != "completed" || target.IsDir {
		return nil, fmt.Errorf("仅支持对已完成的单文件下载进行校验")
	}
	if strings.TrimSpace(target.LocalPath) == "" {
		return &DownloadVerification{TaskID: taskID, Status: "missing", Detail: "缺少本地文件路径"}, nil
	}
	info, err := os.Stat(target.LocalPath)
	if err != nil || info.IsDir() {
		return &DownloadVerification{TaskID: taskID, Status: "missing", Detail: "本地文件已移动或删除"}, nil
	}
	if target.Size > 0 && info.Size() != target.Size {
		return &DownloadVerification{
			TaskID: taskID,
			Status: "mismatch",
			Detail: fmt.Sprintf("本地文件大小（%d 字节）与记录大小（%d 字节）不一致", info.Size(), target.Size),
		}, nil
	}

	ctx, cancel := context.WithTimeout(a.appContext(), 3*time.Minute)
	defer cancel()

	algo := strings.TrimSpace(target.ContentHashName)
	expected := strings.TrimSpace(target.ContentHash)
	if (algo == "" || expected == "") && target.UserID != "" && target.FileID != "" {
		caps := drive.RegistryCaps(drive.ProviderOf(target.UserID, target.DriveID, ""))
		for _, method := range caps.ProvideHashes {
			if _, _, ok := newDownloadHasher(method); !ok {
				continue
			}
			if resolved, rerr := drive.ResolveTransferHashContext(ctx, target.UserID, target.DriveID, target.FileID, method, false); rerr == nil && strings.TrimSpace(resolved) != "" {
				algo = method
				expected = strings.TrimSpace(resolved)
				break
			}
		}
	}

	if algo == "" || expected == "" {
		return &DownloadVerification{
			TaskID: taskID,
			Status: "size",
			Detail: "文件大小一致（该网盘未提供可信内容哈希）",
		}, nil
	}
	if _, _, ok := newDownloadHasher(algo); !ok {
		return &DownloadVerification{
			TaskID:    taskID,
			Status:    "size",
			Algorithm: algo,
			Detail:    fmt.Sprintf("文件大小一致（暂不支持比对 %s 哈希）", algo),
		}, nil
	}

	actual, label, err := hashLocalFile(ctx, target.LocalPath, algo)
	if err != nil {
		return nil, fmt.Errorf("计算本地文件哈希失败: %w", err)
	}
	if strings.EqualFold(actual, expected) {
		return &DownloadVerification{
			TaskID:    taskID,
			Status:    "hash",
			Algorithm: label,
			Detail:    label + " 哈希校验通过，内容完全一致",
		}, nil
	}
	return &DownloadVerification{
		TaskID:    taskID,
		Status:    "mismatch",
		Algorithm: label,
		Detail:    label + " 哈希不一致，本地文件可能已损坏或被修改",
	}, nil
}
