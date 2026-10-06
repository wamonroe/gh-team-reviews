package github

import "time"

// PullRequest is the subset of a GitHub pull request needed to decide whether
// it still needs the viewer's review.
type PullRequest struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	IsDraft    bool      `json:"isDraft"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Author     *actor    `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	ReviewRequests           connection[requestedReviewer]  `json:"reviewRequests"`
	TimelineItems            connection[reviewRequestEvent] `json:"timelineItems"`
	LatestOpinionatedReviews connection[review]             `json:"latestOpinionatedReviews"`
	ViewerDecisions          connection[decision]           `json:"viewerDecisions"`
}

type connection[T any] struct {
	Nodes []T `json:"nodes"`
}

type actor struct {
	Login string `json:"login"`
}

// requestedReviewer is either a user (Login) or a team (CombinedSlug).
type requestedReviewer struct {
	RequestedReviewer *struct {
		Login        string `json:"login"`
		CombinedSlug string `json:"combinedSlug"`
	} `json:"requestedReviewer"`
}

// reviewRequestEvent is a timeline entry recording a review request being
// made or removed, in chronological order.
type reviewRequestEvent struct {
	Typename          string    `json:"__typename"`
	CreatedAt         time.Time `json:"createdAt"`
	RequestedReviewer *struct {
		Login        string `json:"login"`
		CombinedSlug string `json:"combinedSlug"`
	} `json:"requestedReviewer"`
}

func (e reviewRequestEvent) removed() bool {
	return e.Typename == "ReviewRequestRemovedEvent"
}

type review struct {
	State  string `json:"state"`
	Author *actor `json:"author"`
}

// decision is a review that approved or requested changes.
type decision struct {
	SubmittedAt time.Time `json:"submittedAt"`
}

func (pr PullRequest) AuthorLogin() string {
	if pr.Author == nil {
		return ""
	}
	return pr.Author.Login
}

func (pr PullRequest) Repo() string {
	return pr.Repository.NameWithOwner
}

// ViewerDecidedAt returns when the viewer last approved or requested changes.
// Comment-only reviews don't count.
func (pr PullRequest) ViewerDecidedAt() (time.Time, bool) {
	nodes := pr.ViewerDecisions.Nodes
	if len(nodes) == 0 {
		return time.Time{}, false
	}
	return nodes[len(nodes)-1].SubmittedAt, true
}

// UserRequestedAt returns when login was last asked to review, or false if
// they never were or that request was later removed. The request may no
// longer be pending: GitHub clears it once the user submits any review, even
// a comment.
func (pr PullRequest) UserRequestedAt(login string) (time.Time, bool) {
	e, ok := pr.lastRequestEvent(func(user, _ string) bool { return user == login })
	if !ok || e.removed() {
		return time.Time{}, false
	}
	return e.CreatedAt, true
}

// PendingUsers returns the logins of users with an outstanding review request.
func (pr PullRequest) PendingUsers() []string {
	var logins []string
	for _, node := range pr.ReviewRequests.Nodes {
		if r := node.RequestedReviewer; r != nil && r.Login != "" {
			logins = append(logins, r.Login)
		}
	}
	return logins
}

// TeamRequestedAt returns when team ("org/slug") was last asked to review,
// or false if it never was or that request was later removed. GitHub clears
// a team's request as soon as one member reviews, so the timeline is the
// only record that the team was asked.
func (pr PullRequest) TeamRequestedAt(team string) (time.Time, bool) {
	e, ok := pr.lastRequestEvent(func(_, slug string) bool { return slug == team })
	if ok {
		return e.CreatedAt, !e.removed()
	}
	// Pending, but requested before the timeline window we fetched.
	for _, node := range pr.ReviewRequests.Nodes {
		if r := node.RequestedReviewer; r != nil && r.CombinedSlug == team {
			return time.Time{}, true
		}
	}
	return time.Time{}, false
}

// lastRequestEvent returns the most recent request or removal event for the
// reviewer matching the given user login or team slug.
func (pr PullRequest) lastRequestEvent(match func(login, slug string) bool) (reviewRequestEvent, bool) {
	nodes := pr.TimelineItems.Nodes
	for i := len(nodes) - 1; i >= 0; i-- {
		if r := nodes[i].RequestedReviewer; r != nil && match(r.Login, r.CombinedSlug) {
			return nodes[i], true
		}
	}
	return reviewRequestEvent{}, false
}

// Approvers returns the logins whose latest opinionated review (approve or
// request changes) is an approval.
func (pr PullRequest) Approvers() []string {
	var logins []string
	for _, r := range pr.LatestOpinionatedReviews.Nodes {
		if r.State == "APPROVED" && r.Author != nil {
			logins = append(logins, r.Author.Login)
		}
	}
	return logins
}
