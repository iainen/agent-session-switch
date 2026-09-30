// Command agent-session-switch lists Claude Code and Codex sessions from any
// directory, then changes to the chosen session's directory and resumes it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"

	"github.com/iainen/agent-session-switch/internal/favorites"
	"github.com/iainen/agent-session-switch/internal/session"
	"github.com/iainen/agent-session-switch/internal/trash"
	"github.com/iainen/agent-session-switch/internal/tui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: agent-session-switch [flags] [filter] [-- agent-args...]

Pick a Claude Code or Codex session, then jump to its directory and resume it.

Flags:
`

type cliArgs struct {
	limit       int
	query       string
	extra       []string // arguments after "--", passed to the agent
	showVersion bool
}

// parseArgs parses the command line. Everything after "--" is kept verbatim
// for the agent, which the flag package would otherwise swallow.
func parseArgs(args []string, out io.Writer) (cliArgs, error) {
	var c cliArgs
	for i, a := range args {
		if a == "--" {
			args, c.extra = args[:i], args[i+1:]
			break
		}
	}
	fs := flag.NewFlagSet("agent-session-switch", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() {
		fmt.Fprint(out, usage)
		fs.PrintDefaults()
	}
	fs.IntVar(&c.limit, "n", 300, "max number of recent sessions to load per agent")
	fs.BoolVar(&c.showVersion, "version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if c.limit < 1 {
		return c, errors.New("-n must be at least 1")
	}
	if fs.NArg() > 1 {
		return c, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args()[1:], " "))
	}
	c.query = fs.Arg(0)
	return c, nil
}

func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	c, err := parseArgs(args, stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return 0
	case err != nil:
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if c.showVersion {
		fmt.Fprintln(stdout, "agent-session-switch", versionString())
		return 0
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	opts := tui.Options{Home: home, Trash: trash.New(home), Favs: favorites.Open(home), Query: c.query}
	total := 0
	for a := session.Agent(0); a < session.NumAgents; a++ {
		opts.Sessions[a] = session.Load(home, a, c.limit)
		total += len(opts.Sessions[a])
	}
	for _, t := range opts.Trash.List() {
		opts.Trashed[t.Agent] = append(opts.Trashed[t.Agent], t)
		total++
	}
	if total == 0 {
		fmt.Fprintln(stderr, "没有找到会话")
		return 1
	}

	chosen, err := tui.Run(opts)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if chosen == nil {
		return 0
	}
	if err := resume(chosen, c.extra, stderr); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// resume changes to the session's directory and starts the agent there.
func resume(s *session.Session, extra []string, stderr io.Writer) error {
	if err := os.Chdir(s.Cwd); err != nil {
		return fmt.Errorf("切换目录失败: %w", err)
	}
	bin := s.Agent.Bin()
	path, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("找不到 %s 命令", bin)
	}
	argv := append(append([]string{bin}, s.Agent.ResumeArgs(s.ID)...), extra...)
	fmt.Fprintf(stderr, "→ cd %s && %s\n", s.Cwd, strings.Join(argv, " "))
	return execAgent(path, argv)
}
