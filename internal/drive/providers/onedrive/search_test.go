package onedrive

import (
	"encoding/json"
	"testing"
)

func TestMapSearchItemsParentReference(t *testing.T) {
	var items []Item
	data := []byte(`[
		{"id":"nested-file","name":"nested.txt","parentReference":{"id":"nested-folder"}},
		{"id":"root-file","name":"root.txt"}
	]`)
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	files := mapSearchItems(items, "drive")
	if len(files) != 2 {
		t.Fatalf("搜索结果数量 = %d，期望 2", len(files))
	}
	if files[0].ParentFileID != "nested-folder" {
		t.Errorf("嵌套文件父目录 = %q，期望 nested-folder", files[0].ParentFileID)
	}
	if files[1].ParentFileID != RootID {
		t.Errorf("缺少父目录信息时回退值 = %q，期望 %q", files[1].ParentFileID, RootID)
	}
}
