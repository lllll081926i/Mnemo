// Package app is the Wails binding layer. All methods on App are callable from
// the frontend; state changes are pushed via Events.Emit.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sync/singleflight"

	"mnemo-go/internal/captcha"
	"mnemo-go/internal/config"
	"mnemo-go/internal/drive"
	"mnemo-go/internal/drive/driveutil"
	_ "mnemo-go/internal/drive/providers" // register all plugins
	"mnemo-go/internal/drive/providers/pikpak"
	"mnemo-go/internal/logging"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
	"mnemo-go/internal/preview"
	"mnemo-go/internal/store"
	"mnemo-go/internal/transfer"
	"mnemo-go/internal/transfer/migrate"
	"mnemo-go/internal/updater"
)

// App is the root Wails-bound struct.
type App struct {
	stateMu    sync.RWMutex
	persistMu  sync.Mutex
	persistErr error
	ctx        context.Context
	store      *store.Store
	preview    *preview.Server
	dl         *transfer.Manager
	uploads    *transfer.UploadQueue
	secrets    config.Secrets
	dataDir    string

	updateMu   sync.Mutex
	updateInfo *updater.Info

	migrate      *migrate.Engine
	schedStop    chan struct{} // sync scheduler stop, closed on Shutdown
	shutdownOnce sync.Once
	forceQuit    atomic.Bool

	accountRefreshMu         sync.Mutex
	accountRefreshLast       map[string]time.Time
	accountRefreshRetryAfter map[string]time.Time
	accountRefreshGroup      singleflight.Group

	transferLogMu       sync.Mutex
	transferCommandLogs map[string]*transferCommandLog

	syncRunMu sync.Mutex
	syncRuns  map[string]*activeSyncRun
}

// transferCommandLog coalesces a burst of identical controls (for example,
// "pause all") into one start/completion pair instead of one pair per task.
// Individual task IDs remain available at debug level through the request log.
type transferCommandLog struct {
	count   int
	started time.Time
	timer   *time.Timer
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
		logging.Error("application startup skipped", "reason", "nil app")
		return
	}
	startupAt := time.Now()
	logging.Info("application startup started")
	a.stateMu.Lock()
	a.ctx = ctx
	a.stateMu.Unlock()
	st, err := a.ensurePersistence()
	if err != nil {
		logging.Error("persistence initialization failed", "error", err)
		panic(fmt.Errorf("store: %w", err))
	}
	dataDir := a.dataDirectory()
	secrets := a.secretsSnapshot()
	if err := logging.Configure(dataDir); err != nil {
		logging.Warn("persistent logging unavailable", "error", err, "data_dir", dataDir)
	} else {
		logging.Info("persistent logging enabled", "file", logging.Path())
	}
	if settings, settingsErr := st.GetSettings(); settingsErr == nil {
		if settings.LogLevel == "" {
			settings.LogLevel = "info"
		}
		if levelErr := logging.SetLevel(settings.LogLevel); levelErr != nil {
			logging.Warn("invalid persisted log level, using info", "value", settings.LogLevel, "error", levelErr)
			_ = logging.SetLevel("info")
		}
	} else {
		logging.Warn("failed to load persisted log level", "error", settingsErr)
	}

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
		case "dropbox_redirect_uri":
			return secrets.DropboxRedirectURI
		}
		return ""
	})

	// upload session persistence so providers can resume interrupted uploads
	store.InitUploadSessions(dataDir)
	drive.SetUploadSessionStore(uploadSessionAdapter{})

	// Internal media/preview server. /local/ access is restricted to exact
	// registered files under the download directory. The application data
	// directory is excluded because it contains settings, logs and task state.
	dlDir := transfer.DownloadDir(st)
	mediaProxy, err := preview.NewServer(dlDir)
	if err != nil {
		logging.Error("preview server initialization failed", "error", err)
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
		logging.Error("download manager initialization failed", "error", err)
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
	driveutil.SetUploadThrottle(netx.WaitGlobalUpload)
	if s, err := st.GetSettings(); err == nil {
		netx.SetGlobalUploadRate(s.MaxUploadSpeed)
	}

	logging.Info("application startup completed", "preview_port", mediaProxy.Port, "duration", logging.Duration(startupAt))
	if message := a.StartSyncScheduler(); message != "" {
		logging.Warn("sync scheduler startup failed", "error", message)
	}
	a.emit("app:ready", map[string]any{"port": mediaProxy.Port})
}

// emit pushes an event to the frontend.
func (a *App) emit(name string, data any) {
	logging.Debug("frontend event emitted", "event", name)
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
		shutdownAt := time.Now()
		logging.Info("application shutdown started")
		captcha.Close()
		a.stateMu.Lock()
		migrations, downloads, uploads, mediaProxy, stop := a.migrate, a.dl, a.uploads, a.preview, a.schedStop
		a.schedStop = nil
		if stop != nil {
			close(stop)
		}
		a.stateMu.Unlock()
		a.cancelAllSyncRuns()
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
		logging.Info("application shutdown completed", "duration", logging.Duration(shutdownAt))
		logging.Close()
	})
}

// Startup is the exported Wails startup hook.
func (a *App) Startup(ctx context.Context) { a.startup(ctx) }
