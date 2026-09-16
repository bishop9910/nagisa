package data

import (
	"strings"
	"testing"
	"time"
)

func TestApplySQLitePragmas(t *testing.T) {
	opts := DataOptions{WAL: true, ForeignKeys: true, BusyTimeout: 5 * time.Second}
	got := applySQLitePragmas("file:nagisa.db", opts)
	for _, want := range []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=foreign_keys(1)",
		"_pragma=synchronous(NORMAL)",
		"_txlock=immediate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dsn %q is missing %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "file:nagisa.db?") {
		t.Errorf("dsn %q did not keep the file prefix", got)
	}

	// An operator supplied value must win over the default.
	custom := applySQLitePragmas("file:nagisa.db?_pragma=busy_timeout(1000)&_txlock=deferred", opts)
	if strings.Count(custom, "busy_timeout") != 1 {
		t.Errorf("dsn %q duplicated busy_timeout", custom)
	}
	if strings.Contains(custom, "busy_timeout(5000)") {
		t.Errorf("dsn %q overrode the configured timeout", custom)
	}
	if strings.Count(custom, "_txlock") != 1 {
		t.Errorf("dsn %q duplicated the transaction lock mode", custom)
	}

	// A disabled WAL must not appear, and an existing query string must be
	// extended rather than replaced.
	noWAL := applySQLitePragmas("file:nagisa.db?cache=shared", DataOptions{ForeignKeys: true, BusyTimeout: time.Second})
	if strings.Contains(noWAL, "journal_mode") {
		t.Errorf("dsn %q enabled WAL while it was disabled", noWAL)
	}
	if !strings.Contains(noWAL, "cache=shared&") {
		t.Errorf("dsn %q dropped the existing parameters", noWAL)
	}
}

func TestNormalizedDriver(t *testing.T) {
	for _, name := range []string{"sqlite", "sqlite3", "SQLite", "modernc", "modernc.org/sqlite"} {
		if got := normalizedDriver(name); got != "sqlite" {
			t.Errorf("normalizedDriver(%q) = %q", name, got)
		}
	}
	for _, name := range []string{"mysql", "postgres"} {
		if got := normalizedDriver(name); got != name {
			t.Errorf("normalizedDriver(%q) = %q", name, got)
		}
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike("100%_done"); got != `100\%\_done` {
		t.Errorf("escapeLike = %q", got)
	}
	if got := escapeLike(`back\slash`); got != `back\\slash` {
		t.Errorf("escapeLike = %q", got)
	}
}

func TestContentDisposition(t *testing.T) {
	got := contentDisposition("attachment", "报告 2026.pdf")
	if !strings.HasPrefix(got, "attachment; ") {
		t.Errorf("content disposition = %q", got)
	}
	if !strings.Contains(got, `filename="`) {
		t.Errorf("content disposition has no ascii fallback: %q", got)
	}
	if !strings.Contains(got, "filename*=UTF-8''") {
		t.Errorf("content disposition has no extended form: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Errorf("content disposition contains a newline: %q", got)
	}
	// A quote in the name must not be able to break out of the header: the
	// fallback replaces it and the extended form percent encodes it, so the
	// only quotes left are the two delimiters of the fallback value.
	quoted := contentDisposition("inline", `a"b.txt`)
	if strings.Count(quoted, `"`) != 2 {
		t.Errorf("a quote in the file name was not neutralised: %q", quoted)
	}
	if !strings.Contains(quoted, "a%22b.txt") {
		t.Errorf("the extended form did not encode the quote: %q", quoted)
	}
	empty := contentDisposition("", "x.txt")
	if !strings.HasPrefix(empty, "attachment; ") {
		t.Errorf("an empty disposition did not default to attachment: %q", empty)
	}
}

func TestSplitNameAndMimeFamily(t *testing.T) {
	cases := []struct{ in, base, ext string }{
		{"report.pdf", "report", ".pdf"},
		{"archive.tar.gz", "archive.tar", ".gz"},
		{"no-extension", "no-extension", ""},
		{".hidden", ".hidden", ""},
		{"a.verylongextensionthatisnotreal", "a.verylongextensionthatisnotreal", ""},
	}
	for _, tc := range cases {
		base, ext := splitName(tc.in)
		if base != tc.base || ext != tc.ext {
			t.Errorf("splitName(%q) = (%q, %q), want (%q, %q)", tc.in, base, ext, tc.base, tc.ext)
		}
	}

	families := map[string]string{
		"image/png":               "image",
		"video/mp4":               "video",
		"audio/mpeg":              "audio",
		"text/plain":              "text",
		"application/pdf":         "application",
		"font/woff2":              "font",
		"":                        "other",
		"something-without-slash": "other",
		"x-custom/whatever":       "other",
	}
	for mimeType, want := range families {
		if got := mimeFamily(mimeType); got != want {
			t.Errorf("mimeFamily(%q) = %q, want %q", mimeType, got, want)
		}
	}
}
