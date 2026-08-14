package lanzou

import "testing"

func TestSolveAcwScV2(t *testing.T) {
	// vector ported from the legacy acwScV2.test.ts
	html := `<script>var arg1='00112233445566778899AABBCCDDEEFF00112233';</script>`
	got := solveAcwScV2(html)
	want := "41eb1062441a5dadc03039c05aff6731a59d0124"
	if got != want {
		t.Fatalf("solveAcwScV2 = %q, want %q", got, want)
	}
}

func TestSolveAcwScV2EmptyOnNormalPage(t *testing.T) {
	if got := solveAcwScV2(`{"zt":1,"info":"ok"}`); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if isAcwScV2Challenge(`{"zt":1}`) {
		t.Fatal("normal page must not be a challenge")
	}
	if !isAcwScV2Challenge(`<script>var arg1='00112233445566778899AABBCCDDEEFF00112233';</script>`) {
		t.Fatal("challenge page must be detected")
	}
}

func TestMergeAcwCookie(t *testing.T) {
	cases := []struct{ cookie, acw, want string }{
		{"ylogin=1; PHPSESSID=abc", "V1", "ylogin=1; PHPSESSID=abc; acw_sc__v2=V1"},
		{"ylogin=1; acw_sc__v2=OLD", "V2", "ylogin=1; acw_sc__v2=V2"},
		{"", "V3", "acw_sc__v2=V3"},
	}
	for _, c := range cases {
		if got := mergeAcwCookie(c.cookie, c.acw); got != c.want {
			t.Errorf("mergeAcwCookie(%q, %q) = %q, want %q", c.cookie, c.acw, got, c.want)
		}
	}
}
