package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/wamonroe/gh-team-reviews/internal/github"
	"github.com/wamonroe/gh-team-reviews/internal/review"
)

var now = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

type fakeTerminal struct {
	out, errOut bytes.Buffer
	tty         bool
	width       int
}

func (t *fakeTerminal) Out() io.Writer         { return &t.out }
func (t *fakeTerminal) ErrOut() io.Writer      { return &t.errOut }
func (t *fakeTerminal) IsTerminalOutput() bool { return t.tty }
func (t *fakeTerminal) Size() (int, int, error) {
	if t.width == 0 {
		return 0, 0, errors.New("not a terminal")
	}
	return t.width, 24, nil
}

// pr builds a PullRequest from the same JSON shape the GraphQL API returns.
func pr(t *testing.T, raw string) github.PullRequest {
	t.Helper()
	var p github.PullRequest
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func entries(t *testing.T) []review.Entry {
	members := []string{"me", "alice", "bob"}
	return []review.Entry{
		{
			PR: pr(t, `{"number":12,"title":"Retry failed webhook deliveries with exponential backoff",
				"url":"https://github.com/acme/api/pull/12","updatedAt":"2026-10-06T11:15:00Z",
				"author":{"login":"dave"},"repository":{"nameWithOwner":"acme/api"},
				"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]}}`),
			Viewer: "me", Team: "acme/payments", Members: members, Required: 2,
		},
		{
			PR: pr(t, `{"number":345,"title":"Remove legacy 決済 flow and dead code","url":"https://github.com/other/web/pull/345",
				"updatedAt":"2026-10-03T09:00:00Z",
				"author":{"login":"erin"},"repository":{"nameWithOwner":"other/web"},
				"timelineItems":{"nodes":[{"__typename":"ReviewRequestedEvent","createdAt":"2026-10-01T00:00:00Z",
					"requestedReviewer":{"combinedSlug":"acme/payments"}}]},
				"latestOpinionatedReviews":{"nodes":[{"state":"APPROVED","author":{"login":"alice"}}]}}`),
			Viewer: "me", Team: "acme/payments", Members: members, Required: 2,
			DismissedAt: time.Date(2026, 10, 6, 7, 0, 0, 0, time.UTC),
		},
	}
}

func TestTableTTY(t *testing.T) {
	term := &fakeTerminal{tty: true, width: 80}
	if err := Table(term, entries(t), "acme/payments", true, now); err != nil {
		t.Fatal(err)
	}

	want := "PR             REVIEWS  FOR   UPDATED  DISMISSED  AUTHOR  TITLE\n" +
		link("https://github.com/acme/api/pull/12", "api#12") + "         0/1      you   45m                 dave    Retry failed webhoo...\n" +
		link("https://github.com/other/web/pull/345", "other/web#345") + "  1/2      team  3d       5h         erin    Remove legacy 決済 ...\n"
	if got := term.out.String(); got != want {
		t.Errorf("Table() =\n%s\nwant\n%s\n(%q)", got, want, got)
	}
}

func TestTablePiped(t *testing.T) {
	term := &fakeTerminal{}
	if err := Table(term, entries(t), "", false, now); err != nil {
		t.Fatal(err)
	}

	want := "acme/api#12\t45m\tdave\tRetry failed webhook deliveries with exponential backoff\n" +
		"other/web#345\t3d\terin\tRemove legacy 決済 flow and dead code\n"
	if got := term.out.String(); got != want {
		t.Errorf("Table() =\n%s\nwant\n%s\n(%q)", got, want, got)
	}
}

func TestTableEmpty(t *testing.T) {
	tty := &fakeTerminal{tty: true, width: 80}
	if err := Table(tty, nil, "", false, now); err != nil {
		t.Fatal(err)
	}
	if tty.out.String() != "" || tty.errOut.String() != "No PRs need your review.\n" {
		t.Errorf("Table() on a TTY wrote out=%q err=%q", tty.out.String(), tty.errOut.String())
	}

	piped := &fakeTerminal{}
	if err := Table(piped, nil, "", false, now); err != nil {
		t.Fatal(err)
	}
	if piped.out.String() != "" || piped.errOut.String() != "" {
		t.Errorf("Table() piped wrote out=%q err=%q", piped.out.String(), piped.errOut.String())
	}
}

func TestJSON(t *testing.T) {
	var out bytes.Buffer
	if err := JSON(&out, entries(t)); err != nil {
		t.Fatal(err)
	}

	want := `[
  {
    "url": "https://github.com/acme/api/pull/12",
    "repo": "acme/api",
    "number": 12,
    "title": "Retry failed webhook deliveries with exponential backoff",
    "author": "dave",
    "updated_at": "2026-10-06T11:15:00Z",
    "reason": "you",
    "approvals": [],
    "required": 1
  },
  {
    "url": "https://github.com/other/web/pull/345",
    "repo": "other/web",
    "number": 345,
    "title": "Remove legacy 決済 flow and dead code",
    "author": "erin",
    "updated_at": "2026-10-03T09:00:00Z",
    "reason": "team",
    "approvals": [
      "alice"
    ],
    "required": 2,
    "dismissed_at": "2026-10-06T07:00:00Z"
  }
]
`
	if got := out.String(); got != want {
		t.Errorf("JSON() =\n%s\nwant\n%s", got, want)
	}
}

// link is cell text wrapped in an OSC 8 terminal hyperlink.
func link(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
