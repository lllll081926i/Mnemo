// Package s3 implements the S3-compatible object storage provider
// (mounted storage, direct upload).
package s3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	"mnemo-go/internal/model"
)

const providerID = model.ProviderS3

func init() {
	drive.Register(drive.Registration{
		ID:   providerID,
		Meta: drive.GetMeta(providerID),
		Caps: drive.NewCapabilities(providerID, map[string]bool{
			"mountedStorage":  true,
			"permanentDelete": true,
			"recycleBin":      false,
			"trashView":       false,
		}, func(c *drive.Capabilities) {
			c.SetUploadMode(drive.UploadModeDirect)
		}),
		Factory: func() drive.Driver { return &Driver{} },
	})
}

// Driver implements drive.Driver for S3. File ids are bucket-relative keys.
type Driver struct {
	drive.BaseDriver
}

func (d *Driver) ID() string                       { return providerID }
func (d *Driver) Meta() drive.Meta                 { return drive.GetMeta(providerID) }
func (d *Driver) Capabilities() drive.Capabilities { return drive.RegistryCaps(providerID) }
func (d *Driver) RootID() string                   { return "/" }

type conn struct {
	client *s3.Client
	bucket string
	prefix string // base path prefix, "" or "dir/"
}

// TransportOverride, when set, is used as the S3 SDK transport. Test-only hook
// to route real S3 traffic to a local mock (never set in production).
var TransportOverride http.RoundTripper

func httpClientForS3() *http.Client {
	if TransportOverride != nil {
		return &http.Client{Transport: TransportOverride}
	}
	return nil
}

func connOf(c drive.Context) (*conn, error) {
	cfg := c.Token
	if cfg == nil || cfg.Conn == nil || cfg.Conn.Endpoint == "" {
		return nil, errors.New("s3: 连接不存在，请重新连接")
	}
	cc := cfg.Conn
	endpoint := cc.Endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}
	bucket := cc.Bucket
	if bucket == "" {
		return nil, errors.New("s3: 缺少 bucket")
	}
	// Path-style addressing is the default (MinIO / Aliyun OSS / most S3-compatible
	// endpoints need it). Users who want virtual-host style (e.g. AWS S3 native)
	// set ForcePathStyle=false explicitly via a non-nil pointer.
	usePathStyle := true
	if cc.ForcePathStyle != nil {
		usePathStyle = *cc.ForcePathStyle
	}
	client := s3.New(s3.Options{
		Region:       firstNonEmpty(cc.Region, "us-east-1"),
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(firstNonEmpty(cc.Username, "minioadmin"), cc.Password, cc.SessionToken),
		UsePathStyle: usePathStyle,
		HTTPClient:   httpClientForS3(),
	})
	return &conn{client: client, bucket: bucket, prefix: normalizePrefix(cc.BasePath)}, nil
}

func (c *conn) keyOf(id string) string {
	key := strings.TrimPrefix(pathOf(id), "/")
	return c.prefix + key
}

func (c *conn) idOf(key string) string {
	return "/" + strings.TrimPrefix(key, c.prefix)
}

func pathOf(id string) string {
	if id == "" || id == "/" || id == "root" {
		return "/"
	}
	if !strings.HasPrefix(id, "/") {
		return "/" + id
	}
	return id
}

