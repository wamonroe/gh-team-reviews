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
	"time"

	"github.com/cli/go-gh/v2/pkg/term"

	"github.com/wamonroe/gh-team-reviews/internal/config"
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

	flags := flag.NewFlagSet("gh team-reviews", flag.ContinueOnError)
	team := flags.String("team", "", `team to track as ORG/TEAM; overrides the config file (use --team="" to disable)`)
	required := flags.Int("required", 0, "team approvals needed before a PR drops off; overrides the config file")
	asJSON := flags.Bool("json", false, "output JSON")
	showVersion := flags.Bool("version", false, "print the version")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), `Usage: gh team-reviews [flags]

Lists open PRs that still need your review: PRs where you're directly
requested, plus PRs requested of your team that have fewer than the required
number of team approvals and that you haven't reviewed yet.

Without a team (from --team or the config file) only direct requests are listed.

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

	client, err := github.NewClient()
	if err != nil {
		return err
	}
	entries, err := review.Build(context.Background(), client, review.Options{Team: cfg.Team, Required: cfg.Required})
	if err != nil {
		return err
	}

	if *asJSON {
		return output.JSON(os.Stdout, entries)
	}
	return output.Table(term.FromEnv(), entries, cfg.Team, time.Now())
}
