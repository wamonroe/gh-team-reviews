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
	ReviewRequests           connection[requestedReviewer] `json:"reviewRequests"`
	TimelineItems            connection[requestedReviewer] `json:"timelineItems"`
	LatestOpinionatedReviews connection[review]            `json:"latestOpinionatedReviews"`
	ViewerReviews            struct {
		TotalCount int `json:"totalCount"`
	} `json:"viewerReviews"`
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

type review struct {
	State  string `json:"state"`
	Author *actor `json:"author"`
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

func (pr PullRequest) ReviewedByViewer() bool {
	return pr.ViewerReviews.TotalCount > 0
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

// RequestedTeams returns teams with a pending request plus any team that was
// ever requested. GitHub removes a team's request as soon as one member
// reviews, so the timeline is the only record that the team was asked.
func (pr PullRequest) RequestedTeams() []string {
	seen := map[string]bool{}
	var teams []string
	for _, nodes := range [][]requestedReviewer{pr.ReviewRequests.Nodes, pr.TimelineItems.Nodes} {
		for _, node := range nodes {
			if r := node.RequestedReviewer; r != nil && r.CombinedSlug != "" && !seen[r.CombinedSlug] {
				seen[r.CombinedSlug] = true
				teams = append(teams, r.CombinedSlug)
			}
		}
	}
	return teams
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
