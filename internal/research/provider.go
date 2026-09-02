package research

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrProviderUnavailable = errors.New("research provider unavailable")
	ErrInvalidQuery        = errors.New("invalid research query")
	ErrNoResults           = errors.New("research returned no results")
)

// Provider identifies one external research backend.
type Provider interface {
	Name() string
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)
}

// SearchRequest describes one bounded research operation.
type SearchRequest struct {
	Query      string
	MaxResults int
}

// SearchResponse is the normalized result envelope returned by all providers.
type SearchResponse struct {
	Provider string
	Query    string
	Results  []Result
	Duration time.Duration
}

// Result is one normalized research finding.
//
// The research layer deliberately stores evidence metadata rather than only
// snippets. This allows later ranking, caching, and local verification to use
// source quality and provenance.
type Result struct {
	Title       string
	URL         string
	Snippet     string
	Source      string
	Provider    string
	PublishedAt time.Time

	// Evidence fields.
	Authority   Authority
	MatchScore  float64
	ContentHash string

	// Optional raw metadata retained by a provider.
	Metadata map[string]any
}

// Authority represents the approximate trust level of a source.
//
// It is a ranking signal, not proof. Local execution and objective
// verification remain the final acceptance criteria.
type Authority int

const (
	AuthorityUnknown Authority = iota
	AuthorityCommunity
	AuthorityTechnicalDiscussion
	AuthorityProjectSource
	AuthorityMaintainer
	AuthorityOfficial
)

func (a Authority) String() string {
	switch a {
	case AuthorityOfficial:
		return "official"
	case AuthorityMaintainer:
		return "maintainer"
	case AuthorityProjectSource:
		return "project_source"
	case AuthorityTechnicalDiscussion:
		return "technical_discussion"
	case AuthorityCommunity:
		return "community"
	default:
		return "unknown"
	}
}

// Rank is the normalized numeric ranking value for authority.
func (a Authority) Rank() float64 {
	switch a {
	case AuthorityOfficial:
		return 1.00
	case AuthorityMaintainer:
		return 0.90
	case AuthorityProjectSource:
		return 0.80
	case AuthorityTechnicalDiscussion:
		return 0.60
	case AuthorityCommunity:
		return 0.40
	default:
		return 0.20
	}
}

// Validate ensures a request is safe and meaningful before reaching a
// provider.
func (r SearchRequest) Validate() error {
	r.Query = strings.TrimSpace(r.Query)

	if r.Query == "" {
		return ErrInvalidQuery
	}

	if r.MaxResults < 0 {
		return fmt.Errorf("%w: max results cannot be negative", ErrInvalidQuery)
	}

	return nil
}

// Normalize applies conservative bounds to a search request.
//
// A zero MaxResults means "provider default"; the caller can apply a
// configured project-wide default before invoking the provider.
func (r SearchRequest) Normalize() SearchRequest {
	r.Query = strings.TrimSpace(r.Query)

	if r.MaxResults < 0 {
		r.MaxResults = 0
	}

	return r
}

// Validate ensures a provider response has a coherent envelope.
func (r SearchResponse) Validate() error {
	if strings.TrimSpace(r.Provider) == "" {
		return fmt.Errorf("%w: provider name missing", ErrProviderUnavailable)
	}

	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("%w: query missing", ErrInvalidQuery)
	}

	if len(r.Results) == 0 {
		return ErrNoResults
	}

	return nil
}

// NormalizeResult fills provider-independent fields and clamps invalid
// ranking values before results enter ranking/cache layers.
func NormalizeResult(result Result, provider string) Result {
	result.Title = strings.TrimSpace(result.Title)
	result.URL = strings.TrimSpace(result.URL)
	result.Snippet = strings.TrimSpace(result.Snippet)
	result.Source = strings.TrimSpace(result.Source)

	if result.Provider == "" {
		result.Provider = provider
	}

	if result.MatchScore < 0 {
		result.MatchScore = 0
	}
	if result.MatchScore > 1 {
		result.MatchScore = 1
	}

	if result.Authority < AuthorityUnknown || result.Authority > AuthorityOfficial {
		result.Authority = AuthorityUnknown
	}

	return result
}

// NormalizeResponse applies common provider normalization without changing
// the provider-specific payload.
func NormalizeResponse(response SearchResponse) SearchResponse {
	response.Provider = strings.TrimSpace(response.Provider)
	response.Query = strings.TrimSpace(response.Query)

	normalized := make([]Result, 0, len(response.Results))
	for _, result := range response.Results {
		normalized = append(
			normalized,
			NormalizeResult(result, response.Provider),
		)
	}

	response.Results = normalized

	return response
}
