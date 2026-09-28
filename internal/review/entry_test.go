package review

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/wamonroe/gh-team-reviews/internal/github"
)

const team = "acme/payments"

var members = []string{"me", "alice", "bob", "carol"}

// pr builds a PullRequest from the same JSON shape the GraphQL API returns.
func pr(t *testing.T, raw string) github.PullRequest {
	t.Helper()
	var p github.PullRequest
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func entry(p github.PullRequest) Entry {
	return Entry{PR: p, Viewer: "me", Team: team, Members: members, Required: 2}
}

func TestNeedsReview(t *testing.T) {
	tests := []struct {
		name string
		json string
		want bool
	}{
		{
			name: "team pending, no reviews",
			json: `{"author":{"login":"dave"},
				"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: true,
		},
		{
			name: "team request consumed by one teammate approval",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":{"login":"alice"}}]}}`,
			want: true,
		},
		{
			name: "two teammate approvals",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"latestOpinionatedReviews":{"nodes":[
					{"state":"APPROVED","author":{"login":"alice"}},
					{"state":"APPROVED","author":{"login":"bob"}}]}}`,
			want: false,
		},
		{
			name: "approvals from outside the team don't count",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"latestOpinionatedReviews":{"nodes":[
					{"state":"APPROVED","author":{"login":"alice"}},
					{"state":"APPROVED","author":{"login":"outsider"}}]}}`,
			want: true,
		},
		{
			name: "changes requested doesn't count as an approval",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"latestOpinionatedReviews":{"nodes":[
					{"state":"APPROVED","author":{"login":"alice"}},
					{"state":"CHANGES_REQUESTED","author":{"login":"bob"}}]}}`,
			want: true,
		},
		{
			name: "viewer already reviewed",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"viewerReviews":{"totalCount":1}}`,
			want: false,
		},
		{
			name: "team never requested",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/other"}}]},
				"latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":{"login":"alice"}}]}}`,
			want: false,
		},
		{
			name: "directly requested even with enough approvals and a prior review",
			json: `{"author":{"login":"dave"},
				"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]},
				"latestOpinionatedReviews":{"nodes":[
					{"state":"APPROVED","author":{"login":"alice"}},
					{"state":"APPROVED","author":{"login":"bob"}}]},
				"viewerReviews":{"totalCount":1}}`,
			want: true,
		},
		{
			name: "viewer's own PR",
			json: `{"author":{"login":"me"},
				"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: false,
		},
		{
			name: "draft",
			json: `{"author":{"login":"dave"},"isDraft":true,
				"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]}}`,
			want: false,
		},
		{
			name: "deleted author",
			json: `{"author":null,
				"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entry(pr(t, tt.json)).NeedsReview(); got != tt.want {
				t.Errorf("NeedsReview() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsReviewWithoutTeam(t *testing.T) {
	teamPending := pr(t, `{"author":{"login":"dave"},
		"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`)
	direct := pr(t, `{"author":{"login":"dave"},
		"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]}}`)

	if (Entry{PR: teamPending, Viewer: "me", Required: 2}).NeedsReview() {
		t.Error("team-requested PR listed with no team configured")
	}
	if !(Entry{PR: direct, Viewer: "me", Required: 2}).NeedsReview() {
		t.Error("direct request not listed with no team configured")
	}
}

const mixedReviews = `"latestOpinionatedReviews":{"nodes":[
	{"state":"APPROVED","author":{"login":"alice"}},
	{"state":"APPROVED","author":{"login":"outsider"}},
	{"state":"CHANGES_REQUESTED","author":{"login":"bob"}}]}`

func TestTeamEntryCountsTeamReviews(t *testing.T) {
	e := entry(pr(t, `{"author":{"login":"dave"},
		"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
		`+mixedReviews+`}`))

	if got := e.Reason(); got != "team" {
		t.Errorf("Reason() = %q, want team", got)
	}
	if got := e.Approvals(); !slices.Equal(got, []string{"alice"}) {
		t.Errorf("Approvals() = %v, want [alice]", got)
	}
	if got := e.RequiredApprovals(); got != 2 {
		t.Errorf("RequiredApprovals() = %d, want 2", got)
	}
}

// A direct request waits only on the viewer, even when the team was also
// requested and teammates have reviewed.
func TestDirectEntryWaitsOnViewerOnly(t *testing.T) {
	e := entry(pr(t, `{"author":{"login":"dave"},
		"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]},
		"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
		`+mixedReviews+`}`))

	if got := e.Reason(); got != "you" {
		t.Errorf("Reason() = %q, want you", got)
	}
	if got := e.Approvals(); len(got) != 0 {
		t.Errorf("Approvals() = %v, want none", got)
	}
	if got := e.RequiredApprovals(); got != 1 {
		t.Errorf("RequiredApprovals() = %d, want 1", got)
	}
}
