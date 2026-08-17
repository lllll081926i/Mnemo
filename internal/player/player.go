// Package player controls an external mpv process via platform-native JSON IPC.
package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Command is a JSON IPC command.
type Command struct {
	Command   []any `json:"command"`
	RequestID int   `json:"request_id,omitempty"`
}

// Event is a JSON IPC event from mpv.
type Event struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
	RequestID int             `json:"request_id,omitempty"`
}

// Player controls an mpv process.
type Player struct {
	cmd          *exec.Cmd
	conn         io.ReadWriteCloser
	reader       *bufio.Reader
	mu           sync.Mutex
	ctx          context.Context
	ipcCleanup   func()
	ready        chan struct{}
	closed       chan struct{}
	closeOnce    sync.Once
	disconnected bool
	reqID        int
	pending      map[int]chan Event
	// callbacks
	OnEvent func(Event)
}

// Options carries mpv binary path and user data dir.
type Options struct {
	MpvPath   string
	ConfigDir string
	ExtraArgs []string
	Env       []string
}

// Start launches mpv with JSON IPC on a private platform-specific endpoint.
func Start(ctx context.Context, opts Options) (*Player, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(opts.MpvPath) == "" {
		return nil, errors.New("player: mpv path is empty")
	}
	if opts.ConfigDir != "" {
		if err := os.MkdirAll(opts.ConfigDir, 0o755); err != nil {
			return nil, fmt.Errorf("player: create config dir: %w", err)
		}
	}
	ipcAddress, cleanup := newIPCAddress()

	p := &Player{
		ctx:        ctx,
		ipcCleanup: cleanup,
		ready:      make(chan struct{}),
		closed:     make(chan struct{}),
		pending:    map[int]chan Event{},
	}
	args := []string{
		"--no-terminal",
		"--idle",
		"--input-ipc-server=" + ipcAddress,
		"--keep-open=yes",
		"--volume=50",
	}
	if opts.ConfigDir != "" {
		args = append(args, "--config-dir="+opts.ConfigDir)
	}
	args = append(args, opts.ExtraArgs...)

	p.cmd = exec.CommandContext(ctx, opts.MpvPath, args...)
	if len(opts.Env) > 0 {
		p.cmd.Env = mergeEnv(os.Environ(), opts.Env)
	}
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := p.cmd.Start(); err != nil {
		cleanup()
		return nil, err
	}

	// wait for IPC connection
	go func() {
		// discard stderr (mpv logs)
		_, _ = io.Copy(io.Discard, stderr)
	}()
	go func() {
		if err := p.cmd.Wait(); err != nil {
			p.disconnect(fmt.Errorf("player: mpv exited: %w", err))
		} else {
			p.disconnect(errors.New("player: mpv exited"))
		}
	}()

	// mpv creates the platform-specific IPC endpoint after process startup.
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var conn io.ReadWriteCloser
	for {
		conn, err = dialIPC(connectCtx, ipcAddress)
		if err == nil {
			break
		}
		select {
		case <-connectCtx.Done():
			_ = p.cmd.Process.Kill()
			p.disconnect(fmt.Errorf("player: connect IPC: %w", err))
			if errors.Is(connectCtx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("player: connect IPC: %w", err)
			}
			return nil, connectCtx.Err()
		case <-p.closed:
			return nil, errors.New("player: mpv exited before IPC was ready")
		case <-time.After(50 * time.Millisecond):
		}
	}
	p.mu.Lock()
	if p.disconnected {
		p.mu.Unlock()
		_ = conn.Close()
		return nil, errors.New("player: mpv exited before IPC was ready")
	}
	p.conn = conn
	p.reader = bufio.NewReader(conn)
	p.mu.Unlock()
	close(p.ready)

	go p.readLoop()
	return p, nil
}

func (p *Player) readLoop() {
	for {
		line, err := p.reader.ReadString('\n')
		if err != nil {
			p.disconnect(fmt.Errorf("player: IPC disconnected: %w", err))
			return
		}
		ev := Event{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &ev); err != nil {
			continue
		}
		if ev.RequestID > 0 {
			p.mu.Lock()
			ch, ok := p.pending[ev.RequestID]
			if ok {
				delete(p.pending, ev.RequestID)
			}
			p.mu.Unlock()
			if ok {
				ch <- ev
			}
		}
		if p.OnEvent != nil {
			p.OnEvent(ev)
		}
	}
}

