package lanzou

import (
	"strings"
	"testing"
)

func TestRemoveNotes(t *testing.T) {
	html := `<!-- c1 -->var a = "https://x.com/f"; // tail
/* block */var b = 1;`
	out := removeNotes(html)
	if !strings.Contains(out, "https://x.com/f") {
		t.Errorf("https URL lost: %q", out)
	}
	for _, gone := range []string{"c1", "tail", "block"} {
		if strings.Contains(out, gone) {
			t.Errorf("%q still present in %q", gone, out)
		}
	}
}

func TestRemoveJSComment(t *testing.T) {
	src := `var a = 1; /* x */ var b = "ok"; // line
var c = 2;`
	out := removeJSComment(src)
	for _, keep := range []string{"var a = 1;", `"ok"`, "var c = 2;"} {
		if !strings.Contains(out, keep) {
			t.Errorf("%q lost from %q", keep, out)
		}
	}
	if strings.Contains(out, "x */") {
		t.Errorf("block comment survived: %q", out)
	}
}

func TestHtmlJsonToMap(t *testing.T) {
	html := `
      var sg = 'AbCdEf';
      var ajaxdata = '?ctdf';
      $.ajax({
        data : {
          'action':'downprocess',
          'sign':sg,
          'websign':'',
          'kd':1,
          'p':'123'
        }
      })`
	param, err := htmlJsonToMap(html)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"action":  "downprocess",
		"sign":    "AbCdEf",
		"websign": "",
		"kd":      "1",
		"p":       "123",
	}
	for k, v := range want {
		if param[k] != v {
			t.Errorf("param[%q] = %q, want %q", k, param[k], v)
		}
	}
}

func TestHtmlJsonToMapMissingData(t *testing.T) {
	if _, err := htmlJsonToMap("<html>nothing</html>"); err == nil {
		t.Fatal("expected error for missing data parameter")
	}
}

func TestGetJSFunctionByName(t *testing.T) {
	html := `function other() { return {a:1}; }
function down_p() {
  var x = { 'a':'1' };
  if (x) { x.a = '2'; }
  $.ajax({ data : {'sign':'S','p':''} });
}`
	fn, err := getJSFunctionByName(html, "down_p")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fn, "function down_p(") {
		t.Errorf("fn prefix mismatch: %q", fn)
	}
	if !strings.Contains(fn, "data :") {
		t.Errorf("fn body incomplete: %q", fn)
	}
	if _, err := getJSFunctionByName(html, "nope"); err == nil {
		t.Fatal("expected error for missing function")
	}
}

func TestParseAjaxmResp(t *testing.T) {
	ok, err := parseAjaxmResp(`{"zt":1,"dom":"https://c1.lanosso.com","url":"abc/file.zip","inf":"f.zip"}`)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Dom != "https://c1.lanosso.com" || ok.URL != "abc/file.zip" || ok.Inf != "f.zip" {
		t.Errorf("parseAjaxmResp = %+v", ok)
	}
	if _, err := parseAjaxmResp(`{"zt":3,"inf":"密码错误"}`); err == nil || err.Error() != "密码错误" {
		t.Errorf("expected 密码错误 error, got %v", err)
	}
}
