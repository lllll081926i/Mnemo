package dlengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointProgressRejectsInvalidLocalData(t *testing.T) {
	for _, sample := range []struct {
		name     string
		state    state
		fileSize int64
	}{
		{name: "稀疏文件长度不能充当下载进度", state: state{Total: 100, Chunk: 50, Done: []bool{false, false}}, fileSize: 100},
		{name: "缺失本地数据", state: state{Total: 100, Chunk: 50, Done: []bool{true, false}}, fileSize: -1},
		{name: "截断分片文件", state: state{Total: 100, Chunk: 50, Done: []bool{true, false}}, fileSize: 50},
		{name: "截断单流文件", state: state{Stream: true, Total: 100, StreamBytes: 50}, fileSize: 49},
		{name: "负数单流偏移", state: state{Stream: true, Total: 100, StreamBytes: -1}, fileSize: 50},
		{name: "单流越界", state: state{Stream: true, Total: 100, StreamBytes: 101}, fileSize: 101},
		{name: "单流不能携带分片位图", state: state{Stream: true, Total: 100, StreamBytes: 50, Done: []bool{true}}, fileSize: 50},
		{name: "不同资源", state: state{Stream: true, Total: 100, StreamBytes: 50, ResourceHash: urlFingerprint("other")}, fileSize: 50},
	} {
		t.Run(sample.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file")
			if sample.fileSize >= 0 {
				if err := os.WriteFile(path+".part", make([]byte, sample.fileSize), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := persistState(path+".state.json", &sample.state); err != nil {
				t.Fatal(err)
			}
			progress, ok := CheckpointProgress(path, "file")
			if sample.name == "稀疏文件长度不能充当下载进度" {
				if !ok || progress.Downloaded != 0 {
					t.Fatal("稀疏文件被误认为已下载")
				}
			} else if ok {
				t.Fatalf("错误的本地断点被接受：%+v", progress)
			}
		})
	}
}

func TestCheckpointNormalizesFullyWrittenPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path+".part", make([]byte, 100), 0600); err != nil {
		t.Fatal(err)
	}
	saved := &state{Total: 100, Chunk: 50, Done: []bool{false, false}, PartialBytes: []int64{50, 20}}
	if err := persistState(path+".state.json", saved); err != nil {
		t.Fatal(err)
	}
	if progress, ok := CheckpointProgress(path, ""); !ok || progress.Downloaded != 70 {
		t.Fatalf("完整前缀的周期检查点无效：%+v, %v", progress, ok)
	}
	if saved.Done[0] || saved.PartialBytes[0] != 50 {
		t.Fatal("检查点保存修改了仍在使用的分片状态")
	}
}

func TestWeakETagAloneCannotAuthorizeResume(t *testing.T) {
	if resumeIdentityMatches(state{ETag: `W/"stable"`}, resourceValidator{ETag: `W/"stable"`}) {
		t.Fatal("弱 ETag 不能单独确认字节身份")
	}
	previous := state{ETag: `W/"stable"`, LastModified: probeModified}
	if !resumeIdentityMatches(previous, resourceValidator{ETag: previous.ETag, LastModified: previous.LastModified}) || resumeIdentityMatches(previous, resourceValidator{ETag: previous.ETag, LastModified: rangeModified}) {
		t.Fatal("弱 ETag 没有同时校验修改时间")
	}
}
