// Package web は nagara の画面と HTTP の入り口。
package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/newbee1939/nagara/internal/video"
)

// Library は web が必要とする動画の保存先。実装（filestore など）は外から渡す。
type Library interface {
	Save(v video.Video, media io.Reader) error
	Find(id string) (video.Video, error)
	All() ([]video.Video, error)
	Media(id string) (io.ReadSeekCloser, error)
}

//go:embed templates static
var assets embed.FS

type server struct {
	lib       Library
	maxUpload int64
	pages     map[string]*template.Template
}

// New は nagara の http.Handler を返す。maxUpload はアップロードできる最大バイト数。
func New(lib Library, maxUpload int64) http.Handler {
	s := &server{lib: lib, maxUpload: maxUpload, pages: parsePages()}
	static, _ := fs.Sub(assets, "static")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /upload", s.uploadForm)
	mux.HandleFunc("POST /videos", s.upload)
	mux.HandleFunc("GET /videos/{id}", s.watch)
	mux.HandleFunc("GET /media/{id}", s.media)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("/", s.notFound)
	return mux
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	all, err := s.lib.All()
	if err != nil {
		s.fail(w, err)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	videos := make([]video.Video, 0, len(all))
	for _, v := range all {
		if v.Matches(query) {
			videos = append(videos, v)
		}
	}
	s.render(w, http.StatusOK, "index.html", map[string]any{"Query": query, "Videos": videos})
}

// uploadPage はアップロード画面に渡す値。エラー時は入力値を残して再表示する。
type uploadPage struct {
	Title, Description string
	Errors             video.Invalid
}

func (s *server) uploadForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "upload.html", uploadPage{})
}

func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			page := uploadPage{Errors: video.Invalid{"media": "ファイルが大きすぎます"}}
			s.render(w, http.StatusRequestEntityTooLarge, "upload.html", page)
			return
		}
		s.render(w, http.StatusBadRequest, "error.html", "フォームを読み取れませんでした")
		return
	}
	defer r.MultipartForm.RemoveAll()

	page := uploadPage{Title: r.FormValue("title"), Description: r.FormValue("description")}

	file, _, err := r.FormFile("media")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		s.fail(w, err)
		return
	}
	mediaType := "" // ファイルが無ければ video.New が「動画を選んでください」と返す
	if file != nil {
		defer file.Close()
		if mediaType, err = sniff(file); err != nil {
			s.fail(w, err)
			return
		}
	}

	v, err := video.New(page.Title, page.Description, mediaType, parseSeconds(r.FormValue("duration")))
	if errors.As(err, &page.Errors) {
		s.render(w, http.StatusUnprocessableEntity, "upload.html", page)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.lib.Save(v, file); err != nil {
		s.fail(w, err)
		return
	}
	http.Redirect(w, r, "/videos/"+v.ID, http.StatusSeeOther)
}

func (s *server) watch(w http.ResponseWriter, r *http.Request) {
	v, err := s.lib.Find(r.PathValue("id"))
	if errors.Is(err, video.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.render(w, http.StatusOK, "watch.html", v)
}

// media は動画本体を返す。http.ServeContent が Range リクエストに応えるので、シークもできる。
func (s *server) media(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, err := s.lib.Find(id)
	if errors.Is(err, video.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	f, err := s.lib.Media(id)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", v.MediaType)
	http.ServeContent(w, r, "", v.CreatedAt, f)
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, "error.html", "ページが見つかりません")
}

func (s *server) fail(w http.ResponseWriter, err error) {
	log.Printf("nagara: %v", err)
	s.render(w, http.StatusInternalServerError, "error.html", "問題が発生しました。時間をおいて試してください")
}

// render はテンプレートを一度バッファに書き出してから返す。途中で失敗しても壊れた HTML を送らない。
func (s *server) render(w http.ResponseWriter, status int, page string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("nagara: render %s: %v", page, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

func parsePages() map[string]*template.Template {
	funcs := template.FuncMap{"clock": clock, "date": date}
	pages := map[string]*template.Template{}
	for _, name := range []string{"index.html", "upload.html", "watch.html", "error.html"} {
		pages[name] = template.Must(template.New(name).Funcs(funcs).ParseFS(assets, "templates/layout.html", "templates/"+name))
	}
	return pages
}

// sniff はファイルの先頭を見て形式を判定し、読み取り位置を先頭に戻す。
// 拡張子やブラウザの申告は信用しない。
func sniff(f io.ReadSeeker) (string, error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

// parseSeconds はブラウザが読み取った再生時間（秒）を受け取る。読めなければ 0（不明）。
func parseSeconds(s string) time.Duration {
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil || !(sec > 0 && sec < 1e6) { // NaN・Inf・負の値・桁あふれを弾く
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

// clock は再生時間を 5:07 や 1:02:03 の形にする。不明（0）なら空文字。
func clock(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	t := int(d.Round(time.Second).Seconds())
	h, m, sec := t/3600, t/60%60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func date(t time.Time) string {
	return t.Local().Format("2006.01.02")
}
