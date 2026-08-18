// Package app is the Wails binding layer. All methods on App are callable from
// the frontend; state changes are pushed via Events.Emit.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/captcha"
	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	_ "mnemo-go/internal/drive/providers" // register all plugins
	"mnemo-go/internal/drive/providers/pan189"
	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/preview"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer"
	"mnemo-go/internal/transfer/dlengine"
	"mnemo-go/internal/transfer/migrate"
	"os"
	"strings"
	"time"
)

// App is the root Wails-bound struct.
type App struct {
	stateMu      sync.RWMutex
	persistMu    sync.Mutex
	persistErr   error
	ctx          context.Context
	store        *store.Store
	preview      *preview.Server
	dl           *transfer.Manager
	uploads      *transfer.UploadQueue
	secrets      config.Secrets
	dataDir      string

	migrate      *migrate.Engine
	schedStop    chan struct{} // sync scheduler stop, closed on Shutdown
	shutdownOnce sync.Once
}

// NewApp constructs the app (no side effects; wiring happens in startup).
func NewApp() *App { return &App{} }

// storeOrError keeps Wails bindings safe while startup is still in progress
// (or when startup failed before persistence could be opened).
func (a *App) storeOrError() (*store.Store, error) {
	if a == nil {
		return nil, errors.New("应用尚未初始化")
	}
	return a.ensurePersistence()
}

// ensurePersistence opens the Store at most once. Wails browser development
// mode can invoke bound methods while OnStartup is still being scheduled, so
// Store-backed read methods must not depend on the startup callback winning a
// race against the first frontend render.
func (a *App) ensurePersistence() (*store.Store, error) {
	if a == nil {
		return nil, errors.New("应用尚未初始化")
	}
	a.stateMu.RLock()
	st := a.store
	a.stateMu.RUnlock()
	if st != nil {
		return st, nil
	}

	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	a.stateMu.RLock()
	st = a.store
	err := a.persistErr
	a.stateMu.RUnlock()
	if st != nil {
		return st, nil
	}
	if err != nil {
		return nil, err
	}

	configDir, err := config.UserConfigDir("Mnemo")
	if err != nil {
		configDir = "."
	}
	dataDir, err := config.DataDir("Mnemo", configDir)
	if err != nil {
		a.stateMu.Lock()
		a.persistErr = err
		a.stateMu.Unlock()
		return nil, err
	}
	secrets := config.LoadSecrets(dataDir)
	st, err = store.Open(dataDir)
	if err != nil {
		a.stateMu.Lock()
		a.persistErr = err
		a.stateMu.Unlock()
		return nil, err
	}
	st.SetAccountsDir(configDir)
	a.stateMu.Lock()
	a.dataDir = dataDir
	a.secrets = secrets
	a.store = st
	a.stateMu.Unlock()
	return st, nil
}

