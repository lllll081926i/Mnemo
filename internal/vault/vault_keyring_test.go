package vault

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDesktopKeyringRoundTripAndUnavailableFailClosed(t *testing.T) {
	for _, platform := range []string{"darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			dir := t.TempDir()
			key := bytes.Repeat([]byte{0x5a}, 32)
			encoded := hex.EncodeToString(key)
			backend := desktopKeyring{platform: platform, run: func(program string, arguments []string, input string) ([]byte, error) {
				if strings.Contains(strings.Join(arguments, " "), encoded) {
					t.Fatal("密钥不得进入进程参数")
				}
				if input != "" {
					if !strings.Contains(input, encoded) {
						t.Fatal("未通过标准输入传递密钥")
					}
					return nil, nil
				}
				return []byte(encoded + "\n"), nil
			}}
			if err := backend.write(dir, key); err != nil {
				t.Fatal(err)
			}
			if got, err := backend.read(dir); err != nil || !bytes.Equal(got, key) {
				t.Fatalf("系统凭据读取不一致：%v", err)
			}
			if err := backend.write(dir, key); err == nil {
				t.Fatal("不应覆盖已有系统凭据")
			}
			backend.run = func(string, []string, string) ([]byte, error) { return nil, exec.ErrNotFound }
			if _, err := backend.read(dir); err == nil || errors.Is(err, exec.ErrNotFound) {
				t.Fatal("系统凭据丢失不应降级生成新密钥")
			}
		})
	}
}
