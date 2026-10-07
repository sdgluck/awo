// Package tmux stores the chosen profile as a window user option and bumps a
// stamp file so shells know to re-read it.
package tmux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Option = "@aws_profile"

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", errors.New(strings.TrimSpace(string(ee.Stderr)))
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// CurrentWindow resolves the window of the pane awo was started in.
func CurrentWindow() (string, error) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", errors.New("not inside tmux (TMUX_PANE unset); pass --window")
	}
	return run("display", "-p", "-t", pane, "#{window_id}")
}

func Get(win string) string {
	v, _ := run("show", "-wqv", "-t", win, Option)
	return v
}

func Set(win, profile string) error {
	if _, err := run("set", "-w", "-t", win, Option, profile); err != nil {
		return err
	}
	return touchStamp()
}

func Unset(win string) error {
	if _, err := run("set", "-wu", "-t", win, Option); err != nil {
		return err
	}
	return touchStamp()
}

// Label is "index:name" for display.
func Label(win string) string {
	v, err := run("display", "-p", "-t", win, "#{window_index}:#{window_name}")
	if err != nil {
		return win
	}
	return v
}

// StampPath is read by the zsh hook with the `read` builtin (no fork); a new
// value means some window's profile changed.
func StampPath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "awo", "stamp")
}

func touchStamp() error {
	p := StampPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)+"\n"), 0o644)
}

// Popup opens the shell command cmd in a display-popup on client (may be empty).
func Popup(client string, w, h int, title, cmd string) error {
	args := []string{"display-popup", "-E", "-w", strconv.Itoa(w), "-h", strconv.Itoa(h), "-T", title}
	if client != "" {
		args = append(args, "-c", client)
	}
	args = append(args, cmd)
	_, err := run(args...)
	return err
}