// appContext returns a non-nil context for provider/transfer work. Wails UI
// calls must use wailsContext instead, because a background context cannot be
// passed to runtime dialogs or event APIs.
func (a *App) appContext() context.Context {
	if a == nil {
		return context.Background()
	}
	a.stateMu.RLock()
	ctx := a.ctx
	a.stateMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (a *App) wailsContext() (context.Context, bool) {
	if a == nil {
		return nil, false
	}
	a.stateMu.RLock()
	ctx := a.ctx
	a.stateMu.RUnlock()
	return ctx, ctx != nil
}

func (a *App) dataDirectory() string {
	if a == nil {
		return ""
	}
	a.stateMu.RLock()
	dir := a.dataDir
	a.stateMu.RUnlock()
	return dir
}

func (a *App) secretsSnapshot() config.Secrets {
	if a == nil {
		return config.Secrets{}
	}
	a.stateMu.RLock()
	secrets := a.secrets
	a.stateMu.RUnlock()
	return secrets
}

func (a *App) previewServer() *preview.Server {
	if a == nil {
		return nil
	}
	a.stateMu.RLock()
	p := a.preview
	a.stateMu.RUnlock()
	return p
}

func (a *App) downloadManager() *transfer.Manager {
	if a == nil {
		return nil
	}
	a.stateMu.RLock()
	dl := a.dl
	a.stateMu.RUnlock()
	return dl
}

func (a *App) uploadQueue() *transfer.UploadQueue {
	if a == nil {
		return nil
	}
	a.stateMu.RLock()
	uploads := a.uploads
	a.stateMu.RUnlock()
	return uploads
}

func (a *App) migrationEngine() *migrate.Engine {
	if a == nil {
		return nil
	}
	a.stateMu.RLock()
	eng := a.migrate
	a.stateMu.RUnlock()
	return eng
}

// startup initializes persistence, providers and transfer engines.
func (a *App) startup(ctx context.Context) {
	if a == nil {
		return
	}
	a.stateMu.Lock()
	a.ctx = ctx
	a.stateMu.Unlock()
	st, err := a.ensurePersistence()
	if err != nil {
		panic(fmt.Errorf("store: %w", err))
	}
	dataDir := a.dataDirectory()
	secrets := a.secretsSnapshot()

	// token resolver so drive ops can load sessions
	drive.SetTokenResolver(func(userID, driveID string) (*model.TokenInfo, error) {
		acc, err := st.GetAccount(userID)
		if err != nil {
			return nil, err
		}
		return drive.CloneToken(acc.Token), nil
	})
	drive.SetTokenUpdater(func(userID, driveID string, token *model.TokenInfo) error {
		if token == nil {
			return nil
		}
		acc, err := st.GetAccount(userID)
		if err != nil {
			return err
		}
		if acc.DriveID != "" && driveID != "" && acc.DriveID != driveID {
			return fmt.Errorf("账号驱动不匹配")
		}
		return st.UpdateAccountToken(userID, drive.CloneToken(token))
	})

	// secret resolver so providers can read OAuth client ids during refresh
	drive.SetSecretResolver(func(key string) string {
		switch key {
		case "onedrive_client_id":
			return secrets.OnedriveClientID
		case "dropbox_app_key":
			return secrets.DropboxAppKey
		}
		return ""
	})

	// upload session persistence so providers can resume interrupted uploads
	store.InitUploadSessions(dataDir)
	drive.SetUploadSessionStore(uploadSessionAdapter{})

	// internal media/preview server. Roots restrict /local/ serving to the
	// download dir, engine dir and data dir (logs, mpv-config, etc.).
	dlDir := transfer.DownloadDir(st)
	engineDir := filepath.Join(dataDir, "engine")
	mediaProxy, err := preview.NewServer(dlDir, engineDir, dataDir)
	if err != nil {
		// preview is not optional for media playback; surface the error instead
		// of crashing silently on a nil Port access later.
		panic(fmt.Errorf("preview server: %w", err))
	}
	a.stateMu.Lock()
	a.preview = mediaProxy
	a.stateMu.Unlock()

	// download manager + upload queue
	downloads, err := transfer.NewManager(st, dlDir, func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})
	if err != nil {
		panic(fmt.Errorf("download manager: %w", err))
	}
	a.stateMu.Lock()
	a.dl = downloads
	a.stateMu.Unlock()
	uploads := transfer.NewUploadQueue(st, func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})
	a.stateMu.Lock()
	a.uploads = uploads
	a.stateMu.Unlock()

	// Keep one migration engine for the app lifetime so cancellation and
	// persisted jobs address the same in-memory registry.
	migrations := migrate.NewEngine(st, func(j *migrate.Job) {
		if j != nil {
			a.emit("migrate:progress", *j)
		}
	})
	a.stateMu.Lock()
	a.migrate = migrations
	a.stateMu.Unlock()
	// recover interrupted migrate jobs (mark as canceled, aligned with legacy)
	migrations.RecoverUnfinished()

	// provider identity dir (pikpak device ids)
	identityDir := filepath.Join(dataDir, "identity")
	_ = os.MkdirAll(identityDir, 0o755)
	pikpak.SetIdentityDir(identityDir)

	// apply persisted proxy so providers and the download engine honor it from
	// the very first request.
	if s, err := st.GetSettings(); err == nil && s.Proxy != "" {
		netx.SetGlobalProxy(s.Proxy)
	}
	// wire the upload rate getter so ProgressReader can throttle direct uploads
	driveutil.SetUploadRateGetter(netx.GlobalUploadRate)
	if s, err := st.GetSettings(); err == nil {
		netx.SetGlobalUploadRate(s.MaxUploadSpeed)
	}

	a.emit("app:ready", map[string]any{"port": mediaProxy.Port})
}

