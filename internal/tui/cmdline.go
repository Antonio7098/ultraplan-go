package tui

import (
	"fmt"
	"strings"
)

// Direction contract (neovim patterns, adapted to UltraPlan content):
// THESIS: an operator console in neovim's idiom — cmdline jump labels,
// ":" command palette, sign column for state, tab counts — replacing
// the "every action is a sidebar row" model without losing content.
// OWN-WORLD: herdr's Tokyo Night remains; new chrome sits on panel bg.
// STORY: power users jump in 1-2 keystrokes via labels; everyone gets
// a status summary at the top and a single-line command palette at
// the bottom. State at a glance per row via the sign column.
// FIRST VIEWPORT: tab bar (with badge counts) / breadcrumb + health
// strip / sidebar (jump labels + sign column) | bordered detail / mode
// bar (or cmdline input when active).
// FORM: continuation of herdr grammar; neovim-level navigation atop it.
// FINISH: unreviewed and undocumented is unfinished; this build ends with
// the finish review, the verdict, DESIGN.md, and every shipping raster
// carrying its provenance.

// jumpLabel runes, mapping first 35 visible sidebar rows to a quick-access
// rune: digits "1"–"9" then letters "a"–"z". Beyond 35 rows the item gets
// no label and must be reached by j/k or scrolling.
func jumpLabelFor(index int) rune {
	if index < 0 || index >= 35 {
		return 0
	}
	if index < 9 {
		return rune('1' + index)
	}
	return rune('a' + index - 9)
}

// jumpRuneFor classifies a key string into a possible label key. Returns
// zero when the key isn't eligible.
func jumpRuneFor(key string) rune {
	if len(key) != 1 {
		return 0
	}
	r := rune(key[0])
	if r >= '1' && r <= '9' {
		return r
	}
	if r >= 'a' && r <= 'z' {
		return r
	}
	return 0
}

// tryJumpLabel returns true when the visible sidebar contains a row whose
// jumpLabel matches r and updates selection to that item.
func (m *Model) tryJumpLabel(r rune) bool {
	if m.Focus != FocusContent {
		return false
	}
	items := m.navItems()
	if len(items) == 0 {
		return false
	}
	for i, item := range items {
		_ = item
		if jumpLabelFor(i) == r {
			m.Selected = i
			m.PendingJump = 0
			m.clampSelection()
			return true
		}
	}
	return false
}

// modalActive reports whether a modal pane is consuming the screen.
func (m Model) modalActive() bool {
	return m.Preview != nil || m.Confirmation != nil || m.Validation != nil || m.ParallelForm != nil || (m.Operation != nil && !m.OperationHidden) || m.RunViewStudy != ""
}

// applyCmdlineKey handles one keypress while the cmdline input is open.
// Returns m for chaining.
func (m *Model) applyCmdlineKey(key string) {
	switch key {
	case "esc":
		m.closeCmdline(false)
	case "enter":
		m.executeCmdline()
	case "backspace":
		if m.Input != nil && len(m.Input.Value) > 0 {
			r := []rune(m.Input.Value)
			m.Input.Value = string(r[:len(r)-1])
		}
	default:
		if len(key) == 1 {
			r := rune(key[0])
			// Cmdline accepts printable, non-control runes. Space is fine
			// (fuzzy queries can contain it).
			if r >= 0x20 && r < 0x7f {
				if m.Input == nil {
					m.Input = &CmdlineInput{Active: true}
				}
				m.Input.Value += key
			}
		}
	}
}

// executeCmdline parses the buffered command and applies the side-effect.
// A handful of single-letter or prefix commands covers the common jumps;
// anything else becomes a `:g <query>` substring search across the
// sidebar. This is intentionally small — neovim's strength is that the
// only thing you really need to remember is how to ask for it.
func (m *Model) executeCmdline() {
	if m.Input == nil {
		return
	}
	cmd := strings.TrimSpace(m.Input.Value)
	m.closeCmdline(true)
	if cmd == "" {
		m.Toast = ":"
		return
	}
	// Split into verb + arg.
	verb, arg, _ := strings.Cut(cmd, " ")
	verb = strings.ToLower(verb)
	switch verb {
	case "p", "1":
		m.setTab(TabProjects)
		m.Toast = ":tab projects"
	case "s", "u", "2":
		m.setTab(TabStudies)
		m.Toast = ":tab studies"
	case "r", "w", "3":
		m.setTab(TabRuns)
		m.Toast = ":tab runs"
	case "b":
		m.Update(KeyMsg("esc"))
		m.Toast = ":back"
	case "q":
		m.Quit = true
	case "c", "cancel":
		m.Update(KeyMsg("c"))
		m.Toast = ":cancel"
	case "refresh":
		m.Update(KeyMsg("r"))
		m.Toast = ":refresh"
	case "g", "/":
		if arg == "" {
			m.Toast = ":g needs a query"
			return
		}
		if !m.jumpToLabelMatch(arg) {
			m.Toast = ":no match for " + arg
		}
	case "e", "enter":
		m.Update(KeyMsg("enter"))
		m.Toast = ":enter"
	case "esc":
		m.Update(KeyMsg("esc"))
		m.Toast = ":esc"
	default:
		// Unknown verb: try as a fuzzy substring against visible labels.
		if !m.jumpToLabelMatch(cmd) {
			m.Toast = ":unknown " + verb
		}
	}
}

// jumpToLabelMatch walks current nav items and selects the first whose
// label contains the query (case-insensitive). On success, the sidebar
// cursor moves and a toast confirms the jump.
func (m *Model) jumpToLabelMatch(query string) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return false
	}
	items := m.navItems()
	for i, item := range items {
		if strings.Contains(strings.ToLower(item.Label), needle) {
			m.Selected = i
			m.PendingJump = 0
			m.clampSelection()
			m.Toast = fmt.Sprintf("→ %s", item.Label)
			return true
		}
	}
	return false
}
