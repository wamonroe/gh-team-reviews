# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-10-27

### Added

- List open, non-draft PRs that still need your review: PRs where you're
  directly requested, plus PRs requested of your team (now, or at any point)
  that have fewer than the required number of team approvals.
- Keep a PR listed until you approve it or request changes. Comment-only
  reviews don't remove it, and a re-request after you've approved brings it
  back.
- `dismiss` and `undismiss` commands to hide PRs you don't need to review until
  you or your team are requested again. Dismissals are stored locally, and
  `--all` shows dismissed PRs with a DISMISSED column.
- Config file at `~/.config/gh-team-reviews/config.yml` (honoring
  `XDG_CONFIG_HOME`) for `team` and `required`, overridable with `--team` and
  `--required`.
- `--json` output, and tab-separated output when piped.
- Precompiled binaries for macOS, Linux, FreeBSD, and Windows.

[Unreleased]: https://github.com/wamonroe/gh-team-reviews/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/wamonroe/gh-team-reviews/releases/tag/v1.0.0