// emit pushes an event to the frontend.
func (a *App) emit(name string, data any) {
	if ctx, ok := a.wailsContext(); ok {
		runtime.EventsEmit(ctx, name, data)
	}
}

// shutdown is wired to Wails OnShutdown to release long-lived resources:
// the download manager, preview server and captcha server. Errors are logged to
// stderr because Wails ignores the returned error and we must not panic on
// the shutdown path.
func (a *App) Shutdown(ctx context.Context) {
	if a == nil {
		return
	}
	a.shutdownOnce.Do(func() {
		captcha.Close()
		a.stateMu.Lock()
		migrations, downloads, uploads, mediaProxy, stop := a.migrate, a.dl, a.uploads, a.preview, a.schedStop
		a.schedStop = nil
		if stop != nil {
			close(stop)
		}
		a.stateMu.Unlock()
		if migrations != nil {
			migrations.CancelAll()
		}
	
		if downloads != nil {
			downloads.Shutdown()
		}
		if uploads != nil {
			uploads.Close()
		}
		if mediaProxy != nil {
			_ = mediaProxy.Close()
		}
	})
}

// OpenBrowser opens a URL in the system browser.
func (a *App) OpenBrowser(url string) {
	if ctx, ok := a.wailsContext(); ok {
		runtime.BrowserOpenURL(ctx, url)
	}
}

// OpenPikPakCaptcha opens the challenge in a temporary application-owned
// WebView and emits the final token when PikPak accepts the verification.
func (a *App) OpenPikPakCaptcha(url string) error {
	return captcha.Open(url, func(token string) {
		a.emit("pikpak:captcha:completed", map[string]string{
			"url":           url,
			"captcha_token": token,
		})
	})
}

// ClosePikPakCaptcha closes the temporary challenge window, if any.
func (a *App) ClosePikPakCaptcha() {
	captcha.Close()
}

// ProviderInfo is the JSON-safe projection of a provider registration exposed
// to the frontend (excludes func-typed Factory/Auth).
type ProviderInfo struct {
	ID           string             `json:"ID"`
	Meta         drive.Meta         `json:"Meta"`
	Capabilities drive.Capabilities `json:"Capabilities"`
	Login        drive.LoginConfig  `json:"Login"`
}

// ---- providers ----

// ListProviders returns the 13 in-scope drive providers (JSON-safe DTOs).
func (a *App) ListProviders() []ProviderInfo {
	regs := drive.All()
	out := make([]ProviderInfo, 0, len(regs))
	for _, r := range regs {
		out = append(out, ProviderInfo{ID: r.ID, Meta: r.Meta, Capabilities: r.Caps, Login: r.Login})
	}
	return out
}

// GetPan189Captcha returns the latest captcha image data URL for 189 cloud.
func (a *App) GetPan189Captcha() string {
	return pan189.CaptchaImage()
}

