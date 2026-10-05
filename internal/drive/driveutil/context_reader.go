package driveutil

import (
	"context"
	"io"
)

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func NewContextReader(ctx context.Context, reader io.Reader) io.Reader {
	return &contextReader{ctx: ctx, reader: reader}
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
