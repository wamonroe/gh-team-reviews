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
devbox run install

# Or, if you have Go installed locally
go build -o gh-team-reviews .
gh extension install .
```

A local install is a symlink to the clone, so rebuilding updates it in place.

## Configure

Create `~/.config/gh-team-reviews/config.yml`. If you've set
[`XDG_CONFIG_HOME`](https://specifications.freedesktop.org/basedir-spec/latest/)
to move your config directory, use `$XDG_CONFIG_HOME/gh-team-reviews/config.yml`
instead.

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
gh team-reviews --all                   # include PRs you've dismissed
```

Without a team, only PRs where you are directly requested are listed.

## What's listed

A PR is listed when it's open, not a draft, not yours, and either:

- **you** are directly requested as a reviewer, or
- your **team** was requested (now, or at any point — GitHub removes the team
  request once someone reviews) and fewer than `required` team members have
  approved it.

Either way, the PR stays listed until you approve it or request changes.
Leaving only comments (to ask a question before you review, say) doesn't take
it off your list, even though GitHub clears your review request when you do.
If the author re-requests your review after you've approved, it comes back.

## Dismissing PRs

When a PR is still on your list but you don't need to review it (say, your
team's files were dropped from it), dismiss it:

```sh
gh team-reviews dismiss https://github.com/acme/storefront/pull/412
gh team-reviews dismiss acme/storefront#412 acme/billing-api#88
gh team-reviews undismiss acme/storefront#412
```

A dismissed PR stays hidden until you or your team are requested to review it
again. New commits or comments don't bring it back. `--all` lists dismissed PRs
too, with a DISMISSED column showing how long ago you dismissed each one.

Dismissals are only stored on your machine, in `dismissed.yml` next to the
config file, so nobody else sees them. Dismissing also clears out entries for
PRs that have since closed or merged.

## Reading the list

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
devbox run build     # build ./gh-team-reviews
devbox run test      # go vet + go test
devbox run install   # build and install as a local gh extension
```

Releases are built by the `release` workflow when a `v*` tag is pushed. Record
changes in [CHANGELOG.md](CHANGELOG.md) before tagging.

## License

[MIT](LICENSE)
