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
	_ "mnemo-go/internal/drive/providers" // register all plugins
	"mnemo-go/internal/engine"
	"mnemo-go/internal/model"
	"mnemo-go/internal/preview"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer"
	"mnemo-go/internal/transfer/migrate"
	"time"
	"mnemo-go/internal/transfer/dlengine"
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

	// internal media/preview server
	a.preview, _ = preview.NewServer()

	// download manager + upload queue
	a.dl, _ = transfer.NewManager(st, transfer.DownloadDir(st), func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})
	a.uploads = transfer.NewUploadQueue(st, func(ev transfer.TaskEvent) {
		a.emit("transfer:event", ev)
	})

	// extract mpv engine
	_ = engine.Extract(filepath.Join(dir, "engine"))

	// provider identity dir (pikpak device ids)
	_ = filepath.Join(dir, "identity")

	a.emit("app:ready", map[string]any{"port": a.preview.Port})
}

// emit pushes an event to the frontend.
func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data)
	}
}

// OpenBrowser opens a URL in the system browser.
func (a *App) OpenBrowser(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// ---- providers ----

// ListProviders returns the 13 in-scope drive providers with metas + caps.
func (a *App) ListProviders() []drive.Registration {
	regs := drive.All()
	out := make([]drive.Registration, 0, len(regs))
	for _, r := range regs {
		out = append(out, r)
	}
	return out
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

// SaveSettings persists settings and applies them.
func (a *App) SaveSettings(s store.Settings) error {
	return a.store.SetSettings(s)
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

// ---- preview ----

// PreviewURL builds a proxied URL for a media file.
func (a *App) PreviewURL(userID, driveID, fileID string) (string, error) {
	u, err := drive.GetDownloadURL(userID, driveID, fileID, 3600)
	if err != nil {
		return "", err
	}
	return a.preview.ProxyURL(u.URL, u.Headers), nil
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
		ID: "mig-" + fmt.Sprint(time.Now().UnixNano()),
		SrcUser: srcUser, SrcDrive: srcDrive, FileIDs: fileIDs,
		DstUser: dstUser, DstDrive: dstDrive, DstParent: dstParent, Move: move,
	}
	eng := migrate.NewEngine(func(j *migrate.Job) {
		a.emit("migrate:progress", j)
	})
	go eng.Run(context.Background(), job)
	return job, nil
}
