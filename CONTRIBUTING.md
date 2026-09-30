# Contributing

Thanks for helping out. This is a small tool, so the process is light.

## Getting started

```sh
git clone https://github.com/iainen/agent-session-switch
cd agent-session-switch
make check
```

You need Go 1.26 or newer. `make lint` additionally needs
[staticcheck](https://staticcheck.dev/).

## Before you open a pull request

- Run `make check` (gofmt, `go vet`, tests with the race detector). CI runs the same on Linux,
  macOS and Windows.
- Add or update tests for behavior changes. Bug fixes should come with a test that fails without
  the fix.
- Update `README.md` if you change keys, flags, or where data is stored, and add a line to
  `CHANGELOG.md` under *Unreleased*.
- Keep changes focused. Unrelated cleanups belong in a separate pull request.

## Ground rules for the code

- **Tests never touch real data.** Use `t.TempDir()` as the home directory. Nothing in this project
  may read or write the developer's real `~/.claude` or `~/.codex` during tests.
- **Deleting is recoverable.** Code that removes a session must move it to the trash. Only
  `trash.Store.Purge` deletes for good, and only for sessions already in the trash.
- **Never overwrite user data silently.** Restoring refuses to replace an existing file, and a
  favorites file that cannot be parsed is left untouched.
- **Agent file formats are not ours.** Parse defensively: skip lines you do not understand rather
  than failing, and keep parsers for each agent in their own file under `internal/session`.

## Layout

| Package | Responsibility |
| --- | --- |
| `internal/session` | Finding and reading Claude and Codex sessions; no writes |
| `internal/trash` | Moving sessions to and from the trash |
| `internal/favorites` | The favorites file |
| `internal/tui` | The Bubble Tea interface; talks to the packages above through `Options` |
| `cmd/agent-session-switch` | Flags, wiring, and starting the agent |

## Adding another agent

1. Add a constant to `session.Agent` (before `NumAgents`) and cases in `String`, `Bin` and
   `ResumeArgs`.
2. Write `internal/session/<agent>.go` with a loader, a preview reader and a `Rescan` case,
   following `claude.go` and `codex.go`.
3. Add the agent's colour to `internal/tui/style.go` and `Load` to `session.Load`.
4. Add tests next to the existing ones; helpers for writing fixture files are in
   `helpers_test.go`.

## Commit messages

Short imperative subject, for example `Fix restore when the project directory was removed`. The
release notes are generated from commit subjects; prefix with `feat:` or `fix:` if you want them
grouped.

## Releasing

Maintainers tag a version, for example `git tag v0.1.0 && git push --tags`. The release workflow
builds binaries for Linux, macOS and Windows with GoReleaser and attaches them to a GitHub release.
