package review

import (
	"slices"
	"time"

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
	// DismissedAt is when the viewer dismissed the PR, or zero if they
	// haven't.
	DismissedAt time.Time
}

// NeedsReview reports whether the PR belongs in the viewer's queue:
//
//   - the viewer is directly requested, or
//   - the team was requested (now or at any point), fewer than Required team
//     members have approved, and the viewer hasn't approved or requested
//     changes.
//
// Comment-only reviews never take a PR off the queue.
func (e Entry) NeedsReview() bool {
	if e.PR.IsDraft || e.PR.AuthorLogin() == e.Viewer {
		return false
	}
	if e.DirectlyRequested() {
		return true
	}
	_, decided := e.PR.ViewerDecidedAt()
	return e.TeamRequested() && !decided && len(e.teamApprovals()) < e.Required
}

// DirectlyRequested reports whether the viewer has a pending review request,
// or had one that their comment-only reviews cleared: they haven't approved
// or requested changes since they were last asked.
func (e Entry) DirectlyRequested() bool {
	if slices.Contains(e.PR.PendingUsers(), e.Viewer) {
		return true
	}
	requestedAt, requested := e.PR.UserRequestedAt(e.Viewer)
	if !requested {
		return false
	}
	decidedAt, decided := e.PR.ViewerDecidedAt()
	return !decided || decidedAt.Before(requestedAt)
}

func (e Entry) TeamRequested() bool {
	if e.Team == "" {
		return false
	}
	_, requested := e.PR.TeamRequestedAt(e.Team)
	return requested
}

// Dismissed reports whether the viewer dismissed the PR and neither they nor
// the team have been asked to review since.
func (e Entry) Dismissed() bool {
	if e.DismissedAt.IsZero() {
		return false
	}
	if at, ok := e.PR.UserRequestedAt(e.Viewer); ok && at.After(e.DismissedAt) {
		return false
	}
	if e.Team != "" {
		if at, ok := e.PR.TeamRequestedAt(e.Team); ok && at.After(e.DismissedAt) {
			return false
		}
	}
	return true
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
