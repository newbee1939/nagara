package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/newbee1939/nagara/internal/filestore"
	"github.com/newbee1939/nagara/internal/video"
)

// mp4 の先頭（ftyp ボックス）。http.DetectContentType が video/mp4 と判定する最小の形。
var mp4 = append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 64)...)

func newServer(t *testing.T) (http.Handler, *filestore.Store) {
	t.Helper()
	store, err := filestore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(store, 1<<20), store
}

func save(t *testing.T, store *filestore.Store, title, description string) video.Video {
	t.Helper()
	v, err := video.New(title, description, "video/mp4", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(v, bytes.NewReader(mp4)); err != nil {
		t.Fatal(err)
	}
	return v
}

func get(h http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func uploadRequest(t *testing.T, fields map[string]string, media []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		w.WriteField(k, v)
	}
	if media != nil {
		part, _ := w.CreateFormFile("media", "talk.mp4")
		part.Write(media)
	}
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/videos", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestIndex(t *testing.T) {
	h, store := newServer(t)
	save(t, store, "5分で理解するSRE", "SLO の話")
	save(t, store, "Go のエラー処理", "")

	rec := get(h, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, want := range []string{"5分で理解するSRE", "Go のエラー処理"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("一覧に %q が無い", want)
		}
	}

	rec = get(h, "/?q=slo")
	if body := rec.Body.String(); !strings.Contains(body, "5分で理解するSRE") || strings.Contains(body, "Go のエラー処理") {
		t.Error("検索で絞り込まれていない")
	}
}

func TestIndexEmpty(t *testing.T) {
	h, _ := newServer(t)
	if body := get(h, "/").Body.String(); !strings.Contains(body, "まだ動画がありません") {
		t.Error("空の案内が出ていない")
	}
	if body := get(h, "/?q=rust").Body.String(); !strings.Contains(body, "「rust」に合う動画はありませんでした") {
		t.Error("検索結果なしの案内が出ていない")
	}
}

func TestUploadForm(t *testing.T) {
	h, _ := newServer(t)
	if rec := get(h, "/upload"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `name="media"`) {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestUploadThenWatch(t *testing.T) {
	h, store := newServer(t)
	req := uploadRequest(t, map[string]string{"title": "5分で理解するSRE", "description": "SLO の話", "duration": "312.4"}, mp4)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	videos, _ := store.All()
	if len(videos) != 1 {
		t.Fatalf("保存された動画 = %d 本", len(videos))
	}
	v := videos[0]
	if v.MediaType != "video/mp4" || v.Duration.Round(time.Second) != 312*time.Second {
		t.Errorf("保存内容 = %+v", v)
	}
	if loc := rec.Header().Get("Location"); loc != "/videos/"+v.ID {
		t.Errorf("Location = %q", loc)
	}

	watch := get(h, "/videos/"+v.ID)
	if watch.Code != http.StatusOK {
		t.Fatalf("status = %d", watch.Code)
	}
	for _, want := range []string{"5分で理解するSRE", "SLO の話", "5:12", `src="/media/` + v.ID + `"`} {
		if !strings.Contains(watch.Body.String(), want) {
			t.Errorf("視聴画面に %q が無い", want)
		}
	}
}

func TestUploadRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		title string
		media []byte
		want  string
	}{
		{"タイトルなし", "", mp4, "タイトルを入力してください"},
		{"ファイルなし", "SRE", nil, "MP4 か WebM の動画を選んでください"},
		{"動画ではない", "SRE", []byte("#!/bin/sh\necho hi"), "MP4 か WebM の動画を選んでください"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, store := newServer(t)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, uploadRequest(t, map[string]string{"title": tt.title, "description": "残る説明"}, tt.media))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("エラー文 %q が無い", tt.want)
			}
			if !strings.Contains(body, "残る説明") {
				t.Error("入力値が残っていない")
			}
			if videos, _ := store.All(); len(videos) != 0 {
				t.Error("保存されてしまった")
			}
		})
	}
}

func TestUploadRejectsTooLarge(t *testing.T) {
	h, _ := newServer(t)
	big := append(append([]byte{}, mp4...), make([]byte, 2<<20)...)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, uploadRequest(t, map[string]string{"title": "大きい"}, big))

	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "ファイルが大きすぎます") {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestMediaSupportsRange(t *testing.T) {
	h, store := newServer(t)
	v := save(t, store, "SRE", "")

	req := httptest.NewRequest(http.MethodGet, "/media/"+v.ID, nil)
	req.Header.Set("Range", "bytes=4-7")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body, _ := io.ReadAll(rec.Body); string(body) != "ftyp" {
		t.Errorf("body = %q", body)
	}
}

func TestNotFound(t *testing.T) {
	h, _ := newServer(t)
	for _, target := range []string{"/videos/0123456789ab", "/videos/..%2f..%2fetc", "/media/0123456789ab", "/nope"} {
		if rec := get(h, target); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}

func TestStatic(t *testing.T) {
	h, _ := newServer(t)
	for _, target := range []string{"/static/style.css", "/static/player.js", "/static/upload.js"} {
		if rec := get(h, target); rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d", target, rec.Code)
		}
	}
}

func TestParseSeconds(t *testing.T) {
	tests := map[string]time.Duration{
		"312.5": 312500 * time.Millisecond,
		"":      0,
		"abc":   0,
		"-3":    0,
		"NaN":   0,
		"Inf":   0,
		"1e300": 0,
	}
	for in, want := range tests {
		if got := parseSeconds(in); got != want {
			t.Errorf("parseSeconds(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestClock(t *testing.T) {
	tests := map[time.Duration]string{
		0:                                     "",
		59*time.Second + 600*time.Millisecond: "1:00",
		5*time.Minute + 7*time.Second:         "5:07",
		time.Hour + 2*time.Minute + 3*time.Second: "1:02:03",
	}
	for in, want := range tests {
		if got := clock(in); got != want {
			t.Errorf("clock(%v) = %q, want %q", in, got, want)
		}
	}
}
