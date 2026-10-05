package dlengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func writePartialTestRange(writer http.ResponseWriter, start, end, total int64, body []byte) {
	writer.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	writer.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	writer.WriteHeader(http.StatusPartialContent)
	_, _ = writer.Write(body)
	writer.(http.Flusher).Flush()
}

func TestSingleConnectionRecoveryPreservesPartialChunkProgress(t *testing.T) {
	t.Parallel()
	payload := bytes.Repeat([]byte("thunder"), 10000)
	chunk := int64(len(payload) / 2)
	prefix := chunk / 2
	var probes, partialRequests atomic.Int32
	var resumedStart atomic.Int64
	resumedStart.Store(-1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"stable"`)
		start, end, ok := parseTestRange(request.Header.Get("Range"))
		if !ok {
			t.Error("恢复请求缺少 Range")
			writer.WriteHeader(400)
			return
		}
		if end == 0 {
			probes.Add(1)
		} else if probes.Load() == 1 && start == chunk {
			partialRequests.Add(1)
			writePartialTestRange(writer, start, end, int64(len(payload)), payload[start:start+prefix])
			time.Sleep(650 * time.Millisecond)
			return
		} else if probes.Load() == 1 && start > chunk {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		} else if probes.Load() > 1 && end != 0 {
			resumedStart.Store(start)
			time.Sleep(600 * time.Millisecond)
		}
		writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	var samples []Progress
	err := Download(t.Context(), Options{Concurrency: 2, ChunkSize: chunk, MinSize: 1, RetrySingleConnection: true}, server.URL, path, func(progress Progress) { samples = append(samples, progress) })
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(samples); index++ {
		if samples[index].Downloaded < samples[index-1].Downloaded || samples[index].Percent < samples[index-1].Percent {
			t.Fatalf("同一资源恢复时进度回退: %+v -> %+v", samples[index-1], samples[index])
		}
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) || probes.Load() != 2 || partialRequests.Load() != 1 || resumedStart.Load() != chunk+prefix {
		t.Fatalf("部分分片未从已写入位置续传: %v, probes=%d partial=%d start=%d", err, probes.Load(), partialRequests.Load(), resumedStart.Load())
	}
}

func TestPausedDownloadResumesPartialChunkWithoutDoubleCounting(t *testing.T) {
	payload := bytes.Repeat([]byte("resume"), 4096)
	prefix := int64(4096)
	var firstRequest atomic.Bool
	firstRequest.Store(true)
	var resumedStart atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("ETag", `"stable"`)
		start, end, ok := parseTestRange(request.Header.Get("Range"))
		if !ok {
			t.Error("无效的续传请求")
			writer.WriteHeader(400)
			return
		}
		if end != 0 && firstRequest.CompareAndSwap(true, false) {
			writePartialTestRange(writer, start, end, int64(len(payload)), payload[:prefix])
			select {
			case <-request.Context().Done():
			case <-time.After(5 * time.Second):
			}
			return
		}
		if end != 0 {
			resumedStart.Store(start)
		}
		writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	opts := Options{Concurrency: 1, ChunkSize: int64(len(payload)), MinSize: 1}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := Download(ctx, opts, server.URL, path, func(progress Progress) {
		if progress.Downloaded > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("暂停未返回取消错误: %v", err)
	}
	data, err := os.ReadFile(path + ".state.json")
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint state
	if err := json.Unmarshal(data, &checkpoint); err != nil || len(checkpoint.PartialBytes) != 1 || checkpoint.PartialBytes[0] != prefix || checkpoint.Done[0] {
		t.Fatalf("部分分片未保存: %v, %+v", err, checkpoint)
	}
	var first, last Progress
	count := 0
	if err := Download(t.Context(), opts, server.URL, path, func(progress Progress) {
		if count == 0 {
			first = progress
		}
		count++
		last = progress
	}); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) || resumedStart.Load() != prefix || first.Downloaded != prefix || last.Downloaded != int64(len(payload)) || last.Percent != 100 {
		t.Fatalf("续传重复计数或重下已完成前缀: %v, start=%d first=%+v last=%+v", err, resumedStart.Load(), first, last)
	}
}

func TestResumeProgressRejectsInvalidPartialOffsets(t *testing.T) {
	for _, sample := range []struct {
		name    string
		partial []int64
		done    []bool
		want    bool
	}{
		{name: "旧断点兼容", done: []bool{true, false}, want: true},
		{name: "合法前缀", partial: []int64{0, 1}, done: []bool{true, false}, want: true},
		{name: "负数前缀", partial: []int64{-1, 0}, done: []bool{false, false}},
		{name: "前缀超过分片", partial: []int64{17, 0}, done: []bool{false, false}},
		{name: "前缀等于分片", partial: []int64{16, 0}, done: []bool{false, false}},
		{name: "完成分片重复计数", partial: []int64{1, 0}, done: []bool{true, false}},
		{name: "末尾分片越界", partial: []int64{0, 8}, done: []bool{false, false}},
		{name: "数组不完整", partial: []int64{1}, done: []bool{false, false}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			checkpoint := state{Total: 24, Chunk: 16, Done: sample.done, PartialBytes: sample.partial}
			if got := validResumeProgress(checkpoint); got != sample.want {
				t.Fatalf("断点校验结果 = %v，期望 %v", got, sample.want)
			}
		})
	}
}

func TestPartialCheckpointCannotResumeChangedResource(t *testing.T) {
	payload := bytes.Repeat([]byte("new-content"), 4096)
	prefix := int64(1024)
	var usedOldPrefix atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start, end, _ := parseTestRange(request.Header.Get("Range"))
		if end != 0 && start == prefix {
			usedOldPrefix.Store(true)
		}
		writer.Header().Set("ETag", `"new"`)
		writeTestRange(writer, start, end, int64(len(payload)), payload[start:end+1])
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path+".part", bytes.Repeat([]byte{'!'}, len(payload)), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := &state{URLHash: urlFingerprint(server.URL), Total: int64(len(payload)), Chunk: int64(len(payload)), Done: []bool{false}, PartialBytes: []int64{prefix}, ETag: `"old"`}
	if err := persistState(path+".state.json", checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := Download(t.Context(), Options{Concurrency: 1, ChunkSize: int64(len(payload)), MinSize: 1}, server.URL, path, nil); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(actual, payload) || usedOldPrefix.Load() {
		t.Fatal("资源变化后复用了旧文件前缀")
	}
}
