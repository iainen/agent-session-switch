# Changelog

All notable changes to this project are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Browse Claude Code and Codex sessions side by side, with a live preview of
  the selected conversation.
- Vim-style keys: `j`/`k` to move, `h`/`l` to switch agent, `/` to filter.
- Favorites (`f`, `F`) stored in `~/.claude/session-favorites.json`.
- Recoverable trash (`d`, `t`, `r`, `x`) stored in `~/.claude/session-trash/`.
- `-n` to limit how many sessions are loaded and `-version`.
