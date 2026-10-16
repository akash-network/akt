# Changelog fragments

Each ordinary PR adds its own Markdown file here. Separate files let concurrent
PRs record changes without editing the same part of `AICHANGELOG.md`.

Use `<unique-slug>.<category>.md`. The slug can be a PR number or a descriptive
name containing lowercase letters, digits, and hyphens. Categories are
`added`, `changed`, `fixed`, `deprecated`, `removed`, and `security`.

For example, `.changelog/104.added.md` could contain:

```markdown
- Package the akt CLI agent skill with releases and document installation.
  Validate the skill's command examples against the built binary.
```

Write Markdown bullets describing what changed and how the issue was handled.
Multiple related bullets can share a fragment. Omit section headings; the
assembler adds them. Do not reuse a filename from a previous release or edit
another PR's fragment. Every PR needs an entry, including documentation and
tooling changes.

Validate from the repository root:

```bash
# Validate all pending fragments.
GOWORK=off make changelog-check

# Also check this PR's changes against its base branch.
GOWORK=off make changelog-check CHANGELOG_BASE=origin/main
```

CI uses the pull request's base commit. It requires a new fragment and rejects
direct archive edits or changes to existing base-branch fragments. A release
assembly has a separate, exact-content check rather than a label-based bypass.

## Preparing a release

From a clean checkout with an authenticated `gh` and a git signing key, run:

```bash
GOWORK=off make tag-release                 # prompts for the tag
GOWORK=off make tag-release VERSION=v1.2.3  # no prompt
```

The target stops unless `gh auth status` succeeds inside it. If your `gh`
is a shell function, as with the 1Password shell plugin, the script cannot
see it; pass the token instead:

```bash
GH_TOKEN=$(gh auth token) GOWORK=off make tag-release VERSION=v1.2.3
```

It branches `release/<tag>` from `origin/main`, runs the assembly and checks
below, commits `chore(release): prepare <tag>`, and opens the PR. Once CI is
green and core has approved and merged the PR, it signs the tag on the merge
commit and pushes it, which starts the release workflow. Interrupting the
wait is safe: rerun with the same tag to resume. If `origin/main` has no
pending fragments, it tags the `origin/main` tip without a PR.

The manual equivalent, on a release-preparation branch based on current
`main`, is:

```bash
GOWORK=off make changelog-assemble
GOWORK=off make changelog-check CHANGELOG_BASE=origin/main
GOWORK=off make changelog-release-check
```

The assembler groups fragments by category, sorts them by filename, inserts
them under `## Unreleased`, and removes the consumed files. Existing archive
contents are preserved, including historical headings. Source filenames are
recorded in HTML comments so a retry after interrupted cleanup cannot duplicate
entries. `README.md` stays in this directory.

Review and commit `AICHANGELOG.md` together with the fragment deletions in a
PR containing only those changes. This generated PR needs no additional
fragment. Merge it before tagging the tested release commit. If more fragments
merge before tagging, assemble those in another preparation PR first. The
tagged release quality check and publication preflight reject pending fragments.
Manual snapshots and dry runs may retain them.

The changelog commands do not commit, tag, or publish. `make tag-release`
commits, opens the PR, and pushes the tag; the release workflow publishes.
GitHub release notes continue to come from GoReleaser's commit-based
configuration.

## Migrating an open PR

Move your PR's additions from `AICHANGELOG.md` into new fragments. Restore
the archive to the base branch's version, retaining other contributors'
history, and run the PR check above. Historical plans that mention writing
directly to `AICHANGELOG.md` predate this workflow; use fragments for new work.
