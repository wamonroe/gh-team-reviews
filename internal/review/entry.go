package review

import (
	"slices"

	"github.com/wamonroe/gh-team-reviews/internal/github"
)

// Entry pairs a pull request with the context needed to judge whether it
// still needs the viewer's review.
type Entry struct {
	PR       github.PullRequest
	Viewer   string
	Team     string // "org/slug", or empty when only tracking direct requests
	Members  []string
	Required int
}

// NeedsReview reports whether the PR belongs in the viewer's queue:
//
//   - the viewer is directly requested, or
//   - the team was requested (now or at any point), fewer than Required team
//     members have approved, and the viewer hasn't reviewed it yet.
func (e Entry) NeedsReview() bool {
	if e.PR.IsDraft || e.PR.AuthorLogin() == e.Viewer {
		return false
	}
	if e.DirectlyRequested() {
		return true
	}
	return e.TeamRequested() && !e.PR.ReviewedByViewer() && len(e.teamApprovals()) < e.Required
}

func (e Entry) DirectlyRequested() bool {
	return slices.Contains(e.PR.PendingUsers(), e.Viewer)
}

func (e Entry) TeamRequested() bool {
	return e.Team != "" && slices.Contains(e.PR.RequestedTeams(), e.Team)
}

// RequiredApprovals is how many approvals this entry is waiting on. A direct
// request waits only on the viewer's own review, whatever anyone else does.
func (e Entry) RequiredApprovals() int {
	if e.DirectlyRequested() {
		return 1
	}
	return e.Required
}

// Approvals returns the approvals counting toward RequiredApprovals: team
// members whose latest review is an approval. Direct requests have none,
// since the viewer's pending review is the only one that counts.
func (e Entry) Approvals() []string {
	if e.DirectlyRequested() {
		return []string{}
	}
	return e.teamApprovals()
}

// Reason is "you" for direct requests and "team" for team requests.
func (e Entry) Reason() string {
	if e.DirectlyRequested() {
		return "you"
	}
	return "team"
}

func (e Entry) teamApprovals() []string {
	approvals := []string{}
	for _, login := range e.PR.Approvers() {
		if slices.Contains(e.Members, login) {
			approvals = append(approvals, login)
		}
	}
	return approvals
}
