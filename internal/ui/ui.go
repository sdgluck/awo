// Package ui is the dense profile picker shown in the tmux popup.
package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sdgluck/awo/internal/awsconf"
	"github.com/sdgluck/awo/internal/sso"
	"github.com/sdgluck/awo/internal/tmux"
)

var (
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold   = lipgloss.NewStyle().Bold(true)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cursor = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
)

// chrome is the rows that aren't profiles: header, filter, footer.
const chrome = 3

type Model struct {
	window, label, current string
	profiles               []awsconf.Profile
	cacheDir               string

	filter      textinput.Model
	shown       []int // indexes into profiles, ranked
	cur         int   // index into shown
	top         int   // first visible row of shown
	width       int
	height      int
	status      string
	statusStyle lipgloss.Style
	expiry      map[string]time.Time // by SessionKey; zero = no token
	loggingIn   bool
}

func New(window string, profiles []awsconf.Profile) Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = dim
	ti.Placeholder = "filter"
	ti.Focus()
	m := Model{
		window:   window,
		label:    tmux.Label(window),
		current:  tmux.Get(window),
		profiles: profiles,
		cacheDir: sso.CacheDir(),
		filter:   ti,
		width:    80,
		height:   len(profiles) + chrome,
	}
	m.loadExpiry()
	m.refilter()
	for i, pi := range m.shown {
		if profiles[pi].Name == m.current {
			m.cur = i
		}
	}
	return m
}

func (m *Model) loadExpiry() {
	m.expiry = map[string]time.Time{}
	for _, p := range m.profiles {
		if _, ok := m.expiry[p.SessionKey]; ok {
			continue
		}
		t, _ := sso.Expiry(m.cacheDir, p.SessionKey)
		m.expiry[p.SessionKey] = t
	}
}

func (m Model) valid(p awsconf.Profile) bool {
	return time.Until(m.expiry[p.SessionKey]) > time.Minute
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

type loginDone struct {
	profile string
	err     error
	thenUse bool
}

func (m Model) login(p awsconf.Profile, thenUse bool) tea.Cmd {
	c := exec.Command("aws", "sso", "login", "--profile", p.Name)
	return tea.ExecProcess(c, func(err error) tea.Msg { return loginDone{p.Name, err, thenUse} })
}

func (m Model) selected() (awsconf.Profile, bool) {
	if len(m.shown) == 0 {
		return awsconf.Profile{}, false
	}
	return m.profiles[m.shown[m.cur]], true
}

func (m Model) use(name string) (tea.Model, tea.Cmd) {
	if err := tmux.Set(m.window, name); err != nil {
		m.status, m.statusStyle = err.Error(), red
		return m, nil
	}
	return m, tea.Quit
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
		return m, nil

	case loginDone:
		m.loggingIn = false
		m.loadExpiry()
		if msg.err != nil {
			m.status, m.statusStyle = "login failed: "+msg.err.Error(), red
			return m, nil
		}
		if msg.thenUse {
			return m.use(msg.profile)
		}
		m.status, m.statusStyle = "logged in", green
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "ctrl+k", "ctrl+p":
			m.move(-1)
			return m, nil
		case "down", "ctrl+j", "ctrl+n", "tab":
			m.move(1)
			return m, nil
		case "enter":
			p, ok := m.selected()
			if !ok {
				return m, nil
			}
			if !m.valid(p) {
				m.loggingIn = true
				return m, m.login(p, true)
			}
			return m.use(p.Name)
		case "ctrl+l":
			if p, ok := m.selected(); ok {
				m.loggingIn = true
				return m, m.login(p, false)
			}
			return m, nil
		case "ctrl+x":
			if err := tmux.Unset(m.window); err != nil {
				m.status, m.statusStyle = err.Error(), red
				return m, nil
			}
			return m, tea.Quit
		}
	}
	prev := m.filter.Value()
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.refilter()
		m.cur, m.top = 0, 0
	}
	return m, cmd
}

func (m *Model) move(d int) {
	if len(m.shown) == 0 {
		return
	}
	m.cur = (m.cur + d + len(m.shown)) % len(m.shown)
	m.scroll()
}

