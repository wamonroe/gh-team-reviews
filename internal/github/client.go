package github

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
        timelineItems(last: 100, itemTypes: [REVIEW_REQUESTED_EVENT, REVIEW_REQUEST_REMOVED_EVENT]) {
          nodes {
            __typename
            ... on ReviewRequestedEvent {
              createdAt
              requestedReviewer { ... on User { login } ... on Team { combinedSlug } }
            }
            ... on ReviewRequestRemovedEvent {
              createdAt
              requestedReviewer { ... on User { login } ... on Team { combinedSlug } }
            }
          }
        }
        latestOpinionatedReviews(first: 50) {
          nodes { state author { login } }
        }
        viewerDecisions: reviews(last: 1, author: $viewer, states: [APPROVED, CHANGES_REQUESTED]) {
          nodes { submittedAt }
        }
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

// PullRequestStates returns the state (OPEN, CLOSED, or MERGED) of each
// pull request in refs, or "" for any that don't exist or you can't see.
func (c *Client) PullRequestStates(ctx context.Context, refs []Ref) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	var params, fields []string
	vars := map[string]interface{}{}
	for i, ref := range refs {
		owner, name, _ := strings.Cut(ref.Repo, "/")
		params = append(params, fmt.Sprintf("$owner%d: String!, $name%d: String!, $number%d: Int!", i, i, i))
		fields = append(fields, fmt.Sprintf(
			"pr%d: repository(owner: $owner%d, name: $name%d) { pullRequest(number: $number%d) { state } }", i, i, i, i))
		vars[fmt.Sprintf("owner%d", i)] = owner
		vars[fmt.Sprintf("name%d", i)] = name
		vars[fmt.Sprintf("number%d", i)] = ref.Number
	}
	query := fmt.Sprintf("query(%s) {\n%s\n}", strings.Join(params, ", "), strings.Join(fields, "\n"))

	var resp map[string]*struct {
		PullRequest *struct {
			State string `json:"state"`
		} `json:"pullRequest"`
	}
	// Missing repos and PRs come back as NOT_FOUND errors alongside the data
	// for the rest.
	err := c.gql.DoWithContext(ctx, query, vars, &resp)
	var gqlErr *api.GraphQLError
	if errors.As(err, &gqlErr) && !slices.ContainsFunc(gqlErr.Errors, func(e api.GraphQLErrorItem) bool {
		return e.Type != "NOT_FOUND"
	}) {
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up pull requests: %w", err)
	}

	states := make([]string, len(refs))
	for i := range refs {
		if repo := resp[fmt.Sprintf("pr%d", i)]; repo != nil && repo.PullRequest != nil {
			states[i] = repo.PullRequest.State
		}
	}
	return states, nil
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
