package dlengine

import (
	"encoding/json"
	"os"
)

func CheckpointProgress(localPath, resourceID string) (Progress, bool) {
	data, err := os.ReadFile(localPath + ".state.json")
	if err != nil {
		return Progress{}, false
	}
	var saved state
	if json.Unmarshal(data, &saved) != nil || !validResumeProgress(saved) {
		return Progress{}, false
	}
	if saved.ResourceHash != "" && (resourceID == "" || saved.ResourceHash != urlFingerprint(resourceID)) {
		return Progress{}, false
	}
	info, err := os.Stat(localPath + ".part")
	if err != nil || !info.Mode().IsRegular() {
		return Progress{}, false
	}
	var downloaded int64
	if saved.Stream {
		if info.Size() < saved.StreamBytes {
			return Progress{}, false
		}
		downloaded = saved.StreamBytes
	} else {
		if info.Size() != saved.Total {
			return Progress{}, false
		}
		for chunk, done := range saved.Done {
			if done {
				downloaded += chunkLen(&saved, chunk)
			} else if len(saved.PartialBytes) > 0 {
				downloaded += saved.PartialBytes[chunk]
			}
		}
	}
	return Progress{Downloaded: downloaded, Total: saved.Total, Percent: percent(downloaded, saved.Total)}, true
}
