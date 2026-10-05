package driveutil

import "testing"

func TestShareURLChecksSchemeAndDomainBoundary(testContext *testing.T) {
	for _, testCase := range []struct {
		raw   string
		valid bool
	}{
		{"https://www.alipan.com/s/share", true},
		{"alipan.com/s/share", true},
		{"//alipan.com/s/share", true},
		{"https://ALIPAN.COM/s/share", true},
		{"https://alipan.com.attacker.test/s/share", false},
		{"https://fakealipan.com/s/share", false},
		{"https://alipan.com@attacker.test/s/share", false},
		{"https://attacker@alipan.com/s/share", false},
		{"ftp://alipan.com/s/share", false},
		{"https://www.123pan.com/s/share", false},
	} {
		testContext.Run(testCase.raw, func(testContext *testing.T) {
			_, err := ParseShareURL(testCase.raw, "alipan.com")
			if (err == nil) != testCase.valid {
				testContext.Fatalf("链接校验错误: %v", err)
			}
		})
	}
}