func normalizePrefix(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return p + "/"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (d *Driver) List(ctx context.Context, c drive.Context, dirID string, _ *drive.ListOptions) ([]model.File, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	prefix := cc.keyOf(dirID)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	var out []model.File
	var token *string
	for {
		resp, err := cc.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(cc.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("s3: list: %w", err)
		}
		for _, p := range resp.CommonPrefixes {
			key := strings.TrimSuffix(*p.Prefix, "/")
			name := baseOf(key)
			out = append(out, driveutil.NewFile(c.DriveID, cc.idOf(key), dirID, name, true, 0, 0))
		}
		for _, o := range resp.Contents {
			key := *o.Key
			name := baseOf(key)
			size := int64(0)
			if o.Size != nil {
				size = *o.Size
			}
			t := time.Time{}
			if o.LastModified != nil {
				t = *o.LastModified
			}
			out = append(out, driveutil.NewFile(c.DriveID, cc.idOf(key), dirID, name, false, size, t.Unix()))
		}
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
		token = resp.NextContinuationToken
	}
	return out, nil
}

func (d *Driver) GetInfo(ctx context.Context, c drive.Context, fileID string) (any, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	if p == "/" {
		return driveutil.NewFile(c.DriveID, "/", "", "S3", true, 0, 0), nil
	}
	key := cc.keyOf(p)
	head, err := cc.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(cc.bucket), Key: aws.String(key)})
	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			return nil, drive.ErrNotFound
		}
		// maybe it's a folder prefix
		ls, lerr := cc.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(cc.bucket), Prefix: aws.String(key + "/"), MaxKeys: aws.Int32(1)})
		if lerr == nil && len(ls.Contents) > 0 {
			return driveutil.NewFile(c.DriveID, cc.idOf(key), parentOf(p), baseOf(key), true, 0, 0), nil
		}
		return nil, drive.ErrNotFound
	}
	size := int64(0)
	if head.ContentLength != nil {
		size = *head.ContentLength
	}
	return driveutil.NewFile(c.DriveID, cc.idOf(key), parentOf(p), baseOf(key), false, size, head.LastModified.Unix()), nil
}

func (d *Driver) GetFile(ctx context.Context, c drive.Context, fileID string) (*model.File, error) {
	v, err := d.GetInfo(ctx, c, fileID)
	if err != nil {
		return nil, err
	}
	f, ok := v.(model.File)
	if !ok {
		return nil, drive.ErrNotFound
	}
	return &f, nil
}

func (d *Driver) GetDownloadURL(ctx context.Context, c drive.Context, fileID string, expireSec int) (*model.DownloadURL, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	info, err := d.GetInfo(ctx, c, p)
	if err != nil {
		return nil, err
	}
	f := info.(model.File)
	if f.IsDir {
		return nil, errors.New("文件夹不能直接下载")
	}
	if expireSec < 60 {
		expireSec = 14400
	}
	presigner := s3.NewPresignClient(cc.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(cc.bucket),
		Key:    aws.String(cc.keyOf(p)),
	}, func(po *s3.PresignOptions) {
		po.Expires = time.Duration(expireSec) * time.Second
	})
	if err != nil {
		return nil, err
	}
	return &model.DownloadURL{
		DriveID:      c.DriveID,
		FileID:       fileID,
		ExpireTime:   time.Now().Add(time.Duration(expireSec) * time.Second).Unix(),
		URL:          req.URL,
		Size:         f.Size,
		DownloadMode: "redirect",
	}, nil
}

func (d *Driver) GetVideoPreview(ctx context.Context, c drive.Context, fileID string) (*model.VideoPreview, error) {
	u, err := d.GetDownloadURL(ctx, c, fileID, 3600)
	if err != nil {
		return nil, err
	}
	return &model.VideoPreview{
		DriveID: c.DriveID,
		FileID:  fileID,
		Size:    u.Size,
		Qualities: []model.VideoQuality{{
			Quality: "origin", Label: "原画", Value: "origin",
			URL: u.URL, Headers: u.Headers, ForceProxy: true,
		}},
	}, nil
}

func (d *Driver) Mkdir(ctx context.Context, c drive.Context, parentID, name string) (*drive.MkdirResult, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	key := cc.keyOf(driveutil.JoinPath(pathOf(parentID), name)) + "/"
	_, err = cc.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(cc.bucket), Key: aws.String(key)})
	if err != nil {
		return &drive.MkdirResult{Error: err.Error()}, nil
	}
	return &drive.MkdirResult{FileID: cc.idOf(strings.TrimSuffix(key, "/"))}, nil
}

