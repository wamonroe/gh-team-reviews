package review

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/wamonroe/gh-team-reviews/internal/dismissed"
	"github.com/wamonroe/gh-team-reviews/internal/github"
)

const baseQuery = "is:pr is:open archived:false draft:false"

// GitHub search allows at most five AND/OR/NOT operators per query, so
// teammates are split across several `reviewed-by:` searches.
const membersPerQuery = 5

// Source is the slice of the GitHub API the queue needs.
type Source interface {
	Viewer(ctx context.Context) (string, error)
	TeamMembers(ctx context.Context, team string) ([]string, error)
	SearchPullRequests(ctx context.Context, query, viewer string) ([]github.PullRequest, error)
}

type Options struct {
	Team      string // "org/slug"; empty tracks direct requests only
	Required  int
	Dismissed dismissed.List
	// IncludeDismissed keeps dismissed PRs in the results; see
	// Entry.Dismissed.
	IncludeDismissed bool
}

// Build returns the PRs that still need the viewer's review, most recently
// updated first, leaving out dismissed PRs unless asked. All searches run
// concurrently.
func Build(ctx context.Context, src Source, opts Options) ([]Entry, error) {
	var viewer string
	var members []string

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		viewer, err = src.Viewer(gctx)
		return err
	})
	if opts.Team != "" {
		g.Go(func() (err error) {
			members, err = src.TeamMembers(gctx, opts.Team)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	queries := Queries(opts.Team, viewer, members)
	results := make([][]github.PullRequest, len(queries))

	g, gctx = errgroup.WithContext(ctx)
	for i, query := range queries {
		g.Go(func() (err error) {
			results[i], err = src.SearchPullRequests(gctx, query, viewer)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	entries := []Entry{}
	for _, prs := range results {
		for _, pr := range prs {
			if seen[pr.URL] {
				continue
			}
			seen[pr.URL] = true

			entry := Entry{
				PR:          pr,
				Viewer:      viewer,
				Team:        opts.Team,
				Members:     members,
				Required:    opts.Required,
				DismissedAt: opts.Dismissed.At(pr.Ref()),
			}
			if entry.NeedsReview() && (opts.IncludeDismissed || !entry.Dismissed()) {
				entries = append(entries, entry)
			}
		}
	}

	slices.SortStableFunc(entries, func(a, b Entry) int {
		return b.PR.UpdatedAt.Compare(a.PR.UpdatedAt)
	})
	return entries, nil
}

// Queries returns the searches that together find every candidate PR.
// Candidates are filtered afterwards by Entry.NeedsReview.
func Queries(team, viewer string, members []string) []string {
	queries := []string{
		baseQuery + " user-review-requested:@me",
		// GitHub clears the viewer's request once they review, even with just
		// a comment, so this catches PRs that have fallen out of
		// `user-review-requested`.
		baseQuery + " -author:@me reviewed-by:@me",
	}
	if team == "" {
		return queries
	}

	org, _, _ := strings.Cut(team, "/")
	queries = append(queries, fmt.Sprintf("%s org:%s team-review-requested:%s", baseQuery, org, team))

	// Once a teammate reviews, GitHub drops the team request, so these catch
	// PRs that have fallen out of `team-review-requested`.
	teammates := slices.DeleteFunc(slices.Clone(members), func(m string) bool { return m == viewer })
	for chunk := range slices.Chunk(teammates, membersPerQuery) {
		reviewedBy := make([]string, len(chunk))
		for i, login := range chunk {
			reviewedBy[i] = "reviewed-by:" + login
		}
		queries = append(queries, fmt.Sprintf("%s org:%s -author:@me -reviewed-by:@me (%s)",
			baseQuery, org, strings.Join(reviewedBy, " OR ")))
	}
	return queries
}
