package pikpak

import (
	"testing"
	"time"
)

func TestInvalidShareExpirationDoesNotBecomePermanent(testContext *testing.T) {
	for _, value := range []string{"invalid", "-2", time.Now().Add(-time.Hour).Format(time.RFC3339)} {
		if _, err := pikpakExpirationDays(value); err == nil {
			testContext.Fatalf("无效有效期被忽略: %s", value)
		}
	}
	for _, value := range []string{"", "0", "-1"} {
		if days, err := pikpakExpirationDays(value); err != nil || days != -1 {
			testContext.Fatalf("永久选项异常: %s, %d, %v", value, days, err)
		}
	}
}
