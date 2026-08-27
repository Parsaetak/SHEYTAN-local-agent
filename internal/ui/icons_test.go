//go:build headless

// v1.0.3 tests: the attachment-tile icon mapping (file family → glyph).
package ui

import "testing"

func TestIconForFile(t *testing.T) {
	cases := map[string]string{
		"photo.PNG":           "image",
		"holiday.jpg":         "image",
		"icon.svg":            "image",
		"song.mp3":            "audio",
		"voice.wav":           "audio",
		"clip.mp4":            "video",
		"movie.mkv":           "video",
		"bundle.zip":          "archive",
		"backup.tar.gz":       "archive",
		"model.gguf":          "gguf",
		"main.go":             "code",
		"script.py":           "code",
		"notes.txt":           "doc",
		"README.md":           "doc",
		"data.csv":            "doc",
		"report.pdf":          "doc",
		"doc.docx":            "doc",
		"weird.xyzunknown":    "files",
		"no-extension":        "files",
		"C:\\path\\to\\a.png": "image",
		"/unix/path/to/b.mp3": "audio",
	}
	for name, want := range cases {
		if got := iconForFile(name); got != want {
			t.Errorf("iconForFile(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFilepathExt(t *testing.T) {
	cases := map[string]string{
		"a/b/c.txt":  ".txt",
		"a\\b\\c.md": ".md",
		"noext":      "",
		".hidden":    ".hidden",
		"tar.gz":     ".gz",
	}
	for name, want := range cases {
		if got := filepathExt(name); got != want {
			t.Errorf("filepathExt(%q) = %q, want %q", name, got, want)
		}
	}
}
