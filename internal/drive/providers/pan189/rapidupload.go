package pan189

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"mnemo-go/internal/drive"
)

// rapidResult is the API 秒传 response.
type rapidResult struct {
	Reuse   bool
	FileID  string
	Message string
}

// RapidUploadByHash creates a file by MD5 fingerprint (AList RapidUpload:
// createUploadFile → fileCommitUrl when fileDataExists=1).
func (d *Driver) RapidUploadByHash(ctx context.Context, c drive.Context, req drive.RapidUploadRequest) (*drive.RapidUploadResult, error) {
	if req.Method != "md5" {
		return &drive.RapidUploadResult{Reuse: false, Message: "天翼云盘仅支持 MD5 秒传"}, nil
	}
	fileMD5 := strings.ToUpper(strings.TrimSpace(req.Hash))
	if !regexp.MustCompile(`^[A-F0-9]{32}$`).MatchString(fileMD5) {
		return &drive.RapidUploadResult{Reuse: false, Message: "无效的 MD5 指纹"}, nil
	}
	if req.Size < 0 {
		return &drive.RapidUploadResult{Reuse: false, Message: "文件大小不能为负数"}, nil
	}
	sess, err := sessionOf(c.Token)
	if err != nil {
		return nil, err
	}
	isFamily, familyID := cloudInfo(sess)
	parentFolderID := toFolderID(req.ParentID)

	// createUploadFile: 个人云 form / 家庭云 params。
	var (
		createURL string
		opts      reqOptions
	)
	if isFamily {
		createURL = apiURL + "/family/file/createFamilyFile.action"
		opts = reqOptions{
			method: "POST",
			params: map[string]string{
				"familyId": familyID, "parentId": parentFolderID,
				"fileMd5": fileMD5, "fileName": req.FileName,
				"fileSize": itoa(req.Size), "resumePolicy": "1",
			},
		}
	} else {
		createURL = apiURL + "/createUploadFile.action"
		opts = reqOptions{
			method: "POST",
			form: map[string]string{
				"parentFolderId": parentFolderID, "fileName": req.FileName,
				"size": itoa(req.Size), "md5": fileMD5, "opertype": "3",
				"flag": "1", "resumePolicy": "1", "isLog": "0",
			},
		}
	}
	raw, err := d.request(ctx, c, createURL, opts)
	if err != nil {
		return nil, err
	}
	var created struct {
		Data struct {
			UploadFileID   json.RawMessage `json:"uploadFileId"`
			FileCommitURL  string          `json:"fileCommitUrl"`
			FileDataExists int             `json:"fileDataExists"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return nil, fmt.Errorf("pan189: 秒传初始化响应无效: %w", err)
	}
	if created.Data.FileDataExists != 1 {
		return &drive.RapidUploadResult{Reuse: false, Message: "未命中秒传"}, nil
	}
	commitURL := created.Data.FileCommitURL
	uploadFileID := rawIDString(created.Data.UploadFileID)
	if commitURL == "" || uploadFileID == "" {
		return &drive.RapidUploadResult{Reuse: false, Message: "秒传缺少提交地址"}, nil
	}

	duplicate := req.Duplicate
	if duplicate != 2 {
		duplicate = 1
	}
	opertype := "1"
	if duplicate == 2 {
		opertype = "3"
	}
	commitOpts := reqOptions{method: "POST", family: boolPtr(isFamily), isXML: true}
	if isFamily {
		commitOpts.headers = map[string]string{
			"ResumePolicy": "1", "UploadFileId": uploadFileID, "FamilyId": familyID,
		}
	} else {
		commitOpts.form = map[string]string{
			"opertype": opertype, "resumePolicy": "1", "uploadFileId": uploadFileID, "isLog": "0",
		}
	}
	raw, err = d.request(ctx, c, commitURL, commitOpts)
	if err != nil {
		return nil, err
	}
	var committed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &committed); err != nil {
		return nil, fmt.Errorf("pan189: 秒传提交响应无效: %w", err)
	}
	if committed.ID == "" {
		return &drive.RapidUploadResult{Reuse: true, Message: "秒传命中"}, nil
	}
	return &drive.RapidUploadResult{Reuse: true, FileID: committed.ID, Message: "秒传命中"}, nil
}

// ResolveTransferHash returns the md5 content hash of a source file
// (mirrors legacy resolveTransferHash; getFile carries no hash so this is
// informational only).
func (d *Driver) ResolveTransferHash(ctx context.Context, c drive.Context, fileID, method string, _ bool) (string, error) {
	if method != "md5" {
		return "", nil
	}
	f, err := d.GetFile(ctx, c, fileID)
	if err != nil {
		return "", err
	}
	hashValue := strings.ToLower(strings.TrimSpace(f.ContentHash))
	if strings.EqualFold(strings.TrimSpace(f.ContentHashName), "md5") && regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(hashValue) {
		return hashValue, nil
	}
	return "", nil
}
