// Package app is the Wails binding layer. All methods on App are callable from
// the frontend; state changes are pushed via Events.Emit.
package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	_ "mnemo-go/internal/drive/providers" // register all plugins
	"mnemo-go/internal/drive/providers/pan189"
	"mnemo-go/internal/engine"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/preview"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer"
	"mnemo-go/internal/transfer/dlengine"
	"mnemo-go/internal/transfer/migrate"
	"os"
	"time"
)

// App is the root Wails-bound struct.
type App struct {
	ctx      context.Context
	store    *store.Store
	preview  *preview.Server
	dl       *transfer.Manager
	uploads  *transfer.UploadQueue
	secrets  config.Secrets
	dataDir  string
	player   *playback
	playerMu sync.Mutex
	schedStop chan struct{} // sync scheduler stop, closed on Shutdown
}

// NewApp constructs the app (no side effects; wiring happens in startup).
func NewApp() *App { return &App{} }

// startup initializes persistence, providers and transfer engines.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dir, err := config.UserDataDir("Mnemo-Go")
	if err != nil {
		dir = "."
	}
	a.dataDir = dir
	a.secrets = config.LoadSecrets(dir)

	st, err := store.Open(dir)
	if err != nil {
		panic(err)
	}
	a.store = st

	// token resolver so drive ops can load sessions
	drive.SetTokenResolver(func(userID, driveID string) (*model.TokenInfo, error) {
		acc, err := st.GetAccount(userID)
		if err != nil {
			return nil, err
		}
		return acc.Token, nil
	})

	// secret resolver so providers can read OAuth client ids during refresh
	drive.SetSecretResolver(func(key string) string {
		switch key {
		case "onedrive_client_id":
			return a.secrets.OnedriveClientID
		case "dropbox_app_key":
			return a.secrets.DropboxAppKey
		}
		return ""
	})

	// upload session persistence so providers can resume interrupted uploads
	store.InitUploadSessions(dir)
	drive.SetUploadSessionStore(uploadSessionAdapter{})

	// internal media/preview server. Roots restrict /local/ serving to the
	// download dir, engine dir and data dir (logs, mpv-config, etc.).
	dlDir := transfer.DownloadDir(st)
	engineDir := filepath.Join(dir, "engine")
	a.preview, err = preview.NewServer(dlDir, engineDir, dir)
	if err != nil {
		// preview is not optional for media playback; surface the error instead
		// of crashing silently on a nil Port access later.
		panic(fmt.Errorf("preview server: %w", err))
	}

	// download manager + upload queue
	a.dl, err = transfer.NewManager(st, dlDir, func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})
	if err != nil {
		panic(fmt.Errorf("download manager: %w", err))
	}
	a.uploads = transfer.NewUploadQueue(st, func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})

	// recover interrupted migrate jobs (mark as canceled, aligned with legacy)
	migrate.NewEngine(st, nil).RecoverUnfinished()

	// extract mpv engine
	if err := engine.Extract(engineDir); err != nil {
		// non-fatal: playback will report a clear error on demand.
		fmt.Fprintf(os.Stderr, "warn: mpv engine extract: %v\n", err)
	}

	// provider identity dir (pikpak device ids)
	_ = os.MkdirAll(filepath.Join(dir, "identity"), 0o755)

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

	a.emit("app:ready", map[string]any{"port": a.preview.Port})
}

// emit pushes an event to the frontend.
func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// shutdown is wired to Wails OnShutdown to release long-lived resources:
// the download manager, preview server and mpv player. Errors are logged to
// stderr because Wails ignores the returned error and we must not panic on
// the shutdown path.
func (a *App) Shutdown(ctx context.Context) {
	if a.player != nil && a.player.player != nil {
		_ = a.player.player.Close()
	}
	if a.dl != nil {
		a.dl.Shutdown()
	}
	if a.uploads != nil {
		a.uploads.Close()
	}
	if a.preview != nil {
		_ = a.preview.Close()
	}
	if a.schedStop != nil {
		close(a.schedStop)
	}
}

