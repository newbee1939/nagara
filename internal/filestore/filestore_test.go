package filestore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/newbee1939/nagara/internal/video"
)

func newVideo(t *testing.T, title string, createdAt time.Time) video.Video {
	t.Helper()
	v, err := video.New(title, "説明", "video/mp4", 90*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	v.CreatedAt = createdAt
	return v
}

func TestSaveAndFind(t *testing.T) {
	s, _ := Open(t.TempDir())
	want := newVideo(t, "SRE", time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))

	if err := s.Save(want, strings.NewReader("movie")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Find() = %+v, want %+v", got, want)
	}

	f, err := s.Media(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if body, _ := io.ReadAll(f); string(body) != "movie" {
		t.Errorf("Media() = %q", body)
	}
}

func TestAllReturnsNewestFirst(t *testing.T) {
	s, _ := Open(t.TempDir())
	base := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	for _, e := range []struct {
		title string
		after time.Duration
	}{{"古い", 0}, {"新しい", 2 * time.Hour}, {"中間", time.Hour}} {
		if err := s.Save(newVideo(t, e.title, base.Add(e.after)), strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
	}

	videos, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, v := range videos {
		titles = append(titles, v.Title)
	}
	if got := strings.Join(titles, ","); got != "新しい,中間,古い" {
		t.Errorf("All() の順序 = %s", got)
	}
}

func TestAllIgnoresNonVideoEntries(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	os.Mkdir(filepath.Join(dir, ".tmp-123"), 0o755)
	os.WriteFile(filepath.Join(dir, ".DS_Store"), nil, 0o644)

	videos, err := s.All()
	if err != nil || len(videos) != 0 {
		t.Errorf("All() = %v, %v; want 空", videos, err)
	}
}

func TestFindAndMediaReturnNotFound(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, id := range []string{"0123456789ab", "../secret", ""} {
		if _, err := s.Find(id); !errors.Is(err, video.ErrNotFound) {
			t.Errorf("Find(%q) = %v, want ErrNotFound", id, err)
		}
		if _, err := s.Media(id); !errors.Is(err, video.ErrNotFound) {
			t.Errorf("Media(%q) = %v, want ErrNotFound", id, err)
		}
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("切断") }

func TestSaveLeavesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	v := newVideo(t, "途中で失敗", time.Now())

	if err := s.Save(v, failingReader{}); err == nil {
		t.Fatal("エラーになるべき")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("ゴミが残っている: %v", entries)
	}
}

func TestOpenCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("ディレクトリが作られていない: %v", err)
	}
}
