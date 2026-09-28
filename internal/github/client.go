package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
)

const searchQuery = `
query($q: String!, $viewer: String!, $cursor: String) {
  search(type: ISSUE_ADVANCED, query: $q, first: 50, after: $cursor) {
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {
        number
        title
        url
        isDraft
        updatedAt
        author { login }
        repository { nameWithOwner }
        reviewRequests(first: 50) {
          nodes { requestedReviewer { ... on User { login } ... on Team { combinedSlug } } }
        }
        timelineItems(first: 100, itemTypes: [REVIEW_REQUESTED_EVENT]) {
          nodes { ... on ReviewRequestedEvent { requestedReviewer { ... on Team { combinedSlug } } } }
        }
        latestOpinionatedReviews(first: 50) {
          nodes { state author { login } }
        }
        viewerReviews: reviews(first: 1, author: $viewer) { totalCount }
      }
    }
  }
}`

const teamMembersQuery = `
query($org: String!, $slug: String!, $cursor: String) {
  organization(login: $org) {
    team(slug: $slug) {
      members(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes { login }
      }
    }
  }
}`

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// Client talks to GitHub using the credentials gh is already logged in with.
type Client struct {
	gql *api.GraphQLClient
}

func NewClient() (*Client, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql}, nil
}

func (c *Client) Viewer(ctx context.Context) (string, error) {
	var resp struct {
		Viewer actor `json:"viewer"`
	}
	if err := c.gql.DoWithContext(ctx, `query { viewer { login } }`, nil, &resp); err != nil {
		return "", fmt.Errorf("looking up current user: %w", err)
	}
	return resp.Viewer.Login, nil
}

// TeamMembers returns the logins of everyone on team ("org/slug"), including
// members of child teams.
func (c *Client) TeamMembers(ctx context.Context, team string) ([]string, error) {
	org, slug, _ := strings.Cut(team, "/")
	var logins []string
	var cursor *string

	for {
		var resp struct {
			Organization *struct {
				Team *struct {
					Members struct {
						PageInfo pageInfo `json:"pageInfo"`
						Nodes    []actor  `json:"nodes"`
					} `json:"members"`
				} `json:"team"`
			} `json:"organization"`
		}
		vars := map[string]interface{}{"org": org, "slug": slug, "cursor": cursor}
		if err := c.gql.DoWithContext(ctx, teamMembersQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("looking up members of %s: %w", team, err)
		}
		if resp.Organization == nil || resp.Organization.Team == nil {
			return nil, fmt.Errorf("team %s not found (or you can't see it)", team)
		}

		members := resp.Organization.Team.Members
		for _, m := range members.Nodes {
			logins = append(logins, m.Login)
		}
		if !members.PageInfo.HasNextPage {
			return logins, nil
		}
		cursor = &members.PageInfo.EndCursor
	}
}

// SearchPullRequests runs an advanced (boolean-capable) search and returns
// every matching pull request across all result pages.
func (c *Client) SearchPullRequests(ctx context.Context, query, viewer string) ([]PullRequest, error) {
	var results []PullRequest
	var cursor *string

	for {
		var resp struct {
			Search struct {
				PageInfo pageInfo      `json:"pageInfo"`
				Nodes    []PullRequest `json:"nodes"`
			} `json:"search"`
		}
		vars := map[string]interface{}{"q": query, "viewer": viewer, "cursor": cursor}
		if err := c.gql.DoWithContext(ctx, searchQuery, vars, &resp); err != nil {
			return nil, fmt.Errorf("searching %q: %w", query, err)
		}

		for _, pr := range resp.Search.Nodes {
			if pr.URL != "" {
				results = append(results, pr)
			}
		}
		if !resp.Search.PageInfo.HasNextPage {
			return results, nil
		}
		cursor = &resp.Search.PageInfo.EndCursor
	}
}
