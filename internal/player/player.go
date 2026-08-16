// Package player controls an external mpv process via JSON IPC over TCP.
package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Command is a JSON IPC command.
type Command struct {
	Command   []any  `json:"command"`
	RequestID int    `json:"request_id,omitempty"`
}

// Event is a JSON IPC event from mpv.
type Event struct {
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	RequestID int          `json:"request_id,omitempty"`
}

// Player controls an mpv process.
type Player struct {
	cmd    *exec.Cmd
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	port   int
	ready  chan struct{}
	reqID  int
	pending map[int]chan Event
	// callbacks
	OnEvent func(Event)
}

// Options carries mpv binary path and user data dir.
type Options struct {
	MpvPath    string
	ConfigDir  string
	ExtraArgs  []string
}

// Start launches mpv with JSON IPC on a random port.
func Start(ctx context.Context, opts Options) (*Player, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	p := &Player{
		port:    port,
		ready:   make(chan struct{}),
		pending: map[int]chan Event{},
	}
	args := []string{
		"--no-terminal",
		"--idle",
		"--input-ipc-server=127.0.0.1:" + strconv.Itoa(port),
		"--keep-open=yes",
		"--volume=50",
	}
	if opts.ConfigDir != "" {
		args = append(args, "--config-dir="+opts.ConfigDir)
	}
	args = append(args, opts.ExtraArgs...)

	p.cmd = exec.CommandContext(ctx, opts.MpvPath, args...)
	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := p.cmd.Start(); err != nil {
		return nil, err
	}

	// wait for IPC connection
	go func() {
		// discard stderr (mpv logs)
		_, _ = io.Copy(io.Discard, stderr)
	}()

	// connect to IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 5*time.Second)
	if err != nil {
		_ = p.cmd.Process.Kill()
		return nil, fmt.Errorf("player: connect %w", err)
	}
	p.conn = conn
	p.reader = bufio.NewReader(conn)
	close(p.ready)

	go p.readLoop()
	return p, nil
}

func (p *Player) readLoop() {
	for {
		line, err := p.reader.ReadString('\n')
		if err != nil {
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
func (p *Player) send(cmd Command) (Event, error) {
	<-p.ready
	p.mu.Lock()
	p.reqID++
	cmd.RequestID = p.reqID
	ch := make(chan Event, 1)
	p.pending[p.reqID] = ch
	p.mu.Unlock()

	b, _ := json.Marshal(cmd)
	p.mu.Lock()
	_, err := p.conn.Write(append(b, '\n'))
	p.mu.Unlock()
	if err != nil {
		return Event{}, err
	}
	select {
	case ev := <-ch:
		if ev.Error != "" {
			return ev, errors.New(ev.Error)
		}
		return ev, nil
	case <-time.After(30 * time.Second):
		return Event{}, errors.New("player: command timeout")
	}
}

// LoadFile loads a URL or file path.
func (p *Player) LoadFile(ctx context.Context, url string) error {
	_, err := p.send(Command{Command: []any{"loadfile", url, "replace"}})
	return err
}

// Pause toggles pause.
func (p *Player) Pause(val bool) error {
	_, err := p.send(Command{Command: []any{"set_property", "pause", val}})
	return err
}

// Seek seeks to a position in seconds.
func (p *Player) Seek(seconds float64) error {
	_, err := p.send(Command{Command: []any{"seek", seconds, "absolute"}})
	return err
}

// SetVolume sets volume 0-100.
func (p *Player) SetVolume(v int) error {
	_, err := p.send(Command{Command: []any{"set_property", "volume", v}})
	return err
}

// SetSpeed sets playback speed.
func (p *Player) SetSpeed(speed float64) error {
	_, err := p.send(Command{Command: []any{"set_property", "speed", speed}})
	return err
}

// GetProperty returns an mpv property value.
func (p *Player) GetProperty(name string) (any, error) {
	ev, err := p.send(Command{Command: []any{"get_property", name}})
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
	if p.conn != nil {
		_ = p.conn.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}