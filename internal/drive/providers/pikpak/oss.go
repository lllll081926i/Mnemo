package pikpak

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

// ossPut uploads a file to the Alibaba OSS endpoint provided by PikPak's
// create upload response. It uses the STS credentials (access_key, secret_key,
// security_token) to sign the PUT request.
func ossPut(ctx context.Context, c drive.Context, localPath string, params *model.ConnConfig, ui *model.UploadingUI) error {
	if params == nil || params.Endpoint == "" {
		return errors.New("pikpak: oss params missing")
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}

	// Reconstruct from ConnConfig mapping: Username=access_key_id, Password=access_key_secret,
	// Bucket=bucket, Endpoint=endpoint, BasePath=key, Region=security_token.
	accessKey := params.Username
	secretKey := params.Password
	bucket := params.Bucket
	objectKey := params.BasePath
	securityToken := params.Region

	url := fmt.Sprintf("https://%s.%s/%s", bucket, params.Endpoint, strings.TrimPrefix(objectKey, "/"))
	contentType := "application/octet-stream"
	date := time.Now().UTC().Format(http.TimeFormat)

	// Build OSS signature
	canonicalResource := "/" + bucket + "/" + strings.TrimPrefix(objectKey, "/")
	stringToSign := "PUT\n\n" + contentType + "\n" + date + "\nx-oss-security-token:" + securityToken + "\n" + canonicalResource

	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	authorization := "OSS " + accessKey + ":" + signature

	// Create request streaming the file body (no full buffering).
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, f)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Date", date)
	req.Header.Set("Authorization", authorization)
	req.Header.Set("x-oss-security-token", securityToken)
	req.ContentLength = info.Size()

	httpClient := &http.Client{Timeout: 600 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("pikpak: oss put %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if ui != nil {
		ui.Upload.DownSize = info.Size()
		ui.Upload.DownProcess = 100
		ui.Upload.IsCompleted = true
	}
	return nil
}

var _ = json.Marshal
var _ = netx.DefaultUA