// OpenBrowser opens a URL in the system browser.
func (a *App) OpenBrowser(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
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
	return pan189.LastCaptchaImage
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
	if a.secrets.OnedriveClientID != "" {
		config["onedrive_client_id"] = a.secrets.OnedriveClientID
	}
	if a.secrets.DropboxAppKey != "" {
		config["dropbox_app_key"] = a.secrets.DropboxAppKey
	}
	tok, err := reg.Auth(context.Background(), drive.AuthRequest{
		Config: config,
		Open: func(url string) error {
			runtime.BrowserOpenURL(a.ctx, url)
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	if tok.TokenFrom == "" {
		tok.TokenFrom = provider
	}
	acc := &model.Account{
		UserID:  tok.UserID,
		DriveID: model.BuildDriveID(provider, model.StripUserID(provider, tok.UserID)),
		Token:   tok,
	}
	if acc.DriveID == "" {
		acc.DriveID = model.BuildDriveID(provider, tok.ProviderAccountID)
	}
	if err := a.store.SaveAccount(acc); err != nil {
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
	accountID := conn.Name
	if accountID == "" {
		accountID = conn.Endpoint
	}
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
	if err := a.store.SaveAccount(acc); err != nil {
		return nil, err
	}
	a.emit("account:changed", acc)
	return acc, nil
}

// ListAccounts returns all accounts.
func (a *App) ListAccounts() []*model.Account {
	list, err := a.store.ListAccounts()
	if err != nil {
		return nil
	}
	return list
}

// RefreshAccount silently refreshes an account's quota + profile from the
// provider, persists the updated token, and returns the refreshed account.
// Frontend polls this at a low frequency for the avatar/quota popover.
func (a *App) RefreshAccount(userID string) (*model.Account, error) {
	acc, err := a.store.GetAccount(userID)
	if err != nil || acc == nil {
		return nil, fmt.Errorf("账号不存在")
	}
	tok, err := drive.RefreshAccount(userID, acc.DriveID)
	if err != nil {
		return acc, nil
	}
	if tok != nil {
		acc.Token = tok
		if acc.DriveID == "" {
			acc.DriveID = model.BuildDriveID(tok.TokenFrom, model.StripUserID(tok.TokenFrom, tok.UserID))
		}
	}
	_ = a.store.SaveAccount(acc)
	a.emit("account:changed", acc)
	return acc, nil
}

// RemoveAccount deletes an account.
func (a *App) RemoveAccount(userID string) error {
	if err := a.store.DeleteAccount(userID); err != nil {
		return err
	}
	a.emit("account:changed", map[string]string{"removed": userID})
	return nil
}

// GetSettings loads app settings.
func (a *App) GetSettings() store.Settings {
	s, _ := a.store.GetSettings()
	return s
}

// SaveSettings persists settings and applies runtime-relevant changes.
func (a *App) SaveSettings(s store.Settings) error {
	if err := a.store.SetSettings(s); err != nil {
		return err
	}
	// apply download directory at runtime
	if s.DownloadDir != "" && a.dl != nil {
		a.dl.SetDir(s.DownloadDir)
	}
	// apply concurrency limit at runtime
	if s.MaxConcurrentDownloads > 0 && a.dl != nil {
		a.dl.SetConcurrency(s.MaxConcurrentDownloads)
	}
	// apply proxy globally (affects netx clients + download engine)
	netx.SetGlobalProxy(s.Proxy)
	// apply upload speed cap at runtime (direct uploads via ProgressReader)
	netx.SetGlobalUploadRate(s.MaxUploadSpeed)
	return nil
}

// ---- transfer bindings ----

// DownloadFile enqueues a download.
func (a *App) DownloadFile(userID, driveID string, f model.File) (*model.DownloadTask, error) {
	return a.dl.AddDownload(userID, driveID, f)
}

// DownloadURL enqueues a direct URL download.
func (a *App) DownloadURL(name, url string, headers map[string]string) (*model.DownloadTask, error) {
	return a.dl.AddDownloadURL(name, url, headers)
}

// ListDownloads lists download tasks.
func (a *App) ListDownloads() []model.DownloadTask { return a.dl.List() }

// PauseDownload pauses a task.
func (a *App) PauseDownload(id string) { a.dl.Pause(id) }

// ResumeDownload resumes a task.
func (a *App) ResumeDownload(id string) { a.dl.Resume(id) }

// CancelDownload cancels a task.
func (a *App) CancelDownload(id string) { a.dl.Cancel(id) }

// RemoveDownload hard-deletes a task record immediately (删除即从列表移除).
func (a *App) RemoveDownload(id string) { a.dl.Remove(id) }

// PrioritizeDownload boosts one task: pauses others so it gets full bandwidth.
func (a *App) PrioritizeDownload(id string) { a.dl.Prioritize(id) }

// ClearDownloads removes finished tasks.
func (a *App) ClearDownloads() { a.dl.ClearCompleted() }

// UploadFiles enqueues uploads.
func (a *App) UploadFiles(userID, driveID, parentID string, localPaths []string) []*model.UploadingUI {
	return a.uploads.AddFiles(userID, driveID, parentID, localPaths)
}

// ListUploads lists upload jobs.
func (a *App) ListUploads() []model.UploadingUI { return a.uploads.List() }

// CancelUpload cancels an upload.
func (a *App) CancelUpload(id string) { a.uploads.Cancel(id) }

// ClearUploads removes finished uploads.
func (a *App) ClearUploads() { a.uploads.ClearCompleted() }

// ResumeUpload restarts a paused or failed upload job.
func (a *App) ResumeUpload(id string) error { return a.uploads.Resume(id) }

// ---- preview ----

// PreviewURL builds a proxied URL for a media file.
func (a *App) PreviewURL(userID, driveID, fileID string) (string, error) {
	u, err := drive.GetDownloadURL(userID, driveID, fileID, 3600)
	if err != nil {
		return "", err
	}
	name := ""
	if f, ferr := drive.GetFile(userID, driveID, fileID); ferr == nil {
		name = f.Name
	}
	return a.preview.ProxyURL(u.URL, u.Headers, name), nil
}

// LocalPreviewURL builds a local file URL.
func (a *App) LocalPreviewURL(path string) string {
	return a.preview.LocalURL(path)
}

// MediaProxy returns the internal server base URL.
func (a *App) MediaProxy() string {
	if a.preview == nil {
		return ""
	}
	return a.preview.BaseURL()
}

var _ = dlengine.DefaultConcurrency

// Startup is the exported Wails startup hook.
func (a *App) Startup(ctx context.Context) { a.startup(ctx) }

// MigrateFiles copies/moves files across two accounts.
func (a *App) MigrateFiles(srcUser, srcDrive, dstUser, dstDrive, dstParent string, fileIDs []string, move bool) (*migrate.Job, error) {
	job := &migrate.Job{
		ID:       "mig-" + fmt.Sprint(time.Now().UnixNano()),
		SrcUser: srcUser, SrcDrive: srcDrive, FileIDs: fileIDs,
		DstUser: dstUser, DstDrive: dstDrive, DstParent: dstParent, Move: move,
	}
	eng := migrate.NewEngine(a.store, func(j *migrate.Job) {
		a.emit("migrate:progress", j)
	})
	// derive from the app context so migrations are canceled on shutdown
	go func() { _ = eng.Run(a.ctx, job) }()
	return job, nil
}

// uploadSessionAdapter bridges store.UploadSession* into the
// drive.UploadSessionStore interface.
type uploadSessionAdapter struct{}

func (uploadSessionAdapter) SaveUploadSession(key string, partNumbers []int) error {
	return store.SaveUploadSession(key, partNumbers)
}
func (uploadSessionAdapter) LoadUploadSession(key string) []int {
	return store.LoadUploadSession(key)
}
func (uploadSessionAdapter) ClearUploadSession(key string) {
	store.ClearUploadSession(key)
}
