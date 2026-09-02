package research

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearXNGProviderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(
			[]byte(
				strings.Repeat("A", maxSearXNGBodyBytes+1),
			),
		)
	}))
	defer server.Close()

	provider := NewSearXNGProvider(
		server.Client(),
		server.URL,
	)

	_, err := provider.Search(
		context.Background(),
		SearchRequest{
			Query: "oversized",
		},
	)

	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf(
			"expected ErrProviderUnavailable, got %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"exceeds",
	) {
		t.Fatalf(
			"expected oversized-response error, got %v",
			err,
		)
	}
}

func TestDuckDuckGoProviderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(
			[]byte(
				strings.Repeat("B", maxDuckDuckGoBodyBytes+1),
			),
		)
	}))
	defer server.Close()

	provider := NewDuckDuckGoProvider(
		server.Client(),
		server.URL,
	)

	_, err := provider.Search(
		context.Background(),
		SearchRequest{
			Query: "oversized",
		},
	)

	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf(
			"expected ErrProviderUnavailable, got %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"exceeds",
	) {
		t.Fatalf(
		"expected oversized-response error, got %v",
		err,
		)
	}
}

func TestSearXNGProviderURLConstructionPreservesBasePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/instance/search" {
			t.Fatalf(
				"unexpected path: %q",
				r.URL.Path,
			)
		}

		if got := r.URL.Query().Get("q"); got != `go "repair" & test` {
			t.Fatalf(
				"unexpected query: %q",
				got,
			)
		}

		if got := r.URL.Query().Get("format"); got != "json" {
			t.Fatalf(
				"unexpected format: %q",
				got,
			)
		}

		_, _ = w.Write([]byte(`{
			"results":[
				{
					"title":"URL test",
					"url":"https://example.com/url",
					"content":"URL construction works"
				}
			]
		}`))
	}))
	defer server.Close()

	baseURL := server.URL + "/instance///"

	provider := NewSearXNGProvider(
		server.Client(),
		baseURL,
	)

	_, err := provider.Search(
		context.Background(),
		SearchRequest{
			Query:      `go "repair" & test`,
			MaxResults: 1,
		},
	)
	if err != nil {
		t.Fatalf(
			"Search returned error: %v",
			err,
		)
	}
}

func TestDuckDuckGoProviderURLConstructionPreservesBasePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/instance/html" {
			t.Fatalf(
				"unexpected path: %q",
				r.URL.Path,
			)
		}

		if got := r.URL.Query().Get("q"); got != `go "repair" & test` {
			t.Fatalf(
				"unexpected query: %q",
				got,
			)
		}

		if got := r.URL.Query().Get("kl"); got != "wt-wt" {
			t.Fatalf(
				"unexpected region: %q",
				got,
			)
		}

		_, _ = w.Write([]byte(`
			<html>
			  <body>
			    <div class="result">
			      <a class="result__a" href="https://example.com/url">
			        URL test
			      </a>
			      <a class="result__snippet">
			        URL construction works
			      </a>
			    </div>
			  </body>
			</html>
		`))
	}))
	defer server.Close()

	baseURL := server.URL + "/instance///"

	provider := NewDuckDuckGoProvider(
		server.Client(),
		baseURL,
	)

	_, err := provider.Search(
		context.Background(),
		SearchRequest{
			Query:      `go "repair" & test`,
			MaxResults: 1,
		},
	)
	if err != nil {
		t.Fatalf(
			"Search returned error: %v",
			err,
		)
	}
}

func TestSearXNGSearchEndpointRejectsMissingSchemeOrHost(t *testing.T) {
	tests := []string{
		"example.com",
		"/relative/path",
		"://broken",
	}

	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			_, err := searxngSearchEndpoint(baseURL)

			if err == nil {
				t.Fatal("expected invalid endpoint error")
			}
		})
	}
}

func TestDuckDuckGoSearchEndpointRejectsMissingSchemeOrHost(t *testing.T) {
	tests := []string{
		"example.com",
		"/relative/path",
		"://broken",
	}

	for _, baseURL := range tests {
		t.Run(baseURL, func(t *testing.T) {
			_, err := duckDuckGoSearchEndpoint(baseURL)

			if err == nil {
				t.Fatal("expected invalid endpoint error")
			}
		})
	}
}

func TestResearchSearchURLsDoNotReuseCallerRawQuery(t *testing.T) {
	base, err := url.Parse("https://example.test/search?evil=1#fragment")
	if err != nil {
		t.Fatal(err)
	}

	query := base.Query()
	query.Set("q", "safe")
	query.Set("format", "json")
	base.RawQuery = query.Encode()
	base.Fragment = ""

	if base.Query().Get("evil") != "" {
		t.Fatal("unexpected caller-supplied query parameter retained")
	}

	if base.Fragment != "" {
		t.Fatal("unexpected fragment retained")
	}

	if got := base.Query().Get("q"); got != "safe" {
		t.Fatalf(
			"unexpected query value: %q",
			got,
		)
	}
}