// ProviderLogin performs a login for a provider with form config.
func (a *App) ProviderLogin(provider string, config map[string]string) (*model.Account, error) {
	reg, ok := drive.Get(provider)
	if !ok {
		return nil, fmt.Errorf("未知网盘: %s", provider)
	}
	if reg.Auth == nil {
		return nil, fmt.Errorf("%s 不支持此登录方式", provider)
	}
	// inject secrets
	if config == nil {
		config = map[string]string{}
	}
	secrets := a.secretsSnapshot()
	if secrets.OnedriveClientID != "" {
		config["onedrive_client_id"] = secrets.OnedriveClientID
	}
	if secrets.DropboxAppKey != "" {
		config["dropbox_app_key"] = secrets.DropboxAppKey
	}
	tok, err := reg.Auth(context.Background(), drive.AuthRequest{
		Config: config,
		Open: func(url string) error {
			ctx, ok := a.wailsContext()
			if !ok {
				return errors.New("应用尚未启动，无法打开登录页面")
			}
			runtime.BrowserOpenURL(ctx, url)
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return nil, fmt.Errorf("%s 登录未返回会话", provider)
	}
	// The registration that performed the login is authoritative. Do not let a
	// stale provider marker from an imported token route later requests to a
	// different driver.
	tok.TokenFrom = provider
	// Every provider must return a stable account identity. Normalize the
	// namespace here so a provider-specific login cannot create an empty or
	// cross-provider account key.
	accountID := model.StripUserID(provider, tok.UserID)
	if accountID == "" {
		accountID = strings.TrimSpace(tok.ProviderAccountID)
	}
	if accountID == "" {
		return nil, fmt.Errorf("%s 登录成功但未返回账号标识", provider)
	}
	tok.ProviderAccountID = accountID
	tok.UserID = model.BuildUserID(provider, accountID)
	if tok.DefaultDriveID == "" {
		tok.DefaultDriveID = model.BuildDriveID(provider, accountID)
	}
	acc := &model.Account{
		UserID:  tok.UserID,
		DriveID: normalizedDriveID(provider, accountID, tok.DefaultDriveID),
		Token:   tok,
	}
	if acc.DriveID == "" {
		acc.DriveID = model.BuildDriveID(provider, tok.ProviderAccountID)
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveAccount(acc); err != nil {
		return nil, err
	}
	a.emit("account:changed", acc)
	return acc, nil
}

// SaveMountedAccount persists a mounted storage account (webdav/s3) from the
// connection form.
func (a *App) SaveMountedAccount(provider string, conn model.ConnConfig) (*model.Account, error) {
	if provider != model.ProviderWebdav && provider != model.ProviderS3 {
		return nil, fmt.Errorf("仅支持挂载存储: %s", provider)
	}
	if err := drive.ValidateConnection(provider, &conn); err != nil {
		return nil, fmt.Errorf("连接校验失败: %w", err)
	}
	accountID := mountedAccountID(provider, conn)
	uid := model.BuildUserID(provider, accountID)
	tok := &model.TokenInfo{
		TokenFrom: provider,
		UserID:    uid,
		UserName:  conn.Name,
		Conn:      &conn,
	}
	acc := &model.Account{
		UserID:  uid,
		DriveID: model.BuildDriveID(provider, accountID),
		Token:   tok,
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveAccount(acc); err != nil {
		return nil, err
	}
	a.emit("account:changed", acc)
	return acc, nil
}

// mountedAccountID namespaces mounted connections by their non-secret
// connection identity. Endpoint alone is insufficient when one server has
// multiple users, and a display name alone can collide as well.
func mountedAccountID(provider string, conn model.ConnConfig) string {
	pathKey := conn.BasePath
	if provider == model.ProviderWebdav {
		if strings.TrimSpace(conn.RootPath) != "" {
			pathKey = conn.RootPath
		}
	}
	identity := strings.Join([]string{
		provider,
		strings.TrimSpace(conn.Name),
		strings.TrimSpace(conn.Endpoint),
		strings.TrimSpace(conn.Username),
		strings.TrimSpace(conn.Region),
		strings.TrimSpace(conn.Bucket),
		strings.TrimSpace(pathKey),
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "mount-" + hex.EncodeToString(sum[:12])
}

// normalizedDriveID keeps provider-returned drive identities (notably the
// real Microsoft Graph drive id) while retaining the provider namespace used
// by the application and cache keys.
func normalizedDriveID(provider, accountID, preferred string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		if strings.HasPrefix(preferred, provider+":") {
			return preferred
		}
		return model.BuildDriveID(provider, preferred)
	}
	return model.BuildDriveID(provider, accountID)
}

// ListAccounts returns all accounts.
func (a *App) ListAccounts() []*model.Account {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListAccounts()
	if err != nil {
		return nil
	}
	return list
}

// RefreshAccount silently refreshes an account's quota + profile from the
// provider, persists the updated token, and returns the refreshed account.
// Frontend polls this at a low frequency for the avatar/quota popover.
func (a *App) RefreshAccount(userID string) (*model.Account, error) {
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	acc, err := st.GetAccount(userID)
	if err != nil || acc == nil {
		return nil, fmt.Errorf("账号不存在")
	}
	tok, err := drive.RefreshAccount(userID, acc.DriveID)
	if err != nil {
		return acc, err
	}
	if tok != nil {
		acc.Token = tok
		provider := tok.TokenFrom
		if provider == "" {
			provider = model.ResolveProviderFromUserID(acc.UserID)
		}
		accountID := model.StripUserID(provider, tok.UserID)
		if accountID == "" {
			accountID = tok.ProviderAccountID
		}
		acc.DriveID = normalizedDriveID(provider, accountID, tok.DefaultDriveID)
	}
	if err := st.SaveAccount(acc); err != nil {
		return acc, fmt.Errorf("保存账号失败: %w", err)
	}
	a.emit("account:changed", acc)
	return acc, nil
}

// RemoveAccount deletes an account.
func (a *App) RemoveAccount(userID string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.DeleteAccount(userID); err != nil {
		return err
	}
	a.emit("account:changed", map[string]string{"removed": userID})
	return nil
}

// GetSettings loads app settings.
func (a *App) GetSettings() store.Settings {
	st, err := a.storeOrError()
	if err != nil {
		return store.Settings{}
	}
	s, _ := st.GetSettings()
	return s
}

// SaveSettings persists settings and applies runtime-relevant changes.
func (a *App) SaveSettings(s store.Settings) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.SetSettings(s); err != nil {
		return err
	}
	dl := a.downloadManager()
	mediaProxy := a.previewServer()
	// apply download directory at runtime
	if s.DownloadDir != "" && dl != nil {
		dl.SetDir(s.DownloadDir)
		if mediaProxy != nil {
			mediaProxy.AddRoot(s.DownloadDir)
		}
	}
	// apply concurrency limit at runtime
	if s.MaxConcurrentDownloads > 0 && dl != nil {
		dl.SetConcurrency(s.MaxConcurrentDownloads)
	}
	// apply proxy globally (affects netx clients + download engine)
	netx.SetGlobalProxy(s.Proxy)
	// apply upload speed cap at runtime (direct uploads via ProgressReader)
	netx.SetGlobalUploadRate(s.MaxUploadSpeed)
	return nil
}

// GetDirectoryCache returns a persisted directory snapshot. The key is
// supplied by the frontend and includes provider/account/mode/ directory
// identity, so snapshots from different accounts cannot be mixed.
func (a *App) GetDirectoryCache(key string) []model.File {
	st, err := a.storeOrError()
	if err != nil || strings.TrimSpace(key) == "" {
		return nil
	}
	list, err := st.LoadDirectoryCache(key)
	if err != nil {
		return nil
	}
	return list
}

// SaveDirectoryCache stores one directory snapshot under the install-local
// data/cache directory.
func (a *App) SaveDirectoryCache(key string, files []model.File) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("缓存键为空")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.SaveDirectoryCache(key, files)
}

// DeleteDirectoryCache invalidates one persisted directory snapshot.
func (a *App) DeleteDirectoryCache(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("缓存键为空")
	}
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteDirectoryCache(key)
}

// ClearCache removes directory/list cache only; account credentials and
// transfer/playback history remain intact.
func (a *App) ClearCache() error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	if err := st.ClearCache(); err != nil {
		return err
	}
	drive.ClearFileMetaCache()
	a.emit("cache:cleared", nil)
	return nil
}

// ---- transfer bindings ----

// DownloadFile enqueues a download.
func (a *App) DownloadFile(userID, driveID string, f model.File) (*model.DownloadTask, error) {
	dl := a.downloadManager()
	if dl == nil {
		return nil, errors.New("下载服务未启动")
	}
	return dl.AddDownload(userID, driveID, f)
}

// PinFileSnapshot keeps the exact list-row metadata available for a provider
// before preview/player code resolves an authenticated URL by file ID.
func (a *App) PinFileSnapshot(userID string, driveID string, f model.File) {
	drive.RememberFile(userID, driveID, f)
}

// DownloadURL enqueues a direct URL download.
func (a *App) DownloadURL(name, url string, headers map[string]string) (*model.DownloadTask, error) {
	dl := a.downloadManager()
	if dl == nil {
		return nil, errors.New("下载服务未启动")
	}
	return dl.AddDownloadURL(name, url, headers)
}

// ListDownloads lists download tasks.
func (a *App) ListDownloads() []model.DownloadTask {
	dl := a.downloadManager()
	if dl == nil {
		return nil
	}
	return dl.List()
}

// PauseDownload pauses a task.
func (a *App) PauseDownload(id string) {
	if dl := a.downloadManager(); dl != nil {
		dl.Pause(id)
	}
}

// ResumeDownload resumes a task.
func (a *App) ResumeDownload(id string) {
	if dl := a.downloadManager(); dl != nil {
		dl.Resume(id)
	}
}

// CancelDownload cancels a task.
func (a *App) CancelDownload(id string) {
	if dl := a.downloadManager(); dl != nil {
		dl.Cancel(id)
	}
}

// RemoveDownload hard-deletes a task record immediately (删除即从列表移除).
func (a *App) RemoveDownload(id string) {
	if dl := a.downloadManager(); dl != nil {
		dl.Remove(id)
	}
}

// PrioritizeDownload boosts one task: pauses others so it gets full bandwidth.
func (a *App) PrioritizeDownload(id string) {
	if dl := a.downloadManager(); dl != nil {
		dl.Prioritize(id)
	}
}

// ClearDownloads removes finished tasks.
func (a *App) ClearDownloads() {
	if dl := a.downloadManager(); dl != nil {
		dl.ClearCompleted()
	}
}

// UploadFiles enqueues uploads.
func canonicalUploadParent(userID, driveID, parentID string) string {
	if strings.TrimSpace(parentID) != "" && strings.TrimSpace(parentID) != "root" {
		return parentID
	}
	if root, err := drive.RootID(userID, driveID); err == nil && root != "" {
		return root
	}
	return parentID
}

// UploadFiles enqueues local files or folders for upload. conflictPolicy
// controls behavior when a same-name file already exists remotely:// "overwrite" (default), "rename" (keep both, append suffix), "skip".
func (a *App) UploadFiles(userID, driveID, parentID, conflictPolicy string, localPaths []string) []*model.UploadingUI {
	uploads := a.uploadQueue()
	if uploads == nil {
		return nil
	}
	// The frontend can briefly still hold the generic "root" sentinel while
	// provider metadata is loading. Canonicalize it before the asynchronous
	// queue starts resolving the remote parent.
	parentID = canonicalUploadParent(userID, driveID, parentID)
	return uploads.AddFiles(userID, driveID, parentID, conflictPolicy, localPaths)
}

// ListUploads lists upload jobs.
func (a *App) ListUploads() []model.UploadingUI {
	uploads := a.uploadQueue()
	if uploads == nil {
		return nil
	}
	return uploads.List()
}

// CancelUpload cancels an upload.
func (a *App) CancelUpload(id string) {
	if uploads := a.uploadQueue(); uploads != nil {
		uploads.Cancel(id)
	}
}

// ClearUploads removes finished uploads.
func (a *App) ClearUploads() {
	if uploads := a.uploadQueue(); uploads != nil {
		uploads.ClearCompleted()
	}
}

// ResumeUpload restarts a paused or failed upload job.
func (a *App) ResumeUpload(id string) error {
	uploads := a.uploadQueue()
	if uploads == nil {
		return errors.New("上传服务未启动")
	}
	return uploads.Resume(id)
}

// ---- preview ----

// PreviewURL builds a proxied URL for a media file.
func (a *App) PreviewURL(userID, driveID, fileID string) (string, error) {
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return "", fmt.Errorf("预览服务未启动")
	}
	u, err := drive.GetDownloadURL(userID, driveID, fileID, 3600)
	if err != nil {
		return "", err
	}
	name := ""
	if f, ferr := drive.GetFile(userID, driveID, fileID); ferr == nil {
		name = f.Name
	}
	return mediaProxy.ProxyURL(u.URL, u.Headers, name), nil
}

