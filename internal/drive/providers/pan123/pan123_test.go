package pan123

import (
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ---- signPath / crc32 ----

func TestCRC32StandardVector(t *testing.T) {
	if got := crc32sum("123456789"); got != 0xCBF43926 {
		t.Fatalf("crc32sum(123456789) = %08x, want cbf43926", got)
	}
}

func TestSignTableDigitMapping(t *testing.T) {
	// digits 0-9 must map to table[0..9] = "adefghlmyi"
	want := map[byte]byte{
		'0': 'a', '1': 'd', '2': 'e', '3': 'f', '4': 'g',
		'5': 'h', '6': 'l', '7': 'm', '8': 'y', '9': 'i',
	}
	for d, w := range want {
		if signTable[d-'0'] != w {
			t.Fatalf("signTable[%c] = %c, want %c", d, signTable[d-'0'], w)
		}
	}
}

var numPartsRe = regexp.MustCompile(`^\d+-\d+-\d+$`)

func TestSignPathStructure(t *testing.T) {
	timeSign, dataSign := signPath("/b/api/file/list/new")
	if !regexp.MustCompile(`^\d+$`).MatchString(timeSign) {
		t.Fatalf("timeSign = %q, want decimal string", timeSign)
	}
	if !numPartsRe.MatchString(dataSign) {
		t.Fatalf("dataSign = %q, want ts-random-dataSign", dataSign)
	}
	// different paths (with same wall clock) must produce distinct dataSign
	// because apiPath feeds crc32 — verify the value differs for a different path.
	_, dataSign2 := signPath("/b/api/file/rename")
	if dataSign == dataSign2 {
		t.Fatalf("dataSign identical across different paths: %q", dataSign)
	}
}

func TestWithAPISign(t *testing.T) {
	raw := "https://yun.123pan.com/b/api/file/list/new"
	signed := withAPISign(raw)
	if !strings.HasPrefix(signed, raw+"?") {
		t.Fatalf("signed url = %q, want prefix %q", signed, raw+"?")
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Query()) != 1 {
		t.Fatalf("expected exactly 1 sign param, got %d", len(u.Query()))
	}
	for k, v := range u.Query() {
		if len(v) != 1 {
			t.Fatalf("param %s has %d values", k, len(v))
		}
		if !regexp.MustCompile(`^\d+$`).MatchString(k) {
			t.Fatalf("sign key %q is not decimal", k)
		}
		if !numPartsRe.MatchString(v[0]) {
			t.Fatalf("sign value %q is not ts-random-dataSign", v[0])
		}
	}
	// query params must not break signing
	signed2 := withAPISign(raw + "?driveId=0")
	if !strings.Contains(signed2, "driveId=0") {
		t.Fatalf("existing query lost: %q", signed2)
	}
}

// ---- id helpers ----

func TestToPan123FileID(t *testing.T) {
	cases := map[string]string{
		"":            "0",
		"pan123_root": "0",
		"root":        "0",
		"/":           "0",
		"0":           "0",
		"123":         "123",
		" 42 ":        "42",
	}
	for in, want := range cases {
		if got := toPan123FileID(in); got != want {
			t.Errorf("toPan123FileID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToPan123Number(t *testing.T) {
	if toPan123Number("0") != 0 || toPan123Number("pan123_root") != 0 || toPan123Number("/") != 0 {
		t.Fatal("root sentinels must map to 0")
	}
	if toPan123Number("42") != 42 {
		t.Fatal("numeric id must parse")
	}
	if toPan123Number("abc") != 0 {
		t.Fatal("non-numeric id must map to 0")
	}
}

func TestParentOf(t *testing.T) {
	if parentOf("") != RootID || parentOf("0") != RootID {
		t.Fatal("empty/0 parent must map to root sentinel")
	}
	if parentOf("5") != "5" {
		t.Fatal("numeric parent kept")
	}
}

// ---- normalization ----

func jnum(s string) json.Number { return json.Number(s) }

func TestNormalizePan123FilePascalCase(t *testing.T) {
	raw := map[string]any{
		"FileId":       jnum("12345678901234567"),
		"FileName":     "a.mp4",
		"Size":         jnum("1024"),
		"Type":         jnum("0"),
		"Etag":         "abc",
		"S3KeyFlag":    "S3F",
		"DownloadUrl":  "https://d.example/a",
		"UpdateAt":     "2026-01-02T03:04:05Z",
		"ParentFileId": jnum("9"),
	}
	f := normalizePan123File(raw)
	if f.FileID != "12345678901234567" {
		t.Errorf("FileID = %q", f.FileID)
	}
	if f.FileName != "a.mp4" || f.Size != 1024 || f.Type != 0 || f.Etag != "abc" || f.S3KeyFlag != "S3F" {
		t.Errorf("fields mismatch: %+v", f)
	}
	if f.ParentFileID != "9" {
		t.Errorf("ParentFileID = %q", f.ParentFileID)
	}
}

func TestNormalizePan123FileCamelCaseAndS3Fallback(t *testing.T) {
	raw := map[string]any{
		"fileId":    "7",
		"fileName":  "b.txt",
		"size":      jnum("2"),
		"type":      jnum("1"),
		"s3keyFlag": "S3B",
	}
	f := normalizePan123File(raw)
	if f.FileID != "7" || f.FileName != "b.txt" || f.Type != 1 || f.S3KeyFlag != "S3B" {
		t.Errorf("camelCase normalize failed: %+v", f)
	}
	// pickS3 regex fallback
	raw2 := map[string]any{"fileId": "8", "S3_key_flag": "X"}
	f2 := normalizePan123File(raw2)
	if f2.S3KeyFlag != "X" {
		t.Errorf("pickS3 regex fallback failed: %q", f2.S3KeyFlag)
	}
}

func TestNormalizePan123FileStringFileID(t *testing.T) {
	raw := map[string]any{"FileId": "999", "FileName": "c"}
	f := normalizePan123File(raw)
	if f.FileID != "999" {
		t.Errorf("string FileID = %q", f.FileID)
	}
}

func TestParseMapUseNumber(t *testing.T) {
	m := parseMap(json.RawMessage(`{"FileId": 12345678901234567, "nested": {"k": 1}}`))
	if got := asString(m["FileId"]); got != "12345678901234567" {
		t.Fatalf("big FileId lost precision: %q", got)
	}
}

// ---- pool merge ----

func TestMergeFileEmptyIncomingKeepsExisting(t *testing.T) {
	ex := pan123File{FileID: "1", FileName: "a", Size: 10, Type: 1, Etag: "e", S3KeyFlag: "S", DownloadURL: "u", UpdateAt: "t"}
	in := pan123File{FileID: "1", FileName: "", Size: 0, Type: 0, Etag: "", S3KeyFlag: "", DownloadURL: "", UpdateAt: ""}
	got := mergeFile(in, ex)
	want := pan123File{FileID: "1", FileName: "a", Size: 10, Type: 0, Etag: "e", S3KeyFlag: "S", DownloadURL: "u", UpdateAt: "t"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeFile = %+v, want %+v", got, want)
	}
}

func TestMergeFileIncomingWins(t *testing.T) {
	ex := pan123File{FileID: "1", FileName: "a", Size: 10, S3KeyFlag: "S"}
	in := pan123File{FileID: "1", FileName: "b", Size: 20, Etag: "e2", S3KeyFlag: "S2"}
	got := mergeFile(in, ex)
	if got.FileName != "b" || got.Size != 20 || got.S3KeyFlag != "S2" || got.Etag != "e2" {
		t.Fatalf("incoming should win: %+v", got)
	}
}

func TestPutPoolAndGet(t *testing.T) {
	putPool(pan123File{FileID: "55", FileName: "x", S3KeyFlag: "S"})
	f, ok := poolGet("55")
	if !ok || f.FileName != "x" || f.S3KeyFlag != "S" {
		t.Fatalf("pool get failed: %+v ok=%v", f, ok)
	}
	if _, ok2 := poolGet("pan123_root"); ok2 {
		t.Fatal("root id must not be pooled")
	}
}

// ---- description backup ----

func TestPan123MetaDescRoundTrip(t *testing.T) {
	f := pan123File{FileID: "3", FileName: "f.bin", Size: 99, Type: 0, Etag: "abcd", S3KeyFlag: "KEY"}
	desc := encodePan123MetaDesc(f)
	if !strings.HasPrefix(desc, "pan123meta:") {
		t.Fatalf("desc = %q", desc)
	}
	got, ok := decodePan123MetaDesc(desc)
	if !ok {
		t.Fatal("decode failed")
	}
	if !reflect.DeepEqual(got, f) {
		t.Fatalf("round trip = %+v, want %+v", got, f)
	}
	// empty backup
	if encodePan123MetaDesc(pan123File{FileID: "1"}) != "" {
		t.Fatal("empty meta must not produce backup")
	}
	// junk input
	if _, ok := decodePan123MetaDesc("pan123meta:!!!invalid"); ok {
		t.Fatal("invalid backup must not decode")
	}
}

// ---- etag / duplicate ----

func TestEtagAsMD5(t *testing.T) {
	if etagAsMd5("ABCDEF01234567890123456789abcdef") != "abcdef01234567890123456789abcdef" {
		t.Fatal("uppercase md5 must be lowercased")
	}
	if etagAsMd5("xyz") != "" || etagAsMd5(strings.Repeat("a", 31)) != "" || etagAsMd5(strings.Repeat("g", 32)) != "" {
		t.Fatal("invalid etags must be rejected")
	}
}

func TestDuplicateFromRequest(t *testing.T) {
	if duplicateFromRequest(2) != 2 || duplicateFromRequest(0) != 1 || duplicateFromRequest(1) != 1 {
		t.Fatal("duplicate mapping must be 2→2 else 1")
	}
}

// ---- expiration ----

func TestFormatPan123Time(t *testing.T) {
	ts := time.Date(2026, 1, 2, 15, 4, 5, 0, time.Local)
	if got := formatPan123Time(ts); got != "2026-01-02 15:04:05" {
		t.Fatalf("formatPan123Time = %q", got)
	}
}

func TestFormatPan123Expiration(t *testing.T) {
	if formatPan123Expiration("") != "" {
		t.Fatal("empty expiration must stay empty")
	}
	if formatPan123Expiration("not-a-date") != "" {
		t.Fatal("invalid expiration must be empty")
	}
	// no-zone value parses as local, formats back identically
	if got := formatPan123Expiration("2026-03-04 05:06:07"); got != "2026-03-04 05:06:07" {
		t.Fatalf("round trip = %q", got)
	}
}

func TestParseFlexibleTime(t *testing.T) {
	if got := parseFlexibleTime("2026-01-02T15:04:05Z"); got.IsZero() || got.Unix() != time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC).Unix() {
		t.Fatalf("RFC3339 parse failed: %v", got)
	}
	if got := parseFlexibleTime("bogus"); !got.IsZero() {
		t.Fatalf("invalid must be zero: %v", got)
	}
}

func TestGetExpiresTimeAMZ(t *testing.T) {
	base := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC).UnixMilli()
	url := "https://cdn.example.com/f?X-Amz-Date=20260102T150405Z&X-Amz-Expires=3600"
	want := base + 3600*1000
	if got := getExpiresTime(url); got != want {
		t.Fatalf("getExpiresTime = %d, want %d", got, want)
	}
}

func TestGetExpiresTimeNumeric(t *testing.T) {
	// second-level timestamp < 1e10 → ms
	if got := getExpiresTime("https://x/f?expire=1767225600"); got != 1767225600*1000 {
		t.Fatalf("seconds timestamp = %d", got)
	}
	// ms-level timestamp >= 1e10 → raw
	if got := getExpiresTime("https://x/f?expires=2000000000000"); got != 2000000000000 {
		t.Fatalf("ms timestamp = %d", got)
	}
	// small numbers (business params) skipped
	if got := getExpiresTime("https://x/f?e=1"); got != 0 {
		t.Fatalf("small value must be skipped, got %d", got)
	}
	// date string parse
	want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local).UnixMilli()
	if got := getExpiresTime("https://x/f?expire=2026-01-02"); got != want {
		t.Fatalf("date string = %d, want %d", got, want)
	}
}

func TestGetExpiresTimeNoParams(t *testing.T) {
	if got := getExpiresTime("https://x/f"); got != 0 {
		t.Fatalf("no params must yield 0, got %d", got)
	}
	if got := getExpiresTime(""); got != 0 {
		t.Fatalf("empty url must yield 0, got %d", got)
	}
}

// ---- redirect extraction ----

func TestExtractPan123RedirectURL(t *testing.T) {
	// JSON redirect_url
	got := extractPan123RedirectURL(`{"data":{"redirect_url":"https://cdn.example.com/a"}}`, "https://yun.123pan.com/x")
	if got != "https://cdn.example.com/a" {
		t.Fatalf("json redirect = %q", got)
	}
	// JSON redirectUrl
	got = extractPan123RedirectURL(`{"data":{"redirectUrl":"https://cdn.example.com/b"}}`, "https://yun.123pan.com/x")
	if got != "https://cdn.example.com/b" {
		t.Fatalf("json redirectUrl = %q", got)
	}
	// HTML href fallback
	got = extractPan123RedirectURL(`<html><a href="https://cdn.example.com/c">go</a></html>`, "https://yun.123pan.com/x")
	if got != "https://cdn.example.com/c" {
		t.Fatalf("href redirect = %q", got)
	}
	// invalid scheme rejected
	if got := extractPan123RedirectURL(`{"data":{"redirect_url":"ftp://x/y"}}`, "https://yun.123pan.com/x"); got != "" {
		t.Fatalf("ftp scheme must be rejected, got %q", got)
	}
	// relative href (no scheme) rejected by regex
	if got := extractPan123RedirectURL(`<a href="/rel">x</a>`, "https://yun.123pan.com/x"); got != "" {
		t.Fatalf("relative href must be rejected, got %q", got)
	}
	// empty
	if got := extractPan123RedirectURL("", "https://yun.123pan.com/x"); got != "" {
		t.Fatalf("empty body must yield empty, got %q", got)
	}
}

// ---- login body ----

func TestPan123LoginBody(t *testing.T) {
	email := pan123LoginBody("a@b.com", "p")
	if email["mail"] != "a@b.com" || email["password"] != "p" || email["type"] != 2 {
		t.Fatalf("email body = %+v", email)
	}
	phone := pan123LoginBody("13800000000", "p")
	if phone["passport"] != "13800000000" || phone["remember"] != true {
		t.Fatalf("phone body = %+v", phone)
	}
}

// ---- chunk plan ----

func TestCalcPan123ChunkPlan(t *testing.T) {
	const mb = int64(1024 * 1024)
	cases := []struct {
		size       int64
		chunkSize  int64
		chunkCount int64
	}{
		{0, 1, 1},
		{100, 100, 1},
		{16 * mb, 16 * mb, 1},
		{16*mb + 1, 16 * mb, 2},
		{32 * mb, 16 * mb, 2},
		{33 * mb, 16 * mb, 3},
		{48*mb + 1, 16 * mb, 4},
	}
	for _, c := range cases {
		plan := calcPan123ChunkPlan(c.size)
		if plan.ChunkSize != c.chunkSize || plan.ChunkCount != c.chunkCount {
			t.Errorf("size=%d plan=%+v, want chunkSize=%d chunkCount=%d", c.size, plan, c.chunkSize, c.chunkCount)
		}
	}
}

// ---- mapping ----

func TestMapFile(t *testing.T) {
	// folder entry
	folder := pan123File{FileID: "1", FileName: "dir", Type: 1, Size: 0, UpdateAt: "2026-01-02T03:04:05Z"}
	f := mapFile(folder, "pan123:acc", "0")
	if !f.IsDir || f.Category != "folder" || f.Icon != "iconfile-folder" {
		t.Fatalf("folder mapping wrong: %+v", f)
	}
	if f.ParentFileID != RootID {
		t.Fatalf("parent must map to root sentinel, got %q", f.ParentFileID)
	}
	if f.SizeStr == "" || f.TimeStr == "" {
		t.Fatalf("size/time strings missing: %+v", f)
	}
	// file entry
	file := pan123File{FileID: "2", FileName: "v.mp4", Type: 0, Size: 1024, UpdateAt: "2026-01-02T03:04:05Z", S3KeyFlag: "S", Etag: "abc"}
	f2 := mapFile(file, "pan123:acc", "5")
	if f2.IsDir || f2.Category != "video" || f2.Ext != "mp4" {
		t.Fatalf("file mapping wrong: %+v", f2)
	}
	if f2.ParentFileID != "5" {
		t.Fatalf("numeric parent kept, got %q", f2.ParentFileID)
	}
	if f2.Description == "" {
		t.Fatal("description backup missing")
	}
}

// ---- upload request data ----

func TestParseUploadRequestData(t *testing.T) {
	raw := map[string]any{
		"Bucket": "b", "Key": "k", "FileId": jnum("7"), "Reuse": true, "StorageNode": "n", "UploadId": "u",
	}
	d := parseUploadRequestData(raw)
	if d.Bucket != "b" || d.Key != "k" || d.FileID != "7" || !d.Reuse || d.StorageNode != "n" || d.UploadID != "u" {
		t.Fatalf("parseUploadRequestData = %+v", d)
	}
}

// ---- file list paging data ----

func TestRawList(t *testing.T) {
	items := []any{map[string]any{"FileId": jnum("1")}}
	if got := rawList(map[string]any{"InfoList": items}); len(got) != 1 {
		t.Fatalf("InfoList extraction failed: %v", got)
	}
	if got := rawList(map[string]any{"infoList": items}); len(got) != 1 {
		t.Fatalf("infoList extraction failed: %v", got)
	}
	if got := rawList(map[string]any{}); got != nil {
		t.Fatalf("missing list must be nil, got %v", got)
	}
	// next sentinel detection uses data.Next in the paging loop; verify pick
	if asString(pick(map[string]any{"Next": "-1"}, "Next", "next")) != "-1" {
		t.Fatal("Next extraction failed")
	}
}