// send writes a JSON command and returns the response.
func (p *Player) send(ctx context.Context, cmd Command) (Event, error) {
	if p == nil {
		return Event{}, errors.New("player: nil player")
	}
	if ctx == nil {
		ctx = p.ctx
		if ctx == nil {
			ctx = context.Background()
		}
	}
	select {
	case <-p.ready:
	case <-ctx.Done():
		return Event{}, ctx.Err()
	case <-p.closed:
		return Event{}, errors.New("player: closed")
	}
	p.mu.Lock()
	if p.disconnected || p.conn == nil {
		p.mu.Unlock()
		return Event{}, errors.New("player: IPC disconnected")
	}
	p.reqID++
	cmd.RequestID = p.reqID
	ch := make(chan Event, 1)
	id := p.reqID
	p.pending[id] = ch
	p.mu.Unlock()

	b, err := json.Marshal(cmd)
	if err != nil {
		p.removePending(id)
		return Event{}, err
	}
	p.mu.Lock()
	conn := p.conn
	if p.disconnected || conn == nil {
		p.mu.Unlock()
		p.removePending(id)
		return Event{}, errors.New("player: IPC disconnected")
	}
	_, err = conn.Write(append(b, '\n'))
	p.mu.Unlock()
	if err != nil {
		p.removePending(id)
		p.disconnect(err)
		return Event{}, err
	}
	select {
	case ev := <-ch:
		if ev.Error != "" {
			return ev, errors.New(ev.Error)
		}
		return ev, nil
	case <-ctx.Done():
		p.removePending(id)
		return Event{}, ctx.Err()
	case <-time.After(30 * time.Second):
		p.removePending(id)
		return Event{}, errors.New("player: command timeout")
	}
}

// LoadFile loads a URL or file path.
func (p *Player) LoadFile(ctx context.Context, url string) error {
	_, err := p.send(ctx, Command{Command: []any{"loadfile", url, "replace"}})
	return err
}

// Pause toggles pause.
func (p *Player) Pause(val bool) error {
	_, err := p.send(nil, Command{Command: []any{"set_property", "pause", val}})
	return err
}

// Seek seeks to a position in seconds.
func (p *Player) Seek(seconds float64) error {
	_, err := p.send(nil, Command{Command: []any{"seek", seconds, "absolute"}})
	return err
}

// SetVolume sets volume 0-100.
func (p *Player) SetVolume(v int) error {
	_, err := p.send(nil, Command{Command: []any{"set_property", "volume", v}})
	return err
}

// SetSpeed sets playback speed.
func (p *Player) SetSpeed(speed float64) error {
	_, err := p.send(nil, Command{Command: []any{"set_property", "speed", speed}})
	return err
}

// GetProperty returns an mpv property value.
func (p *Player) GetProperty(name string) (any, error) {
	ev, err := p.send(nil, Command{Command: []any{"get_property", name}})
	if err != nil {
		return nil, err
	}
	// parse the event data for the value
	var val any
	if ev.Data != nil {
		_ = json.Unmarshal(ev.Data, &val)
	}
	return val, nil
}

// Close stops mpv.
func (p *Player) Close() error {
	if p == nil {
		return nil
	}
	p.disconnect(errors.New("player: closed"))
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}

func (p *Player) removePending(id int) {
	p.mu.Lock()
	delete(p.pending, id)
	p.mu.Unlock()
}

func (p *Player) disconnect(err error) {
	p.closeOnce.Do(func() {
		close(p.closed)
		p.mu.Lock()
		p.disconnected = true
		conn := p.conn
		p.conn = nil
		cleanup := p.ipcCleanup
		p.ipcCleanup = nil
		pending := p.pending
		p.pending = map[int]chan Event{}
		p.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
		if cleanup != nil {
			cleanup()
		}
		message := "player: closed"
		if err != nil && err.Error() != "" {
			message = err.Error()
		}
		for _, ch := range pending {
			select {
			case ch <- Event{Error: message}:
			default:
			}
		}
	})
}

// Alive reports whether the IPC connection is still usable.
func (p *Player) Alive() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.disconnected && p.conn != nil
}

func mergeEnv(base, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))
	for _, item := range append(append([]string(nil), base...), overrides...) {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+values[key])
	}
	return out
}