func (d *Driver) Rename(ctx context.Context, c drive.Context, fileID, name string) (*drive.RenameResult, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	p := pathOf(fileID)
	info, err := d.GetInfo(ctx, c, p)
	if err != nil {
		return nil, err
	}
	f := info.(model.File)
	to := driveutil.JoinPath(parentOf(p), name)
	if err := d.copyObj(ctx, cc, cc.keyOf(p), cc.keyOf(to)); err != nil {
		return nil, err
	}
	_ = cc.deleteObj(ctx, cc.keyOf(p))
	return &drive.RenameResult{FileID: cc.idOf(strings.TrimPrefix(cc.keyOf(to), cc.prefix)), ParentFileID: parentOf(to), Name: name, IsDir: f.IsDir}, nil
}

func (d *Driver) Trash(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return d.Delete(ctx, c, idsToRefs(fileIDs))
}

func (d *Driver) Delete(ctx context.Context, c drive.Context, refs []drive.FileRef) ([]string, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	var ok []string
	for _, ref := range refs {
		if err := cc.deleteRecursive(ctx, cc.keyOf(ref.ID)); err == nil {
			ok = append(ok, ref.ID)
		}
	}
	return ok, nil
}

func (c *conn) deleteObj(ctx context.Context, key string) error {
	if key == "" || key == "/" {
		return nil
	}
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	return err
}

// listAllUnder returns every object key under the given prefix (no delimiter),
// paginating through all results. Used for recursive directory operations.
func (c *conn) listAllUnder(ctx context.Context, prefix string) ([]string, error) {
	var out []string
	var token *string
	for {
		resp, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, obj := range resp.Contents {
			out = append(out, *obj.Key)
		}
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
		token = resp.NextContinuationToken
	}
	return out, nil
}

// deleteRecursive removes a key and, when it is a directory prefix, every
// object under it. Deletion is batched (DeleteObjects, 1000 per batch) for
// efficiency.
func (c *conn) deleteRecursive(ctx context.Context, key string) error {
	if key == "" || key == "/" {
		return nil
	}
	// gather the object itself plus everything under key + "/"
	prefix := strings.TrimPrefix(key, "/")
	keys := []string{prefix}
	under, err := c.listAllUnder(ctx, prefix+"/")
	if err != nil {
		// fall back to single delete
		return c.deleteObj(ctx, key)
	}
	keys = append(keys, under...)
	// batch delete (max 1000 per DeleteObjects call)
	for i := 0; i < len(keys); i += 1000 {
		end := i + 1000
		if end > len(keys) {
			end = len(keys)
		}
		var objs []types.ObjectIdentifier
		for _, k := range keys[i:end] {
			objs = append(objs, types.ObjectIdentifier{Key: aws.String(k)})
		}
		if _, err := c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		}); err != nil {
			return err
		}
	}
	return nil
}

// copyRecursive copies a single object (CopyObject) or, when the source is a
// directory prefix, every object under it to the destination prefix.
func (c *conn) copyRecursive(ctx context.Context, fromKey, toKey string) error {
	// try a single CopyObject first; if the key is a prefix it lists and copies
	// each child, rewriting the prefix.
	under, err := c.listAllUnder(ctx, fromKey+"/")
	if err != nil {
		return c.copyOne(ctx, fromKey, toKey)
	}
	if len(under) == 0 {
		// not a directory: copy the single object
		return c.copyOne(ctx, fromKey, toKey)
	}
	// copy the marker object itself (if any) then each child
	for _, k := range under {
		dst := toKey + "/" + strings.TrimPrefix(k, fromKey+"/")
		if err := c.copyOne(ctx, k, dst); err != nil {
			return err
		}
	}
	return nil
}

func (c *conn) copyOne(ctx context.Context, from, to string) error {
	if from == "" || from == "/" {
		return nil
	}
	_, err := c.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(c.bucket),
		CopySource: aws.String(c.bucket + "/" + from),
		Key:        aws.String(to),
	})
	return err
}

