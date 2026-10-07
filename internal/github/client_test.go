package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// graphQLRequest is what go-gh sends for each query.
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// fakeGraphQL answers each request with the next canned response body, in
// order, and records the requests it saw.
type fakeGraphQL struct {
	t         *testing.T
	status    int
	responses []string
	requests  []graphQLRequest
}

func (f *fakeGraphQL) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Helper()
	if r.Method != http.MethodPost || r.URL.String() != "https://api.github.com/graphql" {
		f.t.Errorf("request = %s %s, want POST https://api.github.com/graphql", r.Method, r.URL)
	}
	if got := r.Header.Get("Authorization"); got != "token test-token" {
		f.t.Errorf("Authorization header = %q, want %q", got, "token test-token")
	}

	var req graphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Fatalf("decoding request body: %v", err)
	}
	f.requests = append(f.requests, req)
	if len(f.requests) > len(f.responses) {
		f.t.Fatalf("unexpected request %d: %s", len(f.requests), req.Query)
	}

	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(f.responses[len(f.requests)-1])),
		Request:    r,
	}, nil
}

// fakeClient returns a Client whose requests are answered by responses, in
// order, without touching the network or gh's stored credentials.
func fakeClient(t *testing.T, responses ...string) (*Client, *fakeGraphQL) {
	t.Helper()
	fake := &fakeGraphQL{t: t, responses: responses}
	gql, err := api.NewGraphQLClient(api.ClientOptions{
		Host:      "github.com",
		AuthToken: "test-token",
		Transport: fake,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{gql: gql}, fake
}

func TestViewer(t *testing.T) {
	client, _ := fakeClient(t, `{"data":{"viewer":{"login":"me"}}}`)

	got, err := client.Viewer(context.Background())
	if err != nil || got != "me" {
		t.Errorf("Viewer() = %q, %v; want %q", got, err, "me")
	}
}

func TestTeamMembersPaginates(t *testing.T) {
	client, fake := fakeClient(t,
		`{"data":{"organization":{"team":{"members":{
			"pageInfo":{"hasNextPage":true,"endCursor":"c1"},
			"nodes":[{"login":"alice"},{"login":"bob"}]}}}}}`,
		`{"data":{"organization":{"team":{"members":{
			"pageInfo":{"hasNextPage":false,"endCursor":"c2"},
			"nodes":[{"login":"carol"}]}}}}}`,
	)

	got, err := client.TeamMembers(context.Background(), "acme/payments")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alice", "bob", "carol"}; !slices.Equal(got, want) {
		t.Errorf("TeamMembers() = %v, want %v", got, want)
	}

	if len(fake.requests) != 2 {
		t.Fatalf("made %d requests, want 2", len(fake.requests))
	}
	first, second := fake.requests[0].Variables, fake.requests[1].Variables
	if first["org"] != "acme" || first["slug"] != "payments" || first["cursor"] != nil {
		t.Errorf("first page variables = %v", first)
	}
	if second["cursor"] != "c1" {
		t.Errorf("second page cursor = %v, want c1", second["cursor"])
	}
}

func TestTeamMembersNotFound(t *testing.T) {
	for name, body := range map[string]string{
		"no org":  `{"data":{"organization":null}}`,
		"no team": `{"data":{"organization":{"team":null}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := fakeClient(t, body)
			_, err := client.TeamMembers(context.Background(), "acme/payments")
			if err == nil || !strings.Contains(err.Error(), "team acme/payments not found") {
				t.Errorf("TeamMembers() error = %v, want team not found", err)
			}
		})
	}
}

func TestPullRequestStates(t *testing.T) {
	// GitHub returns the PRs it found alongside NOT_FOUND errors for the
	// ones it didn't.
	client, fake := fakeClient(t, `{
		"data":{
			"pr0":{"pullRequest":{"state":"OPEN"}},
			"pr1":{"pullRequest":null},
			"pr2":null,
			"pr3":{"pullRequest":{"state":"MERGED"}}},
		"errors":[
			{"type":"NOT_FOUND","path":["pr1","pullRequest"],"message":"Could not resolve to a PullRequest with the number of 9."},
			{"type":"NOT_FOUND","path":["pr2"],"message":"Could not resolve to a Repository with the name 'acme/gone'."}]}`)

	refs := []Ref{
		{Repo: "acme/api", Number: 1},
		{Repo: "acme/api", Number: 9},
		{Repo: "acme/gone", Number: 2},
		{Repo: "acme/web", Number: 3},
	}
	got, err := client.PullRequestStates(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"OPEN", "", "", "MERGED"}; !slices.Equal(got, want) {
		t.Errorf("PullRequestStates() = %q, want %q", got, want)
	}

	vars := fake.requests[0].Variables
	if vars["owner3"] != "acme" || vars["name3"] != "web" || vars["number3"] != float64(3) {
		t.Errorf("variables = %v", vars)
	}
}

func TestPullRequestStatesNoRefs(t *testing.T) {
	client, fake := fakeClient(t)
	got, err := client.PullRequestStates(context.Background(), nil)
	if err != nil || got != nil {
		t.Errorf("PullRequestStates(nil) = %v, %v; want nil, nil", got, err)
	}
	if len(fake.requests) != 0 {
		t.Errorf("made %d requests, want 0", len(fake.requests))
	}
}

func TestPullRequestStatesErrors(t *testing.T) {
	t.Run("GraphQL errors other than NOT_FOUND", func(t *testing.T) {
		client, _ := fakeClient(t, `{
			"data":{"pr0":{"pullRequest":null},"pr1":null},
			"errors":[
				{"type":"NOT_FOUND","path":["pr0","pullRequest"],"message":"not found"},
				{"type":"FORBIDDEN","path":["pr1"],"message":"Resource not accessible by integration"}]}`)

		_, err := client.PullRequestStates(context.Background(), []Ref{
			{Repo: "acme/api", Number: 1}, {Repo: "acme/secret", Number: 2},
		})
		if err == nil || !strings.Contains(err.Error(), "Resource not accessible") {
			t.Errorf("PullRequestStates() error = %v, want the FORBIDDEN error", err)
		}
	})

	t.Run("HTTP error", func(t *testing.T) {
		client, fake := fakeClient(t, `{"message":"Bad credentials"}`)
		fake.status = http.StatusUnauthorized

		_, err := client.PullRequestStates(context.Background(), []Ref{{Repo: "acme/api", Number: 1}})
		var httpErr *api.HTTPError
		if err == nil || !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			t.Errorf("PullRequestStates() error = %v, want a 401 HTTPError", err)
		}
	})
}

func TestSearchPullRequests(t *testing.T) {
	client, fake := fakeClient(t,
		`{"data":{"search":{
			"pageInfo":{"hasNextPage":true,"endCursor":"c1"},
			"nodes":[
				{"number":1,"title":"Add retries","url":"https://github.com/acme/api/pull/1",
				 "isDraft":false,"updatedAt":"2026-10-01T12:00:00Z",
				 "author":{"login":"dave"},"repository":{"nameWithOwner":"acme/api"},
				 "reviewRequests":{"nodes":[{"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				 "timelineItems":{"nodes":[
					{"__typename":"ReviewRequestedEvent","createdAt":"2026-09-30T08:00:00Z",
					 "requestedReviewer":{"login":"me"}},
					{"__typename":"ReviewRequestRemovedEvent","createdAt":"2026-09-30T09:00:00Z",
					 "requestedReviewer":{"login":"me"}}]},
				 "latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":{"login":"alice"}}]},
				 "viewerDecisions":{"nodes":[]}},
				{}]}}}`,
		`{"data":{"search":{
			"pageInfo":{"hasNextPage":false,"endCursor":"c2"},
			"nodes":[{"number":2,"url":"https://github.com/acme/web/pull/2","repository":{"nameWithOwner":"acme/web"}}]}}}`,
	)

	got, err := client.SearchPullRequests(context.Background(), "is:pr is:open", "me")
	if err != nil {
		t.Fatal(err)
	}
	// The empty node is an issue (or an inaccessible PR) and is skipped.
	if len(got) != 2 {
		t.Fatalf("SearchPullRequests() returned %d PRs, want 2", len(got))
	}

	pr := got[0]
	if pr.Number != 1 || pr.Title != "Add retries" || pr.Repo() != "acme/api" || pr.AuthorLogin() != "dave" {
		t.Errorf("first PR = %+v", pr)
	}
	if want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC); !pr.UpdatedAt.Equal(want) {
		t.Errorf("UpdatedAt = %v, want %v", pr.UpdatedAt, want)
	}
	if _, ok := pr.UserRequestedAt("me"); ok {
		t.Error("UserRequestedAt(me) = true, want false after the request was removed")
	}
	if _, ok := pr.TeamRequestedAt("acme/payments"); !ok {
		t.Error("TeamRequestedAt(acme/payments) = false, want true")
	}
	if want := []string{"alice"}; !slices.Equal(pr.Approvers(), want) {
		t.Errorf("Approvers() = %v, want %v", pr.Approvers(), want)
	}
	if got[1].Repo() != "acme/web" {
		t.Errorf("second PR repo = %q, want acme/web", got[1].Repo())
	}

	first, second := fake.requests[0].Variables, fake.requests[1].Variables
	if first["q"] != "is:pr is:open" || first["viewer"] != "me" || first["cursor"] != nil {
		t.Errorf("first page variables = %v", first)
	}
	if second["cursor"] != "c1" {
		t.Errorf("second page cursor = %v, want c1", second["cursor"])
	}
}
