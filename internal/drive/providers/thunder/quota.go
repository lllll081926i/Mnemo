package thunder

import (
	"bytes"
	"context"
	"errors"
	"net/http"
)

type quotaBytes integer

func (value *quotaBytes) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == `""` || string(data) == "null" {
		return errors.New("迅雷容量字段不能为空")
	}
	var parsed integer
	if err := parsed.UnmarshalJSON(data); err != nil {
		return err
	}
	*value = quotaBytes(parsed)
	return nil
}

func (cl *client) refreshQuota(ctx context.Context) error {
	var result struct {
		Quota *struct {
			Limit *quotaBytes `json:"limit"`
			Usage *quotaBytes `json:"usage"`
		} `json:"quota"`
	}
	if err := cl.request(ctx, http.MethodGet, "/about", nil, &result); err != nil {
		return err
	}
	if result.Quota == nil || result.Quota.Limit == nil || result.Quota.Usage == nil || *result.Quota.Limit <= 0 {
		return errors.New("迅雷容量响应缺少有效的总容量或已用容量")
	}
	cl.token.TotalSize = int64(*result.Quota.Limit)
	cl.token.UsedSize = int64(*result.Quota.Usage)
	cl.token.FreeSize = max(0, cl.token.TotalSize-cl.token.UsedSize)
	cl.changed = true
	return nil
}
