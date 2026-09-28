package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"

	"github.com/wamonroe/gh-team-reviews/internal/review"
)

type jsonEntry struct {
	URL       string    `json:"url"`
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	Reason    string    `json:"reason"`
	Approvals []string  `json:"approvals"`
	Required  int       `json:"required"`
}

func JSON(w io.Writer, entries []review.Entry) error {
	out := make([]jsonEntry, len(entries))
	for i, e := range entries {
		out[i] = jsonEntry{
			URL:       e.PR.URL,
			Repo:      e.PR.Repo(),
			Number:    e.PR.Number,
			Title:     e.PR.Title,
			Author:    e.PR.AuthorLogin(),
			UpdatedAt: e.PR.UpdatedAt,
			Reason:    e.Reason(),
			Approvals: e.Approvals(),
			Required:  e.RequiredApprovals(),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Table prints entries as a table on a terminal, or as tab-separated values
// without a header when piped. The REVIEWS and FOR columns only appear when a
// team is being tracked.
func Table(t term.Term, entries []review.Entry, team string, now time.Time) error {
	isTTY := t.IsTerminalOutput()
	if len(entries) == 0 {
		if isTTY {
			fmt.Fprintln(t.ErrOut(), "No PRs need your review.")
		}
		return nil
	}

	width, _, err := t.Size()
	if err != nil {
		width = 80
	}
	org, _, _ := strings.Cut(team, "/")

	tp := tableprinter.New(t.Out(), isTTY, width)
	if team != "" {
		tp.AddHeader([]string{"PR", "REVIEWS", "FOR", "UPDATED", "AUTHOR", "TITLE"})
	} else {
		tp.AddHeader([]string{"PR", "UPDATED", "AUTHOR", "TITLE"})
	}

	for _, e := range entries {
		label := e.PR.Repo()
		if org != "" {
			label = strings.TrimPrefix(label, org+"/")
		}
		label = fmt.Sprintf("%s#%d", label, e.PR.Number)

		tp.AddField(label, tableprinter.WithTruncate(nil), tableprinter.WithColor(linkTo(e.PR.URL, isTTY)))
		if team != "" {
			tp.AddField(reviews(e), tableprinter.WithTruncate(nil))
			tp.AddField(e.Reason(), tableprinter.WithTruncate(nil))
		}
		tp.AddField(age(now.Sub(e.PR.UpdatedAt)), tableprinter.WithTruncate(nil))
		tp.AddField(e.PR.AuthorLogin(), tableprinter.WithTruncate(nil))
		tp.AddField(e.PR.Title)
		tp.EndRow()
	}
	return tp.Render()
}

// reviews renders approvals against what the entry is waiting on, e.g. "1/2"
// for a team request or "0/1" for a direct request waiting on the viewer.
func reviews(e review.Entry) string {
	return fmt.Sprintf("%d/%d", len(e.Approvals()), e.RequiredApprovals())
}

func age(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// linkTo wraps a cell in an OSC 8 terminal hyperlink. The table has already
// padded the cell, so trailing spaces are kept outside the link.
func linkTo(url string, isTTY bool) func(string) string {
	return func(cell string) string {
		if !isTTY {
			return cell
		}
		text := strings.TrimRight(cell, " ")
		return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\" + cell[len(text):]
	}
}
