// Package video は nagara の中心にある「技術動画」を表す。
// 保存方法や HTTP には依存しない。
package video

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Video は投稿された 1 本の技術動画。
type Video struct {
	ID          string
	Title       string
	Description string
	MediaType   string
	Duration    time.Duration // 不明なら 0
	CreatedAt   time.Time
}

const (
	maxTitleLen       = 100
	maxDescriptionLen = 5000
)

// ErrNotFound は指定した動画が存在しないことを表す。
var ErrNotFound = errors.New("video: not found")

// Invalid は入力の誤りを、項目名（title / description / media）ごとにまとめたもの。
type Invalid map[string]string

func (e Invalid) Error() string {
	fields := make([]string, 0, len(e))
	for f := range e {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	return "video: invalid " + strings.Join(fields, ", ")
}

// ブラウザがそのまま再生できる形式だけを受け付ける。
var playable = map[string]bool{"video/mp4": true, "video/webm": true}

// New は入力を検証し、新しい動画を作る。
func New(title, description, mediaType string, duration time.Duration) (Video, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	invalid := Invalid{}
	switch {
	case title == "":
		invalid["title"] = "タイトルを入力してください"
	case utf8.RuneCountInString(title) > maxTitleLen:
		invalid["title"] = "タイトルは100文字以内にしてください"
	}
	if utf8.RuneCountInString(description) > maxDescriptionLen {
		invalid["description"] = "説明は5000文字以内にしてください"
	}
	if !playable[mediaType] {
		invalid["media"] = "MP4 か WebM の動画を選んでください"
	}
	if len(invalid) > 0 {
		return Video{}, invalid
	}

	return Video{
		ID:          newID(),
		Title:       title,
		Description: description,
		MediaType:   mediaType,
		Duration:    duration,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// Matches は、検索語をすべてタイトルか説明に含むかを返す（大文字小文字は区別しない）。
func (v Video) Matches(query string) bool {
	text := strings.ToLower(v.Title + "\n" + v.Description)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

// IsID は id が New の発行する形式かを返す。外部から来た id をパスに使う前の確認用。
func IsID(id string) bool {
	if len(id) != idLen*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && strings.ToLower(id) == id
}

const idLen = 6

func newID() string {
	b := make([]byte, idLen)
	rand.Read(b) // crypto/rand.Read は失敗しない（Go 1.24+）
	return hex.EncodeToString(b)
}