// objectExists returns whether an object exists at key. A 404 (or NoSuchKey)
// reports exists=false,nil. Other errors are returned as-is.
func (c *conn) objectExists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	var noKey *types.NoSuchKey
	if errors.As(err, &noKey) {
		return false, nil
	}
	var re *awshttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == 404 {
		return false, nil
	}
	return false, err
}

func (d *Driver) Restore(ctx context.Context, c drive.Context, fileIDs []string) ([]string, error) {
	return nil, drive.NotSupported("restore")
}

func (d *Driver) Move(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	targetParent := pathOf(toParentID)
	var ok []string
	for _, ref := range refs {
		base := baseOf(ref.ID)
		to := driveutil.JoinPath(targetParent, base)
		if err := d.copyObj(ctx, cc, cc.keyOf(ref.ID), cc.keyOf(to)); err != nil {
			continue
		}
		_ = cc.deleteObj(ctx, cc.keyOf(ref.ID))
		ok = append(ok, ref.ID)
	}
	return ok, nil
}

func (d *Driver) Copy(ctx context.Context, c drive.Context, refs []drive.FileRef, toParentID, _ string) ([]string, error) {
	cc, err := connOf(c)
	if err != nil {
		return nil, err
	}
	targetParent := pathOf(toParentID)
	var ok []string
	for _, ref := range refs {
		base := baseOf(ref.ID)
		to := driveutil.JoinPath(targetParent, base)
		if err := d.copyObj(ctx, cc, cc.keyOf(ref.ID), cc.keyOf(to)); err == nil {
			ok = append(ok, ref.ID)
		}
	}
	return ok, nil
}

func (d *Driver) copyObj(ctx context.Context, cc *conn, from, to string) error {
	return cc.copyRecursive(ctx, from, to)
}

// UploadOneFile performs a direct upload (single PUT, no multipart for simplicity;
// large files could use multipart later). It honors ui.Info.ConflictPolicy when
// the target object already exists: refuse returns an error, rename uploads to
// a generated non-conflicting key, overwrite (the default) replaces it.
func (d *Driver) UploadOneFile(ctx context.Context, c drive.Context, ui *model.UploadingUI) error {
	cc, err := connOf(c)
	if err != nil {
		return err
	}
	key := cc.keyOf(driveutil.JoinPath(pathOf(ui.Info.ParentFileID), ui.Info.Name))

	switch driveutil.ResolveConflictPolicy(ui.Info.ConflictPolicy) {
	case driveutil.ConflictRefuse:
		if exists, e := cc.objectExists(ctx, key); e == nil && exists {
			return errors.New("s3: 目标文件已存在")
		}
	case driveutil.ConflictRename:
		if exists, e := cc.objectExists(ctx, key); e == nil && exists {
			newName := driveutil.GenerateConflictName(ui.Info.Name)
			ui.Info.Name = newName
			key = cc.keyOf(driveutil.JoinPath(pathOf(ui.Info.ParentFileID), newName))
		}
	}

	f, err := os.Open(ui.Info.LocalFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	pr := driveutil.NewProgressReader(f, ui.Info.Size, func(read int64) {
		ui.Upload.DownSize = read
		if ui.Info.Size > 0 {
			ui.Upload.DownProcess = int(read * 100 / ui.Info.Size)
		}
	})
	_, err = cc.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(cc.bucket),
		Key:           aws.String(key),
		Body:          pr,
		ContentLength: aws.Int64(ui.Info.Size),
	})
	if err != nil {
		return fmt.Errorf("s3: upload: %w", err)
	}
	return nil
}

func baseOf(key string) string {
	key = strings.TrimSuffix(key, "/")
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return key
}

func parentOf(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	return "/"
}

func idsToRefs(ids []string) []drive.FileRef {
	refs := make([]drive.FileRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, drive.FileRef{ID: id})
	}
	return refs
}

var _ = awshttp.NewBuildableClient // keep transport import if needed later
