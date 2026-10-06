package review

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wamonroe/gh-team-reviews/internal/dismissed"
	"github.com/wamonroe/gh-team-reviews/internal/github"
)

func TestQueriesWithoutTeam(t *testing.T) {
	got := Queries("", "me", nil)
	want := []string{
		"is:pr is:open archived:false draft:false user-review-requested:@me",
		"is:pr is:open archived:false draft:false -author:@me reviewed-by:@me",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Queries() = %q, want %q", got, want)
	}
}

func TestQueriesChunksTeammatesAndSkipsViewer(t *testing.T) {
	members := []string{"a", "b", "me", "c", "d", "e", "f"}
	got := Queries("acme/payments", "me", members)

	want := []string{
		"is:pr is:open archived:false draft:false user-review-requested:@me",
		"is:pr is:open archived:false draft:false -author:@me reviewed-by:@me",
		"is:pr is:open archived:false draft:false org:acme team-review-requested:acme/payments",
		"is:pr is:open archived:false draft:false org:acme -author:@me -reviewed-by:@me " +
			"(reviewed-by:a OR reviewed-by:b OR reviewed-by:c OR reviewed-by:d OR reviewed-by:e)",
		"is:pr is:open archived:false draft:false org:acme -author:@me -reviewed-by:@me (reviewed-by:f)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Queries() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !slices.Equal(members, []string{"a", "b", "me", "c", "d", "e", "f"}) {
		t.Errorf("Queries() mutated members: %v", members)
	}
}

type fakeSource struct {
	mu       sync.Mutex
	queries  []string
	byPrefix map[string][]github.PullRequest
}

func (f *fakeSource) Viewer(context.Context) (string, error) { return "me", nil }

func (f *fakeSource) TeamMembers(context.Context, string) ([]string, error) {
	return []string{"me", "alice", "bob"}, nil
}

func (f *fakeSource) SearchPullRequests(_ context.Context, query, _ string) ([]github.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	for marker, prs := range f.byPrefix {
		if strings.Contains(query, marker) {
			return prs, nil
		}
	}
	return nil, nil
}

func TestBuildDedupesFiltersAndSorts(t *testing.T) {
	older := pr(t, `{"url":"u/1","updatedAt":"2026-01-01T00:00:00Z","author":{"login":"dave"},
		"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]}}`)
	newer := pr(t, `{"url":"u/2","updatedAt":"2026-01-02T00:00:00Z","author":{"login":"dave"},
		"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
		"latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":{"login":"alice"}}]}}`)
	done := pr(t, `{"url":"u/3","updatedAt":"2026-01-03T00:00:00Z","author":{"login":"dave"},
		"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
		"latestOpinionatedReviews":{"nodes":[
			{"state":"APPROVED","author":{"login":"alice"}},
			{"state":"APPROVED","author":{"login":"bob"}}]}}`)

	src := &fakeSource{byPrefix: map[string][]github.PullRequest{
		"user-review-requested": {older},
		"reviewed-by:alice":     {newer, done, older},
	}}

	entries, err := Build(context.Background(), src, Options{Team: "acme/payments", Required: 2})
	if err != nil {
		t.Fatal(err)
	}

	var urls []string
	for _, e := range entries {
		urls = append(urls, e.PR.URL)
	}
	if !slices.Equal(urls, []string{"u/2", "u/1"}) {
		t.Errorf("Build() urls = %v, want [u/2 u/1]", urls)
	}
	if len(src.queries) != 4 {
		t.Errorf("ran %d searches, want 4: %q", len(src.queries), src.queries)
	}
	if !entries[0].PR.UpdatedAt.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("unexpected first entry: %v", entries[0].PR.UpdatedAt)
	}
}

func TestBuildWithoutTeamOnlySearchesViewerRequests(t *testing.T) {
	src := &fakeSource{}
	if _, err := Build(context.Background(), src, Options{Required: 2}); err != nil {
		t.Fatal(err)
	}
	slices.Sort(src.queries)
	want := []string{
		"is:pr is:open archived:false draft:false -author:@me reviewed-by:@me",
		"is:pr is:open archived:false draft:false user-review-requested:@me",
	}
	if !slices.Equal(src.queries, want) {
		t.Errorf("searches = %q, want %q", src.queries, want)
	}
}

func TestBuildHidesDismissedUnlessIncluded(t *testing.T) {
	kept := pr(t, `{"url":"u/1","number":1,"repository":{"nameWithOwner":"acme/api"},"author":{"login":"dave"},
		"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]}}`)
	hidden := pr(t, `{"url":"u/2","number":2,"repository":{"nameWithOwner":"acme/api"},"author":{"login":"dave"},
		"timelineItems":{"nodes":[
			{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}}]}}`)
	src := &fakeSource{byPrefix: map[string][]github.PullRequest{"user-review-requested": {kept, hidden}}}

	list := dismissed.List{}
	list.Add(github.Ref{Repo: "Acme/API", Number: 2}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))

	for _, include := range []bool{false, true} {
		entries, err := Build(context.Background(), src, Options{Required: 2, Dismissed: list, IncludeDismissed: include})
		if err != nil {
			t.Fatal(err)
		}
		var urls []string
		for _, e := range entries {
			urls = append(urls, e.PR.URL)
		}
		slices.Sort(urls)
		want := []string{"u/1"}
		if include {
			want = []string{"u/1", "u/2"}
		}
		if !slices.Equal(urls, want) {
			t.Errorf("IncludeDismissed=%v: urls = %v, want %v", include, urls, want)
		}
	}
}
