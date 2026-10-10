// awo switches the AWS SSO profile of a whole tmux window.
//
// The choice lives in the window option @aws_profile; the zsh hook printed by
// `awo init zsh` exports it as AWS_PROFILE in every pane at its next prompt.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sdgluck/awo/internal/awsconf"
	"github.com/sdgluck/awo/internal/tmux"
	"github.com/sdgluck/awo/internal/ui"
)

const usage = `usage: awo [command]

  (none)                      pick a profile for the current window
  pick [--window @N]          same, for a given window
  popup @N [client_tty]       open the picker in a tmux popup sized to fit
  set <profile> [--window @N] set the window's profile
  clear [--window @N]         clear the window's profile
  current [--window @N]       print the window's profile
  list                        print configured SSO profiles
  init zsh                    print the shell hook
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "awo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "pick"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "pick":
		win, _, err := window(args)
		if err != nil {
			return err
		}
		profiles, err := profiles()
		if err != nil {
			return err
		}
		_, err = tea.NewProgram(ui.New(win, profiles)).Run()
		return err

	case "popup":
		if len(args) < 1 {
			return errors.New("popup: need a window id")
		}
		client := ""
		if len(args) > 1 {
			client = args[1]
		}
		return popup(args[0], client)

	case "set":
		win, rest, err := window(args)
		if err != nil {
			return err
		}
		if len(rest) != 1 {
			return errors.New("set: need exactly one profile")
		}
		ps, err := profiles()
		if err != nil {
			return err
		}
		for _, p := range ps {
			if p.Name == rest[0] {
				return tmux.Set(win, p.Name)
			}
		}
		return fmt.Errorf("set: no SSO profile %q in %s", rest[0], awsconf.Path())

	case "clear":
		win, _, err := window(args)
		if err != nil {
			return err
		}
		return tmux.Unset(win)

	case "current":
		win, _, err := window(args)
		if err != nil {
			return err
		}
		if p := tmux.Get(win); p != "" {
			fmt.Println(p)
		}
		return nil

	case "list":
		ps, err := profiles()
		if err != nil {
			return err
		}
		for _, p := range ps {
			fmt.Println(p.Name)
		}
		return nil

	case "init":
		if len(args) != 1 || args[0] != "zsh" {
			return errors.New("init: only zsh is supported")
		}
		fmt.Print(zshHook)
		return nil

	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	}
	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

// window parses --window, defaulting to the window of $TMUX_PANE.
func window(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("awo", flag.ContinueOnError)
	win := fs.String("window", "", "tmux window id (@N)")
	// Allow the flag after positional args: `awo set fink-stg --window @3`.
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return "", nil, err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	if *win != "" {
		return *win, pos, nil
	}
	w, err := tmux.CurrentWindow()
	return w, pos, err
}

func profiles() ([]awsconf.Profile, error) {
	ps, err := awsconf.Load(awsconf.Path())
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("no SSO profiles in %s", awsconf.Path())
	}
	return ps, nil
}

// popup sizes the popup to fit the picker (see ui.Model.Size) plus the border.
func popup(win, client string) error {
	ps, err := profiles()
	if err != nil {
		return err
	}
	w, h := ui.New(win, ps).Size()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := shellQuote(self) + " pick --window " + shellQuote(win)
	return tmux.Popup(client, min(w+2, 120), h+2, " awo ", cmd)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

const zshHook = `# awo: follow the tmux window's @aws_profile. Synced in preexec too, so the
# first command typed at an already-drawn prompt sees a switch made from the
# popup. The stamp file changes whenever
# any window's profile does; read is a builtin, so an unchanged stamp costs no
# fork. Only a change of the window's value is applied, so a manual
# export AWS_PROFILE=... in one pane holds until the window is switched again.
typeset -g _awo_stamp="${XDG_CACHE_HOME:-$HOME/.cache}/awo/stamp" _awo_seen=- _awo_applied=
_awo_sync() {
  [[ -n $TMUX_PANE ]] || return 0
  local s=
  [[ -r $_awo_stamp ]] && read -r s < $_awo_stamp
  [[ $s == $_awo_seen ]] && return 0
  _awo_seen=$s
  local p
  p=$(tmux display -p -t "$TMUX_PANE" '#{@aws_profile}' 2>/dev/null) || return 0
  [[ $p == $_awo_applied ]] && return 0
  if [[ -n $p ]]; then
    unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN \
      AWS_SESSION_EXPIRATION AWS_CREDENTIAL_EXPIRATION AWS_REGION AWS_DEFAULT_REGION \
      GRANTED_AWS_ROLE_PROFILE GRANTED_SSO
    export AWS_PROFILE=$p
  elif [[ -n $_awo_applied ]]; then
    unset AWS_PROFILE
  fi
  _awo_applied=$p
}
autoload -Uz add-zsh-hook
add-zsh-hook precmd _awo_sync
add-zsh-hook preexec _awo_sync
`
