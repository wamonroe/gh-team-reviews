// gh team-reviews lists open PRs that still need your review. Unlike GitHub's
// `review-requested:@me`, team-requested PRs stay on the list until enough
// members of the team have approved them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/term"

	"github.com/wamonroe/gh-team-reviews/internal/config"
	"github.com/wamonroe/gh-team-reviews/internal/dismissed"
	"github.com/wamonroe/gh-team-reviews/internal/github"
	"github.com/wamonroe/gh-team-reviews/internal/output"
	"github.com/wamonroe/gh-team-reviews/internal/review"
)

// version can be set with -ldflags "-X main.version=..."; otherwise it comes
// from the git tag Go stamps into the build.
var version = ""

func init() {
	if version != "" {
		return
	}
	version = "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "gh team-reviews: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	configPath := config.DefaultPath()

	if len(args) > 0 {
		switch args[0] {
		case "dismiss":
			return runDismiss(configPath, args[1:])
		case "undismiss":
			return runUndismiss(configPath, args[1:])
		}
	}

	flags := flag.NewFlagSet("gh team-reviews", flag.ContinueOnError)
	team := flags.String("team", "", `team to track as ORG/TEAM; overrides the config file (use --team="" to disable)`)
	required := flags.Int("required", 0, "team approvals needed before a PR drops off; overrides the config file")
	all := flags.Bool("all", false, "include PRs you've dismissed, marked in a DISMISSED column")
	asJSON := flags.Bool("json", false, "output JSON")
	showVersion := flags.Bool("version", false, "print the version")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), `Usage: gh team-reviews [flags]
       gh team-reviews dismiss <pr>...
       gh team-reviews undismiss <pr>...

Lists open PRs that still need your review: PRs where you're directly
requested, plus PRs requested of your team that have fewer than the required
number of team approvals. A PR stays listed until you approve it or request
changes; comment-only reviews don't remove it.

Without a team (from --team or the config file) only direct requests are listed.

Dismiss a PR to hide it until you or your team are requested to review again.
A PR is a URL or OWNER/REPO#NUMBER.

Config file: %s
  team: org/team-slug
  required: 2

Flags:
`, configPath)
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		flags.Usage()
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "team":
			cfg.Team = *team
		case "required":
			cfg.Required = *required
		}
	})
	if err := cfg.Validate(); err != nil {
		return err
	}

	list, err := dismissed.Load(dismissed.Path(configPath))
	if err != nil {
		return err
	}

	client, err := github.NewClient()
	if err != nil {
		return err
	}
	entries, err := review.Build(context.Background(), client, review.Options{
		Team:             cfg.Team,
		Required:         cfg.Required,
		Dismissed:        list,
		IncludeDismissed: *all,
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(os.Stdout, entries)
	}
	return output.Table(term.FromEnv(), entries, cfg.Team, *all, time.Now())
}

// runDismiss adds PRs to the dismissed list. Every PR already on the list is
// looked up too, so closed and merged ones can be pruned.
func runDismiss(configPath string, args []string) error {
	refs, err := parseRefs("dismiss", "Hides PRs from your list until you or your team are requested to review again.", args)
	if err != nil {
		return err
	}

	path := dismissed.Path(configPath)
	list, err := dismissed.Load(path)
	if err != nil {
		return err
	}
	client, err := github.NewClient()
	if err != nil {
		return err
	}
	existing := list.Refs()
	states, err := client.PullRequestStates(context.Background(), append(slices.Clone(refs), existing...))
	if err != nil {
		return err
	}

	for i, ref := range refs {
		if states[i] == "" {
			return fmt.Errorf("%s not found (or you can't see it)", ref)
		}
	}
	for i, ref := range existing {
		if states[len(refs)+i] != "OPEN" {
			list.Remove(ref)
		}
	}
	now := time.Now()
	for i, ref := range refs {
		if states[i] != "OPEN" {
			fmt.Fprintf(os.Stderr, "Skipped %s: already %s\n", ref, strings.ToLower(states[i]))
			continue
		}
		list.Add(ref, now)
		fmt.Printf("Dismissed %s\n", ref)
	}
	return list.Save(path)
}

func runUndismiss(configPath string, args []string) error {
	refs, err := parseRefs("undismiss", "Puts dismissed PRs back on your list.", args)
	if err != nil {
		return err
	}

	path := dismissed.Path(configPath)
	list, err := dismissed.Load(path)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if list.Remove(ref) {
			fmt.Printf("Undismissed %s\n", ref)
		} else {
			fmt.Fprintf(os.Stderr, "%s wasn't dismissed\n", ref)
		}
	}
	return list.Save(path)
}

// parseRefs parses a subcommand's arguments as one or more PRs.
func parseRefs(name, summary string, args []string) ([]github.Ref, error) {
	flags := flag.NewFlagSet("gh team-reviews "+name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), `Usage: gh team-reviews %s <pr>...

%s
A PR is a URL or OWNER/REPO#NUMBER.
`, name, summary)
	}
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return nil, errors.New("no pull requests given")
	}

	refs := make([]github.Ref, flags.NArg())
	for i, arg := range flags.Args() {
		ref, err := github.ParseRef(arg)
		if err != nil {
			return nil, err
		}
		refs[i] = ref
	}
	return refs, nil
}
