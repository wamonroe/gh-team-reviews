package review

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

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
			name: "team requested, viewer only commented",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: true,
		},
		{
			name: "team requested, viewer approved or requested changes",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"viewerDecisions":{"nodes":[{"submittedAt":"2026-01-02T00:00:00Z"}]}}`,
			want: false,
		},
		{
			name: "team request removed",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
					{"__typename":"ReviewRequestRemovedEvent","createdAt":"2026-01-02T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: false,
		},
		{
			name: "team re-requested after removal",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
					{"__typename":"ReviewRequestRemovedEvent","createdAt":"2026-01-02T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-03T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: true,
		},
		{
			name: "removing the viewer doesn't hide a team request",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}},
					{"__typename":"ReviewRequestRemovedEvent","createdAt":"2026-01-02T00:00:00Z","requestedReviewer":{"login":"me"}}]}}`,
			want: true,
		},
		{
			name: "direct request cleared by viewer's comment",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}}]}}`,
			want: true,
		},
		{
			name: "direct request cleared by viewer's approval",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}}]},
				"viewerDecisions":{"nodes":[{"submittedAt":"2026-01-02T00:00:00Z"}]}}`,
			want: false,
		},
		{
			name: "re-requested after viewer approved, then viewer commented",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}},
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-03T00:00:00Z","requestedReviewer":{"login":"me"}}]},
				"viewerDecisions":{"nodes":[{"submittedAt":"2026-01-02T00:00:00Z"}]}}`,
			want: true,
		},
		{
			name: "direct request removed",
			json: `{"author":{"login":"dave"},
				"timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"login":"me"}},
					{"__typename":"ReviewRequestRemovedEvent","createdAt":"2026-01-02T00:00:00Z","requestedReviewer":{"login":"me"}}]}}`,
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
				"viewerDecisions":{"nodes":[{"submittedAt":"2026-01-02T00:00:00Z"}]}}`,
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

func TestDismissed(t *testing.T) {
	dismissedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		json        string
		dismissedAt time.Time
		want        bool
	}{
		{
			name: "not dismissed",
			json: `{"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			want: false,
		},
		{
			name: "dismissed after the team was requested",
			json: `{"timelineItems":{"nodes":[
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			dismissedAt: dismissedAt,
			want:        true,
		},
		{
			name: "team re-requested since",
			json: `{"timelineItems":{"nodes":[
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-03T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			dismissedAt: dismissedAt,
			want:        false,
		},
		{
			name: "viewer requested since",
			json: `{"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]},
				"timelineItems":{"nodes":[
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-03T00:00:00Z","requestedReviewer":{"login":"me"}}]}}`,
			dismissedAt: dismissedAt,
			want:        false,
		},
		{
			name: "someone else requested since",
			json: `{"timelineItems":{"nodes":[
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-01T00:00:00Z","requestedReviewer":{"combinedSlug":"acme/payments"}},
				{"__typename":"ReviewRequestedEvent","createdAt":"2026-01-03T00:00:00Z","requestedReviewer":{"login":"alice"}}]}}`,
			dismissedAt: dismissedAt,
			want:        true,
		},
		{
			name:        "pending team request from before the fetched timeline",
			json:        `{"reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]}}`,
			dismissedAt: dismissedAt,
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entry(pr(t, tt.json))
			e.DismissedAt = tt.dismissedAt
			if got := e.Dismissed(); got != tt.want {
				t.Errorf("Dismissed() = %v, want %v", got, tt.want)
			}
		})
	}
}
