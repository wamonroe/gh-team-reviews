package github

import (
	"fmt"
	"regexp"
	"strconv"
)

// Ref identifies a pull request.
type Ref struct {
	Repo   string // "owner/name"
	Number int
}

var (
	refURL   = regexp.MustCompile(`^https?://[^/]+/([^/\s]+/[^/\s]+)/pull/(\d+)(?:[/?#]\S*)?$`)
	refShort = regexp.MustCompile(`^([^/\s#]+/[^/\s#]+)#(\d+)$`)
)

// ParseRef accepts a pull request URL (optionally with a trailing path like
// /files) or OWNER/REPO#NUMBER.
func ParseRef(s string) (Ref, error) {
	for _, re := range []*regexp.Regexp{refURL, refShort} {
		if m := re.FindStringSubmatch(s); m != nil {
			number, err := strconv.Atoi(m[2])
			if err != nil || number < 1 {
				break
			}
			return Ref{Repo: m[1], Number: number}, nil
		}
	}
	return Ref{}, fmt.Errorf("%q isn't a pull request URL or OWNER/REPO#NUMBER", s)
}

func (r Ref) String() string {
	return fmt.Sprintf("%s#%d", r.Repo, r.Number)
}

func (pr PullRequest) Ref() Ref {
	return Ref{Repo: pr.Repo(), Number: pr.Number}
}
