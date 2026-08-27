package tools

import "testing"

// TestParseDDGLite guards the v1.0.1 regex hoist: the lite-results parser
// must keep extracting result-link anchors after reLite moved to a package
// variable (it used to be recompiled inside parseDDG on every call).
func TestParseDDGLite(t *testing.T) {
	body := `<html><body>
<a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fa&amp;rut=abc">Example A</a>
<a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fb&amp;rut=def">Example B</a>
</body></html>`
	got := parseDDG(body)
	if len(got) != 2 {
		t.Fatalf("parsed %d results, want 2", len(got))
	}
	if got[0].href != "https://example.com/a" {
		t.Errorf("result[0].href = %q, want the unwrapped real URL", got[0].href)
	}
	if got[0].title != "Example A" {
		t.Errorf("result[0].title = %q", got[0].title)
	}
}

// TestParseDDGHTML guards the primary DuckDuckGo html parser.
func TestParseDDGHTML(t *testing.T) {
	body := `<a class="result__a" href="https://example.com/x">Result X</a>
<a class="result__snippet" href="https://example.com/x">A snippet &amp; more</a>`
	got := parseDDG(body)
	if len(got) != 1 {
		t.Fatalf("parsed %d results, want 1", len(got))
	}
	if got[0].title != "Result X" || got[0].snippet != "A snippet & more" {
		t.Errorf("title/snippet wrong: %+v", got[0])
	}
}

// TestBingRedirectDecode guards the Bing base64-redirect unwrapper.
func TestBingRedirectDecode(t *testing.T) {
	// a1 + base64url("https://example.com/real") without padding
	got := bingRealURL("https://www.bing.com/ck/a?!&&p=x&u=a1aHR0cHM6Ly9leGFtcGxlLmNvbS9yZWFs")
	if got != "https://example.com/real" {
		t.Errorf("bingRealURL = %q, want https://example.com/real", got)
	}
}
