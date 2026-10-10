// Package ui is the dense profile picker shown in the tmux popup.
package ui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sdgluck/awo/internal/awsconf"
	"github.com/sdgluck/awo/internal/sso"
	"github.com/sdgluck/awo/internal/tmux"
)

var (
	// 245 rather than 8: bright black is the background colour in some
	// themes (e.g. Solarized Dark).
	dim   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	plain = lipgloss.NewStyle()
	bold  = lipgloss.NewStyle().Bold(true)
	green = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	// red is for errors only; prod gets its own colour so it doesn't read
	// as one.
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	prod   = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	cursor = lipgloss.NewStyle().Background(lipgloss.Color("237")).Bold(true)
)

// chrome is the rows that aren't profiles: header, filter, footer.
const chrome = 3

// maxRows caps the popup's list; longer lists scroll.
const maxRows = 20

const acctW = 12

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
	quitting    bool
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
	m.selectCurrent()
	return m
}

// Size is the content size (excluding any border) that fits every column and
// the header without truncation, showing at most maxRows profiles.
func (m Model) Size() (w, h int) {
	hdr := lipgloss.Width(m.headerLeft()) + 1 + lipgloss.Width("sso ✗ no token") + 1
	return max(m.layout(1<<16).width()+1, hdr, 46), min(len(m.profiles), maxRows) + chrome
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

// quit blanks the final frame so an inline (non-popup) run leaves nothing
// behind in the terminal.
func (m Model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Quit
}

func (m Model) use(name string) (tea.Model, tea.Cmd) {
	if err := tmux.Set(m.window, name); err != nil {
		m.status, m.statusStyle = err.Error(), red
		return m, nil
	}
	return m.quit()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Leave room for the prompt and the count on the right.
		m.filter.Width = max(m.width-1-2-12, 1)
		m.scroll()
		return m, nil

	case loginDone:
		m.loadExpiry()
		if msg.err != nil {
			m.status, m.statusStyle = "login failed: "+msg.err.Error(), red
			return m, nil
		}
		if msg.thenUse {
			return m.use(msg.profile)
		}
		m.status, m.statusStyle = "logged in to "+msg.profile, green
		return m, nil

	case tea.KeyMsg:
		// A status describes the last action; any key moves on from it.
		m.status = ""
		switch msg.String() {
		case "ctrl+c":
			return m.quit()
		case "esc":
			if m.filter.Value() == "" {
				return m.quit()
			}
			m.filter.SetValue("")
			m.refilter()
			m.selectCurrent()
			return m, nil
		case "up", "ctrl+p", "shift+tab":
			m.move(-1)
			return m, nil
		case "down", "ctrl+n", "tab":
			m.move(1)
			return m, nil
		case "pgup":
			m.page(-1)
			return m, nil
		case "pgdown":
			m.page(1)
			return m, nil
		case "enter":
			p, ok := m.selected()
			if !ok {
				return m, nil
			}
			if !m.valid(p) {
				return m, m.login(p, true)
			}
			return m.use(p.Name)
		case "ctrl+l":
			if p, ok := m.selected(); ok {
				return m, m.login(p, false)
			}
			return m, nil
		case "ctrl+x":
			if err := tmux.Unset(m.window); err != nil {
				m.status, m.statusStyle = err.Error(), red
				return m, nil
			}
			return m.quit()
		}
	}
	prev := m.filter.Value()
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.refilter()
		if m.filter.Value() == "" {
			m.selectCurrent()
		} else {
			m.cur, m.top = 0, 0
		}
	}
	return m, cmd
}

// rows is the height of the list. It is sized to the whole profile list, not
// the filtered one, so the view doesn't jump while typing.
func (m Model) rows() int {
	return max(min(m.height-chrome, len(m.profiles)), 1)
}

func (m *Model) selectCurrent() {
	m.cur, m.top = 0, 0
	for i, pi := range m.shown {
		if m.profiles[pi].Name == m.current {
			m.cur = i
		}
	}
	m.scroll()
}

func (m *Model) move(d int) {
	if len(m.shown) == 0 {
		return
	}
	m.cur = (m.cur + d + len(m.shown)) % len(m.shown)
	m.scroll()
}

// page moves a screenful without wrapping.
func (m *Model) page(d int) {
	if len(m.shown) == 0 {
		return
	}
	m.cur = max(min(m.cur+d*m.rows(), len(m.shown)-1), 0)
	m.scroll()
}

