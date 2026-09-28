# gh team-reviews

A [`gh`](https://cli.github.com) extension that lists open pull requests still
waiting on your review — without losing track of team review requests.

GitHub's review requests page (`review-requested:@me`) drops a team-requested PR
as soon as _any one_ member of the team reviews it. That's a problem when a
team wants two or more of its members to review. `gh team-reviews` keeps those
PRs on your list until enough of your team has approved them.

```
$ gh team-reviews
PR               REVIEWS  FOR   UPDATED  AUTHOR       TITLE
storefront#412   1/2      team  1h       octo-alice   feat: add saved cards to checkout
storefront#409   0/2      team  3h       octo-bob     fix: retry failed webhook deliveries
billing-api#88   1/2      team  1d       octo-carol   refactor: extract invoice totals calculator
dashboard#1057   0/1      you   2d       octo-dave    chore: upgrade charting library
```

## Install

```sh
gh extension install wamonroe/gh-team-reviews
```

Or, from a clone of this repository:

```sh
go build -o gh-team-reviews .
gh extension install .
```

A local install is a symlink to the clone, so rebuilding updates it in place.

## Configure

Create `~/.config/gh-team-reviews/config.yml` (or under `$XDG_CONFIG_HOME`):

```yaml
# The team whose review requests you want to track, as org/team-slug.
team: acme/payments

# Team approvals needed before a PR drops off your list. Defaults to 2.
required: 2
```

Flags override the config file:

```sh
gh team-reviews --team my-org/my-team   # track a different team
gh team-reviews --required 3            # need three team approvals
gh team-reviews --team=""               # ignore the configured team
gh team-reviews --json                  # machine-readable output
```

Without a team, only PRs where you are directly requested are listed.

## What's listed

A PR is listed when it's open, not a draft, not yours, and either:

- **you** are directly requested as a reviewer, or
- your **team** was requested (now, or at any point — GitHub removes the team
  request once someone reviews), fewer than `required` team members have
  approved it, and you haven't reviewed it yet.

The REVIEWS column shows approvals against what the PR is waiting on:

- **team** rows count team members whose latest review is an approval, against
  `required` (e.g. `1/2`). Approvals from outside the team don't count.
- **you** rows are always `0/1`: a direct request waits on your review alone,
  whatever anyone else has done.

When output is piped, the table becomes tab-separated with no header.

## Development

This project uses [devbox](https://www.jetify.com/devbox) and
[direnv](https://direnv.net) for its toolchain.

```sh
direnv allow
devbox run test      # go vet + go test
devbox run install   # build and install as a local gh extension
```

Releases are built by the `release` workflow when a `v*` tag is pushed.