func (m *Model) scroll() {
	rows := max(m.height-chrome, 1)
	if m.cur < m.top {
		m.top = m.cur
	}
	if m.cur >= m.top+rows {
		m.top = m.cur - rows + 1
	}
}

func (m *Model) refilter() {
	q := strings.ToLower(m.filter.Value())
	type hit struct{ idx, score int }
	var hits []hit
	for i, p := range m.profiles {
		if s, ok := Score(q, strings.ToLower(p.Name)); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	m.shown = m.shown[:0]
	for _, h := range hits {
		m.shown = append(m.shown, h.idx)
	}
}

// Score ranks substring matches (earlier is better) above subsequence matches
// (fewer gaps is better). ok is false when q is not a subsequence of s.
func Score(q, s string) (int, bool) {
	if q == "" {
		return 0, true
	}
	if i := strings.Index(s, q); i >= 0 {
		return 1000 - i, true
	}
	gaps, last, j := 0, -1, 0
	for i := 0; i < len(s) && j < len(q); i++ {
		if s[i] == q[j] {
			if last >= 0 {
				gaps += i - last - 1
			}
			last = i
			j++
		}
	}
	if j < len(q) {
		return 0, false
	}
	return 500 - gaps, true
}

func fmtDur(d time.Duration) string {
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

func (m Model) tokenBadge(p awsconf.Profile) string {
	t := m.expiry[p.SessionKey]
	switch left := time.Until(t); {
	case t.IsZero():
		return red.Render("sso ✗ no token")
	case left <= time.Minute:
		return red.Render("sso ✗ expired")
	case left < 30*time.Minute:
		return yellow.Render("sso ✓ " + fmtDur(left))
	default:
		return green.Render("sso ✓ " + fmtDur(left))
	}
}

func isProd(name string) bool { return strings.Contains(name, "prd") || strings.Contains(name, "prod") }

func pad(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func (m Model) View() string {
	var b strings.Builder
	w := m.width

	// header
	left := bold.Render("awo") + dim.Render(" · "+m.label)
	right := ""
	if p, ok := m.selected(); ok {
		right = m.tokenBadge(p)
	}
	if m.loggingIn {
		right = yellow.Render("logging in…")
	}
	gap := max(w-lipgloss.Width(left)-lipgloss.Width(right)-1, 1)
	b.WriteString(left + strings.Repeat(" ", gap) + right + "\n")

	b.WriteString(m.filter.View() + "\n")

	// columns: marker(2) name acct role region
	nameW, roleW, regW := 4, 4, 0
	for _, p := range m.profiles {
		nameW = max(nameW, len(p.Name))
		roleW = max(roleW, len(p.Role))
		regW = max(regW, len(p.Region))
	}
	const acctW = 12
	roleW = min(roleW, max(w-2-nameW-1-acctW-1-regW-1, 4))

	rows := max(m.height-chrome, 1)
	for i := m.top; i < len(m.shown) && i < m.top+rows; i++ {
		p := m.profiles[m.shown[i]]
		cols := []string{
			pad(p.Name, nameW),
			pad(trunc(p.AccountID, acctW), acctW),
			pad(trunc(p.Role, roleW), roleW),
			p.Region,
		}
		mark := "  "
		if p.Name == m.current {
			mark = "● "
		}
		var line string
		if i == m.cur {
			line = cursor.Render(pad(mark+strings.Join(cols, " "), w-1))
		} else {
			if mark != "  " {
				mark = green.Render(mark)
			}
			if isProd(p.Name) {
				cols[0] = red.Render(cols[0])
			}
			line = mark + cols[0] + " " + dim.Render(cols[1]) + " " + cols[2] + " " + dim.Render(cols[3])
		}
		b.WriteString(line + "\n")
	}
	for i := len(m.shown) - m.top; i < rows; i++ {
		b.WriteString("\n")
	}

	if m.status != "" {
		b.WriteString(m.statusStyle.Render(trunc(m.status, w-1)))
	} else {
		b.WriteString(dim.Render("⏎ use  ^l login  ^x clear  esc quit"))
	}
	return b.String()
}
