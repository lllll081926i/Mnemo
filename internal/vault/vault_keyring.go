package vault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const keyringMarker = "vault.keyring"

type desktopKeyring struct {
	platform string
	run      func(string, []string, string) ([]byte, error)
}

func runKeyringCommand(program string, arguments []string, input string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, arguments...)
	command.Stdin = strings.NewReader(input)
	return command.Output()
}

func keyringIdentity(dir string) (string, bool, error) {
	marker, err := os.ReadFile(filepath.Join(dir, keyringMarker))
	if err == nil {
		identifier := strings.TrimSpace(string(marker))
		decoded, decodeErr := hex.DecodeString(identifier)
		if decodeErr != nil || len(decoded) != sha256.Size {
			return "", true, errors.New("系统凭据关联记录损坏，请恢复原记录，不要删除密钥")
		}
		return hex.EncodeToString(decoded), true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}
	identifier := sha256.Sum256([]byte(filepath.Clean(absolute)))
	return hex.EncodeToString(identifier[:]), false, nil
}

func (backend desktopKeyring) lookup(identifier string) ([]byte, error) {
	switch backend.platform {
	case "darwin":
		return backend.run("/usr/bin/security", []string{"find-generic-password", "-s", "Mnemo", "-a", identifier, "-w"}, "")
	case "linux":
		return backend.run("secret-tool", []string{"lookup", "application", "Mnemo", "vault", identifier}, "")
	default:
		return nil, exec.ErrNotFound
	}
}

func (backend desktopKeyring) read(dir string) ([]byte, error) {
	identifier, bound, err := keyringIdentity(dir)
	if err != nil {
		return nil, err
	}
	encoded, err := backend.lookup(identifier)
	if err != nil {
		var exitError *exec.ExitError
		missing := errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist)
		if errors.As(err, &exitError) {
			missing = (backend.platform == "darwin" && exitError.ExitCode() == 44) || (backend.platform == "linux" && exitError.ExitCode() == 1)
		}
		if missing && !bound {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("系统凭据无法读取，请解锁 Keychain 或 Secret Service；拒绝替换已有密钥")
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("系统凭据的密钥格式无效，拒绝自动覆盖")
	}
	if !bound {
		if err := writePrivateFileAtomically(filepath.Join(dir, keyringMarker), []byte(identifier)); err != nil {
			return nil, err
		}
	}
	return key, nil
}

func (backend desktopKeyring) write(dir string, key []byte) error {
	if len(key) != 32 {
		return errors.New("系统凭据的密钥长度无效")
	}
	identifier, bound, err := keyringIdentity(dir)
	if err != nil {
		return err
	}
	if bound {
		return errors.New("已有系统凭据关联，拒绝自动覆盖")
	}
	encoded := hex.EncodeToString(key)
	switch backend.platform {
	case "darwin":
		_, err = backend.run("/usr/bin/security", []string{"-i"}, fmt.Sprintf("add-generic-password -s Mnemo -a %s -w %s\n", identifier, encoded))
	case "linux":
		_, err = backend.run("secret-tool", []string{"store", "--label=Mnemo credential key", "application", "Mnemo", "vault", identifier}, encoded)
	default:
		return os.ErrNotExist
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	if err != nil {
		return errors.New("系统凭据保存失败，请解锁 Keychain 或 Secret Service")
	}
	saved, err := backend.lookup(identifier)
	if err != nil || !bytes.Equal(bytes.TrimSpace(saved), []byte(encoded)) {
		return errors.New("系统凭据保存后核对失败，不采用未确认的密钥")
	}
	return writePrivateFileAtomically(filepath.Join(dir, keyringMarker), []byte(identifier))
}
