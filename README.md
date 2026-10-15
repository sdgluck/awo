# awo

Per-tmux-window AWS SSO profile switcher. Pick a profile for a tmux window and
every zsh pane in that window gets it as `AWS_PROFILE`.

## Install

```sh
brew install sdgluck/tap/awo
```

Requires tmux, zsh and the AWS CLI v2.

## Setup

Add the shell hook to `~/.zshrc`:

```sh
eval "$(awo init zsh)"
```

Bind the picker to a key in `tmux.conf`:

```tmux
bind A run-shell -b "awo popup '#{window_id}' '#{client_tty}'"
```

Profiles come from `~/.aws/config` (or `$AWS_CONFIG_FILE`). Only SSO profiles
are listed.

## Picker

Press `prefix A` to open the picker in a popup. Each row shows the profile's
name, account ID, role and region. The marks before the names show whether the
profile's SSO session is signed in (`✓`) or expired (`✗`). Profiles with a `prd`,
`prod` or `production` segment in their name are shown in magenta.

| Key                    | Action                                             |
| ---------------------- | -------------------------------------------------- |
| type                   | filter profiles                                    |
| `↑` `↓` / `^p` `^n`    | move                                               |
| `⏎`                    | use the profile, running `aws sso login` first if expired |
| `^l`                   | run `aws sso login` for the profile                |
| `^x`                   | clear the window's profile                         |
| `esc`                  | clear the filter, or quit                          |

A pane picks up the new profile at its next prompt or command.

## Commands

```
awo                          pick a profile for the current window
awo pick [--window @N]       same, for a given window
awo popup @N [client_tty]    open the picker in a tmux popup
awo set <profile> [--window @N]
awo clear [--window @N]
awo current [--window @N]    print the window's profile
awo list                     print configured SSO profiles
awo init zsh                 print the shell hook
awo version
```

Setting `AWS_PROFILE` by hand in a pane overrides the window's profile in that
pane until the window's profile is changed.

## License

MIT
