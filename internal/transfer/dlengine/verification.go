package dlengine

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"mnemo-go/internal/netx"
)

var ErrHashMismatch = errors.New("下载内容哈希不匹配，文件未发布，请重新下载")

func ValidContentHash(algorithm, expected string) bool {
	hasher, err := netx.NewHash(netx.HashKind(strings.ToLower(strings.TrimSpace(algorithm))))
	if err != nil {
		return false
	}
	digest, err := hex.DecodeString(strings.TrimSpace(expected))
	return err == nil && len(digest) == hasher.Size()
}

func verifyBeforeCommit(ctx context.Context, opts Options, partPath, statePath string) error {
	if opts.ExpectedHash == "" {
		return ctx.Err()
	}
	if !ValidContentHash(opts.HashAlgorithm, opts.ExpectedHash) {
		return errors.New("下载校验算法或摘要无效，文件未发布")
	}
	info, err := os.Stat(partPath)
	if err != nil {
		return err
	}
	if opts.OnVerification != nil {
		opts.OnVerification(0, info.Size())
	}
	actual, err := netx.HashFileWithProgressContext(ctx, partPath, netx.HashKind(strings.ToLower(strings.TrimSpace(opts.HashAlgorithm))), func(done int64) {
		if opts.OnVerification != nil {
			opts.OnVerification(done, info.Size())
		}
	}, info.Size())
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, strings.TrimSpace(opts.ExpectedHash)) {
		if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%w；清理失效检查点失败: %v", ErrHashMismatch, err)
		}
		return ErrHashMismatch
	}
	return ctx.Err()
}
