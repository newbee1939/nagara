package video

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	v, err := New("  5分で理解するSRE  ", "  SLO と SLI の話  ", "video/mp4", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "5分で理解するSRE" || v.Description != "SLO と SLI の話" {
		t.Errorf("前後の空白が除かれていない: %q, %q", v.Title, v.Description)
	}
	if v.MediaType != "video/mp4" || v.Duration != 5*time.Minute {
		t.Errorf("MediaType, Duration = %q, %v", v.MediaType, v.Duration)
	}
	if !IsID(v.ID) {
		t.Errorf("ID の形式が不正: %q", v.ID)
	}
	if v.CreatedAt.IsZero() {
		t.Error("CreatedAt が設定されていない")
	}
}

func TestNewIssuesUniqueIDs(t *testing.T) {
	a, _ := New("a", "", "video/webm", 0)
	b, _ := New("b", "", "video/webm", 0)
	if a.ID == b.ID {
		t.Errorf("ID が重複した: %q", a.ID)
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name                          string
		title, description, mediaType string
		field                         string
	}{
		{"タイトルが空", "   ", "", "video/mp4", "title"},
		{"タイトルが長すぎる", strings.Repeat("あ", 101), "", "video/mp4", "title"},
		{"説明が長すぎる", "t", strings.Repeat("あ", 5001), "video/mp4", "description"},
		{"再生できない形式", "t", "", "video/quicktime", "media"},
		{"ファイルが無い", "t", "", "", "media"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.title, tt.description, tt.mediaType, 0)
			var invalid Invalid
			if !errors.As(err, &invalid) {
				t.Fatalf("Invalid が返らない: %v", err)
			}
			if _, ok := invalid[tt.field]; !ok || len(invalid) != 1 {
				t.Errorf("誤りの項目 = %v, want %s のみ", invalid, tt.field)
			}
		})
	}
}

func TestNewAcceptsLimits(t *testing.T) {
	if _, err := New(strings.Repeat("あ", 100), strings.Repeat("あ", 5000), "video/mp4", 0); err != nil {
		t.Errorf("上限ちょうどは受け付けるべき: %v", err)
	}
}

func TestNewReportsAllErrors(t *testing.T) {
	_, err := New("", "", "", 0)
	var invalid Invalid
	if !errors.As(err, &invalid) || len(invalid) != 2 {
		t.Fatalf("title と media の両方を返すべき: %v", invalid)
	}
	if got := err.Error(); got != "video: invalid media, title" {
		t.Errorf("Error() = %q", got)
	}
}

func TestMatches(t *testing.T) {
	v := Video{Title: "5分で理解するSRE", Description: "SLO と Error Budget"}
	tests := []struct {
		query string
		want  bool
	}{
		{"", true},
		{"sre", true},
		{"error budget", true},
		{"SRE  slo", true},
		{"SRE kubernetes", false},
	}
	for _, tt := range tests {
		if got := v.Matches(tt.query); got != tt.want {
			t.Errorf("Matches(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestIsID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"0123456789ab", true},
		{"0123456789AB", false},
		{"0123456789a", false},
		{"../../etc/pa", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsID(tt.id); got != tt.want {
			t.Errorf("IsID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}
