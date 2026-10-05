package dlengine

import (
	"math"
	"testing"
)

func TestPercentStaysWithinBounds(t *testing.T) {
	for _, test := range []struct {
		name  string
		done  int64
		total int64
		want  int
	}{
		{name: "未知大小", done: 10, total: 0, want: 0},
		{name: "未开始", done: 0, total: 100, want: 0},
		{name: "负数进度", done: -5, total: 100, want: 0},
		{name: "正常进度", done: 42, total: 100, want: 42},
		{name: "小数舍入边界", done: 29, total: 100, want: 29},
		{name: "另一舍入边界", done: 58, total: 100, want: 58},
		{name: "已完成", done: 100, total: 100, want: 100},
		{name: "超过预估大小", done: 29711359, total: 1, want: 100},
		{name: "大文件不溢出", done: math.MaxInt64 / 2, total: math.MaxInt64, want: 49},
		{name: "大文件过半", done: math.MaxInt64/2 + 1, total: math.MaxInt64, want: 50},
		{name: "大文件未完成不舍入到100", done: math.MaxInt64 - 1, total: math.MaxInt64, want: 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := percent(test.done, test.total); got != test.want {
				t.Fatalf("下载百分比 = %d，预期 %d", got, test.want)
			}
		})
	}
}