func (m *Model) scroll() {
	rows := m.rows()
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
		if s, ok := Match(q, p); ok {
			hits = append(hits, hit{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
	m.shown = m.shown[:0]
	for _, h := range hits {
		m.shown = append(m.shown, h.idx)
	}
}

// Match scores p against the lowercase query q. Name matches (see Score) rank
// above substring matches in the account id, role or region.
func Match(q string, p awsconf.Profile) (int, bool) {
	if s, ok := Score(q, strings.ToLower(p.Name)); ok {
		return s, true
	}
	best, ok := 0, false
	for _, f := range []string{p.AccountID, p.Role, p.Region} {
		if i := strings.Index(strings.ToLower(f), q); i >= 0 && (!ok || -1000-i > best) {
			best, ok = -1000-i, true
		}
	}
	return best, ok
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

// token describes p's SSO session: a one-cell glyph for the list, a badge for
// the header, and the colour both are drawn in.
func (m Model) token(p awsconf.Profile) (glyph, badge string, st lipgloss.Style) {
	t := m.expiry[p.SessionKey]
	switch left := time.Until(t); {
	case t.IsZero():
		return "✗", "sso ✗ no token", dim
	case left <= time.Minute:
		return "✗", "sso ✗ expired", dim
	case left < 30*time.Minute:
		return "✓", "sso ✓ " + fmtDur(left), yellow
	default:
		return "✓", "sso ✓ " + fmtDur(left), green
	}
}

// IsProd reports whether name has a prd/prod/production segment, optionally
// numbered: "fink-prd" and "prod2-eu" are prod, "product-dev" is not.
func IsProd(name string) bool {
	seg := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
	for _, f := range strings.FieldsFunc(strings.ToLower(name), seg) {
		switch strings.TrimRight(f, "0123456789") {
		case "prd", "prod", "production":
			return true
		}
	}
	return false
}

func (m Model) known(name string) bool {
	for _, p := range m.profiles {
		if p.Name == name {
			return true
		}
	}
	return false
}

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

// spread puts left and right at the edges of a line w wide.
func spread(left, right string, w int) string {
	return left + strings.Repeat(" ", max(w-lipgloss.Width(left)-lipgloss.Width(right), 1)) + right
}

// layout is the column widths; a zero width drops the column.
type layout struct{ name, acct, role, region int }

// width is the row width: marker(2) token(2) name, then " col" per column.
func (l layout) width() int {
	w := 4 + l.name
	for _, c := range []int{l.acct, l.role, l.region} {
		if c > 0 {
			w += 1 + c
		}
	}
	return w
}

// layout fits the columns into a line w wide, giving up the least useful
// first: the role shrinks, then the region is dropped, then the name shrinks,
// then the account is dropped.
func (m Model) layout(w int) layout {
	l := layout{acct: acctW}
	for _, p := range m.profiles {
		l.name = max(l.name, lipgloss.Width(p.Name))
		l.role = max(l.role, lipgloss.Width(p.Role))
		l.region = max(l.region, lipgloss.Width(p.Region))
	}
	shrink := func(col *int, floor int) {
		if over := l.width() - (w - 1); over > 0 && *col > floor {
			*col -= min(over, *col-floor)
		}
	}
	drop := func(col *int) {
		if l.width() > w-1 {
			*col = 0
		}
	}
	shrink(&l.role, 4)
	drop(&l.region)
	shrink(&l.name, 10)
	drop(&l.acct)
	return l
}

func (m Model) headerLeft() string {
	cur := dim.Render("no profile")
	switch {
	case m.current == "":
	case !m.known(m.current):
		cur = yellow.Render(m.current) + dim.Render(" (not in config)")
	case IsProd(m.current):
		cur = prod.Render(m.current)
	default:
		cur = green.Render(m.current)
	}
	return bold.Render("awo") + dim.Render(" · "+m.label+" · ") + cur
}

// row draws one profile, clipped to w. The selected row keeps its colours
// (prod, current green) on the cursor background, so the prod warning is
// still showing when Enter is pressed.
func (m Model) row(p awsconf.Profile, sel bool, l layout, w int) string {
	st := func(s lipgloss.Style) lipgloss.Style {
		if sel {
			return s.Inherit(cursor)
		}
		return s
	}
	mark := "  "
	if p.Name == m.current {
		mark = "● "
	}
	glyph, _, tokSt := m.token(p)
	nameSt := plain
	if IsProd(p.Name) {
		nameSt = prod
	}
	line := st(green).Render(mark) + st(tokSt).Render(glyph+" ") + st(nameSt).Render(pad(trunc(p.Name, l.name), l.name))
	for _, c := range []struct {
		s  string
		w  int
		st lipgloss.Style
	}{{p.AccountID, l.acct, dim}, {p.Role, l.role, plain}, {p.Region, l.region, dim}} {
		if c.w > 0 {
			line += st(c.st).Render(" " + pad(trunc(c.s, c.w), c.w))
		}
	}
	line = ansi.Truncate(line, w, "")
	if sel {
		line += cursor.Render(strings.Repeat(" ", max(w-lipgloss.Width(line), 0)))
	}
	return line
}

// count is the filter line's right side: matches/total, with arrows when
// rows are scrolled out of view.
func (m Model) count() string {
	arrows := ""
	if m.top > 0 {
		arrows += "↑"
	}
	if m.top+m.rows() < len(m.shown) {
		arrows += "↓"
	}
	if arrows != "" {
		arrows = yellow.Render(arrows) + " "
	}
	return arrows + dim.Render(fmt.Sprintf("%d/%d", len(m.shown), len(m.profiles)))
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	w := m.width - 1

	// header: window, its profile, and the selected profile's SSO session
	right := ""
	if p, ok := m.selected(); ok {
		_, badge, st := m.token(p)
		right = st.Render(badge)
	}
	left := ansi.Truncate(m.headerLeft(), max(w-lipgloss.Width(right)-1, 0), "…")
	b.WriteString(spread(left, right, w) + "\n")

	b.WriteString(spread(m.filter.View(), m.count(), w) + "\n")

	l := m.layout(m.width)
	for i := range m.rows() {
		switch j := m.top + i; {
		case j < len(m.shown):
			b.WriteString(m.row(m.profiles[m.shown[j]], j == m.cur, l, w))
		case i == 0:
			b.WriteString(dim.Render("  no matches"))
		}
		b.WriteString("\n")
	}

	if m.status != "" {
		b.WriteString(m.statusStyle.Render(trunc(m.status, w)))
	} else {
		b.WriteString(dim.Render(trunc("⏎ use  ^l login  ^x clear  esc quit", w)))
	}
	return b.String()
}
