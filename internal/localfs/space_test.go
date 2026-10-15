package localfs

import "testing"

func TestAvailableBytes(testContext *testing.T) {
	if _, err := AvailableBytes(testContext.TempDir()); err != nil {
		testContext.Fatal(err)
	}
	if _, err := AvailableBytes(testContext.TempDir() + "/missing"); err == nil {
		testContext.Fatal("不存在路径没有报错")
	}
}