// LocalPreviewURL builds a local file URL.
func (a *App) LocalPreviewURL(path string) string {
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return ""
	}
	return mediaProxy.LocalURL(path)
}

// MediaProxy returns the internal server base URL.
func (a *App) MediaProxy() string {
	mediaProxy := a.previewServer()
	if mediaProxy == nil {
		return ""
	}
	return mediaProxy.BaseURL()
}

var _ = dlengine.DefaultConcurrency

// Startup is the exported Wails startup hook.
func (a *App) Startup(ctx context.Context) { a.startup(ctx) }

// MigrateFiles copies/moves files across two accounts.
func (a *App) MigrateFiles(srcUser, srcDrive, dstUser, dstDrive, dstParent string, fileIDs []string, move bool) (*migrate.Job, error) {
	if strings.TrimSpace(srcUser) == "" || strings.TrimSpace(srcDrive) == "" ||
		strings.TrimSpace(dstUser) == "" || strings.TrimSpace(dstDrive) == "" {
		return nil, fmt.Errorf("迁移账号信息不完整")
	}
	if len(fileIDs) == 0 {
		return nil, fmt.Errorf("请选择要迁移的文件")
	}
	srcCaps := drive.RegistryCaps(drive.ProviderOf(srcUser, srcDrive, ""))
	dstCaps := drive.RegistryCaps(drive.ProviderOf(dstUser, dstDrive, ""))
	if !srcCaps.Download {
		return nil, fmt.Errorf("源网盘不支持下载，无法迁移")
	}
	if !dstCaps.Upload && len(dstCaps.RapidUploadHashes) == 0 {
		return nil, fmt.Errorf("目标网盘不支持上传，无法迁移")
	}
	if dstParent == "" {
		dstParent = "root"
	}
	ids := append([]string(nil), fileIDs...)
	job := &migrate.Job{
		ID:      "mig-" + fmt.Sprint(time.Now().UnixNano()),
		SrcUser: srcUser, SrcDrive: srcDrive, FileIDs: ids,
		DstUser: dstUser, DstDrive: dstDrive, DstParent: dstParent, Move: move,
		Status: "pending",
	}
	eng := a.migrationEngine()
	if eng == nil {
		return nil, fmt.Errorf("迁移服务未启动")
	}
	st, err := a.storeOrError()
	if err != nil {
		return nil, err
	}
	if err := st.SaveMigrateJob(job); err != nil {
		return nil, err
	}
	a.emit("migrate:progress", *job)
	// derive from the app context so migrations are canceled on shutdown
	ctx := a.appContext()
	go func() { _ = eng.Run(ctx, job) }()
	return job, nil
}

