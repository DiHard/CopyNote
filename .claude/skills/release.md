---
name: release
description: Use this skill whenever the user wants to ship a new CopyNote version — phrases like "release v1.2.0", "выложи релиз", "выпусти новую версию", "make a release", "ship a new build", "cut vX.Y.Z". Performs the full release flow: pre-flight checks, version bump, release notes, build, commit, tag, push, and GitHub Release with the binary attached. Do NOT trigger for plain "build the binary" or "run tests" — those are part of, but not equal to, a release.
---

# CopyNote release process

Goal: produce a tagged GitHub Release with the binary attached, and leave the local repo in a state where the source matches what was published.

Communicate with the user in **Russian** (per `CLAUDE.md` Language Rule). Code and commit messages — see per-step rules below.

## Step 0 — Confirm with the user before doing anything

Ask:
1. **What's the new version?** Or, if not stated: read the current `internal/version/version.go`, list user-visible changes since the last tag (`git log <last-tag>..HEAD --oneline`), and propose PATCH / MINOR / MAJOR per SemVer:
   - PATCH (`X.Y.Z+1`) — bug fixes only
   - MINOR (`X.Y+1.0`) — new features, backwards-compatible
   - MAJOR (`X+1.0.0`) — breaking changes (incompatible settings schema, removed features, changed UX)
2. **Draft of the release notes.** Show the proposed file content and wait for approval.

Do not proceed past Step 0 without explicit user confirmation of both the version and the notes.

## Step 1 — Pre-flight (block on any failure)

Run these in parallel where possible. Stop and ask the user if any check fails — never silently fix or skip.

- `git status` — working tree must be clean (no unstaged or untracked files except the artifacts this skill is about to create: `release-notes-vX.Y.Z.md`, the rebuilt `web/dist/index.html`, and the version bump). If something else is dirty, halt.
- `git rev-parse --abbrev-ref HEAD` — must be `main`.
- `git fetch && git status` — must be in sync with `origin/main` (no commits ahead or behind that the user didn't intend).
- `git tag -l vX.Y.Z` — must be empty (no local tag yet).
- `gh release view vX.Y.Z` — must error with "release not found" (no published release yet). If a tag/release already exists for this version, halt and ask whether to bump again.
- `go test ./...` — must pass.
  - Known: `TestCopy_EmptyValueIsAllowed` is broken on `main` independent of this skill. If it's the only failure, surface it and ask whether to proceed.
- `gh auth status` — must be authenticated.

## Step 2 — Bump version

Edit **only** `internal/version/version.go`:
```go
var Version = "X.Y.Z"  // no leading "v", no other quotes changed
```

Do NOT edit:
- `web/package.json` version (stays at `0.0.0` — never published to npm)
- `model.SchemaVersion` in `internal/model/entry.go` — that's the on-disk JSON format version, unrelated to the app version. Bump it only when `data.json` shape actually changes.
- Any other place — `version.Version` is the single source of truth (see CLAUDE.md §Versioning).

## Step 3 — Write release notes

Create `release-notes-vX.Y.Z.md` at repo root. Format (omit empty sections):

```markdown
## CopyNote vX.Y.Z

### New features
- ...

### Bug fixes
- ...

### Internal
- ...
```

Language: **English**. The audience-facing changelog matches the existing files (`release-notes-v1.0.0.md`, `release-notes-v1.0.1.md`) and ships to GitHub Releases for an international audience. This is the single exception to the Russian-by-default rule, and it's grounded in the existing precedent.

For the first release of a major or minor (e.g. `v2.0.0`, `v1.5.0`), consider a one-line tagline like `v1.0.0 — Initial Release` for the GitHub Release title (Step 7).

## Step 4 — Build

```bash
cd web && npm install && npm run build && cd ..
go build -ldflags="-H=windowsgui -s -w" -o copynote.exe .
```

Verify:
- `web/dist/index.html` updated (it's committed because `//go:embed` bakes it in).
- `copynote.exe` exists at repo root (~7 MB).
- `ls -la copynote.exe` shows recent timestamp.

If icons changed since the last release, also rebuild them per `CLAUDE.md` §Build Commands. Skip otherwise — they almost never change.

## Step 5 — Commit (Russian)

Stage exactly these files:
- `internal/version/version.go`
- `release-notes-vX.Y.Z.md`
- `web/dist/index.html`

Do NOT stage `copynote.exe` (it's `*.exe` in `.gitignore` and will be attached to the GitHub Release in Step 7, not tracked in git).

Commit message (Russian, per project rules):
```
chore: релиз vX.Y.Z

<1–3 строки на русском, что в этом релизе. Для патча — одна строка.>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```

Pass via HEREDOC (see CLAUDE.md style and the global Bash-tool rules).

## Step 6 — Tag and push

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push && git push --tags
```

Annotated tag (`-a`), not lightweight — GitHub Releases need annotated tags for full metadata.

## Step 7 — Create GitHub Release with the binary

```bash
gh release create vX.Y.Z copynote.exe \
  --title "vX.Y.Z" \
  --notes-file release-notes-vX.Y.Z.md
```

For an "initial-of-major-minor" with a tagline, use `--title "vX.Y.Z — <tagline>"` instead.

## Step 8 — Verify and report

- `gh release view vX.Y.Z` — confirm page exists, `copynote.exe` is attached, notes render correctly.
- Output the release URL to the user: `https://github.com/DiHard/CopyNote/releases/tag/vX.Y.Z`.
- One-sentence end-of-turn summary (Russian): что выпустили и куда выложено.

## Recovery — if something went wrong

- **Wrong content shipped in `vX.Y.Z`**: do NOT delete the published GitHub Release (users may already have downloaded). Bump to `vX.Y.Z+1` and ship a corrected one. Add an `### Internal` line in the new notes pointing back at the issue.
- **Tag pushed but `gh release create` failed**: rerun `gh release create vX.Y.Z copynote.exe --title ... --notes-file ...` against the existing tag.
- **Tag wrong locally only (not pushed)**: `git tag -d vX.Y.Z` and redo from Step 6. Pushed tag — see next.
- **Tag pushed but release not yet created and you want to redo**: ask the user before `git push --delete origin vX.Y.Z`. Force-deleting a tag is a shared-state action; explicit confirmation required.

## Why this skill exists

Releases were previously made ad-hoc — `v1.1.0` shipped on GitHub but `version.go` and the local repo were not updated, leaving state diverged from what users see in the About panel of the released binary. This skill enforces a single deterministic path so the local repo, the GitHub tag, and the binary About string always agree.
