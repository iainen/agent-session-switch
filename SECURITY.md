# Security policy

## Reporting a vulnerability

Please report security issues privately through GitHub's
["Report a vulnerability"](https://github.com/iainen/agent-session-switch/security/advisories/new)
form rather than a public issue. Expect a first reply within a week.

## What this tool touches

The tool reads coding-agent session files under `~/.claude` and `~/.codex`, and writes only
`~/.claude/session-favorites.json` and `~/.claude/session-trash/`. It makes no network requests.
Session files can contain sensitive content such as source code and secrets; the tool displays
them in your terminal and never sends them anywhere.

The only process it starts is `claude` or `codex`, looked up on your `PATH`, in the directory
recorded in the session file.