// ListMigrateJobs returns persisted migration jobs, including jobs from a
// previous process which were recovered as canceled during startup.
func (a *App) ListMigrateJobs() []migrate.Job {
	st, err := a.storeOrError()
	if err != nil {
		return nil
	}
	list, err := st.ListMigrateJobs()
	if err != nil {
		return nil
	}
	return list
}

// CancelMigrate cancels an active migration without deleting its history.
func (a *App) CancelMigrate(id string) {
	if eng := a.migrationEngine(); eng != nil {
		eng.Cancel(id)
	}
}

// DeleteMigrateJob removes one migration history record.
func (a *App) DeleteMigrateJob(id string) error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.DeleteMigrateJob(id)
}

// ClearMigrateJobs removes all finished migration history records.
func (a *App) ClearMigrateJobs() error {
	st, err := a.storeOrError()
	if err != nil {
		return err
	}
	return st.ClearMigrateJobs()
}

// uploadSessionAdapter bridges store.UploadSession* into the
// drive.UploadSessionStore interface.
type uploadSessionAdapter struct{}

func (uploadSessionAdapter) SaveUploadSession(key string, partNumbers []int) error {
	return store.SaveUploadSession(key, partNumbers)
}

func (uploadSessionAdapter) SaveUploadSessionState(key, sessionID string, partNumbers []int) error {
	return store.SaveUploadSessionState(key, sessionID, partNumbers)
}
func (uploadSessionAdapter) LoadUploadSession(key string) []int {
	return store.LoadUploadSession(key)
}

func (uploadSessionAdapter) LoadUploadSessionState(key string) (string, []int) {
	return store.LoadUploadSessionState(key)
}
func (uploadSessionAdapter) ClearUploadSession(key string) {
	store.ClearUploadSession(key)
}
