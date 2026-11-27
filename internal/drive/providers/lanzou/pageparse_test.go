package lanzou

import "testing"

func TestResolveLanzouAjaxURL(t *testing.T) {
	cases := []struct {
		name string
		html string
		base string
		want string
		ok   bool
	}{
		{
			name: "relative ajaxm",
			html: `<script>var x='/ajaxm.php?file=123';</script>`,
			base: "https://wwa.lanzouq.com/",
			want: "https://wwa.lanzouq.com/ajaxm.php?file=123",
			ok:   true,
		},
		{
			name: "absolute ajaxfile",
			html: `<script>url : "https://cdn.lanzoux.com/ajaxfile.php?file=456"</script>`,
			base: "https://wwa.lanzouq.com",
			want: "https://cdn.lanzoux.com/ajaxfile.php?file=456",
			ok:   true,
		},
		{
			name: "fallback fid var",
			html: `<script>var fid = '789';</script>`,
			base: "https://wwa.lanzouq.com",
			want: "https://wwa.lanzouq.com/ajaxm.php?file=789",
			ok:   true,
		},
		{
			name: "missing",
			html: `<script>var other = 1;</script>`,
			base: "https://wwa.lanzouq.com",
			want: "",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveLanzouAjaxURL(tc.html, tc.base)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("resolveLanzouAjaxURL() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
