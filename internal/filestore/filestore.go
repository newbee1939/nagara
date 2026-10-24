// Package filestore は動画をローカルのディレクトリに保存する。
//
//	<dir>/<id>/meta.json  動画の情報
//	<dir>/<id>/media      動画ファイル本体
package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/newbee1939/nagara/internal/video"
)

const (
	metaFile  = "meta.json"
	mediaFile = "media"
	tmpPrefix = ".tmp-"
)

// Store はディレクトリを使った動画の保存先。
type Store struct {
	dir string
}

// Open は dir を保存先にした Store を返す。dir が無ければ作る。
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("filestore: %w", err)
	}
	return &Store{dir: dir}, nil
}

// meta.json の形。ドメインの型に保存形式の都合（json タグ）を持ち込まないために分ける。
type record struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	MediaType   string        `json:"mediaType"`
	Duration    time.Duration `json:"duration"`
	CreatedAt   time.Time     `json:"createdAt"`
}

// Save は動画の情報と本体を保存する。
// 一時ディレクトリに書き切ってから rename するので、途中で失敗しても中途半端な動画は残らない。
func (s *Store) Save(v video.Video, media io.Reader) error {
	tmp, err := os.MkdirTemp(s.dir, tmpPrefix)
	if err != nil {
		return fmt.Errorf("filestore: %w", err)
	}
	if err := writeVideo(tmp, v, media); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("filestore: save %s: %w", v.ID, err)
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, v.ID)); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("filestore: save %s: %w", v.ID, err)
	}
	return nil
}

func writeVideo(dir string, v video.Video, media io.Reader) error {
	meta, err := json.MarshalIndent(record(v), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, metaFile), meta, 0o644); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, mediaFile))
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, media); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Find は id の動画を返す。無ければ video.ErrNotFound を返す。
func (s *Store) Find(id string) (video.Video, error) {
	if !video.IsID(id) {
		return video.Video{}, video.ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.dir, id, metaFile))
	if errors.Is(err, fs.ErrNotExist) {
		return video.Video{}, video.ErrNotFound
	}
	if err != nil {
		return video.Video{}, fmt.Errorf("filestore: %w", err)
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return video.Video{}, fmt.Errorf("filestore: %s: %w", id, err)
	}
	return video.Video(r), nil
}

// All はすべての動画を新しい順に返す。
func (s *Store) All() ([]video.Video, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("filestore: %w", err)
	}
	videos := make([]video.Video, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := s.Find(e.Name())
		if errors.Is(err, video.ErrNotFound) {
			continue // 保存途中の一時ディレクトリなど、動画ではないもの
		}
		if err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	slices.SortFunc(videos, func(a, b video.Video) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return videos, nil
}

// Media は id の動画ファイルを開く。無ければ video.ErrNotFound を返す。
func (s *Store) Media(id string) (io.ReadSeekCloser, error) {
	if !video.IsID(id) {
		return nil, video.ErrNotFound
	}
	f, err := os.Open(filepath.Join(s.dir, id, mediaFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, video.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("filestore: %w", err)
	}
	return f, nil
}
