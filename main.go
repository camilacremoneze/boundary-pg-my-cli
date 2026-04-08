package main

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/boundary"
	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/config"
	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/fuzzy"
	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/mycli"
	"github.com/camilacremoneze/pgcli-boundary-vault-integration/internal/pgcli"
)

// ─────────────────────────────────────────────────────────────────────────────
// Custom theme – deep navy background with teal accent
// ─────────────────────────────────────────────────────────────────────────────

type boundaryTheme struct{ fyne.Theme }

func newBoundaryTheme() fyne.Theme { return &boundaryTheme{theme.DarkTheme()} }

func (t *boundaryTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	// Window / panel background – deep navy
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x0f, G: 0x17, B: 0x29, A: 0xff}
	// Card / input backgrounds – slightly lighter navy
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x18, G: 0x24, B: 0x3d, A: 0xff}
	// Primary accent – vivid teal
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x00, G: 0xc8, B: 0xb4, A: 0xff}
	// Focused borders
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x00, G: 0xc8, B: 0xb4, A: 0xff}
	// Foreground (labels, icons)
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xe8, G: 0xf0, B: 0xfe, A: 0xff}
	// Muted text (placeholders, descriptions)
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x7a, G: 0x8a, B: 0xaa, A: 0xff}
	// Separator / divider lines
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0x28, G: 0x38, B: 0x58, A: 0xff}
	// Hover highlight
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x00, G: 0xc8, B: 0xb4, A: 0x28}
	// Selected item highlight
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x00, G: 0xc8, B: 0xb4, A: 0x40}
	// Semantic colours
	case theme.ColorNameSuccess:
		return color.NRGBA{R: 0x34, G: 0xd3, B: 0x99, A: 0xff}
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xfb, G: 0xbd, B: 0x23, A: 0xff}
	case theme.ColorNameError:
		return color.NRGBA{R: 0xf8, G: 0x71, B: 0x71, A: 0xff}
	}
	return t.Theme.Color(name, variant)
}

// ─────────────────────────────────────────────────────────────────────────────
// Session registry – thread-safe, keyed by stable SessionID string
// ─────────────────────────────────────────────────────────────────────────────

type sessionRegistry struct {
	mu    sync.Mutex
	order []string                     // insertion-ordered session IDs
	byID  map[string]*boundary.Session // keyed by SessionID
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{byID: make(map[string]*boundary.Session)}
}

func (r *sessionRegistry) add(s *boundary.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[s.SessionID]; !exists {
		r.order = append(r.order, s.SessionID)
	}
	r.byID[s.SessionID] = s
}

// removeByID removes and returns the session for the given ID.
// The Kill() call happens outside the lock to avoid blocking other callers.
func (r *sessionRegistry) removeByID(id string) *boundary.Session {
	r.mu.Lock()
	sess, ok := r.byID[id]
	if !ok {
		r.mu.Unlock()
		return nil
	}
	delete(r.byID, id)
	for i, oid := range r.order {
		if oid == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
	return sess
}

// snapshot returns all sessions in insertion order.
func (r *sessionRegistry) snapshot() []*boundary.Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*boundary.Session, 0, len(r.order))
	for _, id := range r.order {
		if s, ok := r.byID[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

func (r *sessionRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.order)
}

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("config: %v", err)
	}

	a := app.NewWithID("io.boundary.pgcli-boundary")
	a.Settings().SetTheme(newBoundaryTheme())
	w := a.NewWindow("pgcli · Boundary")
	w.Resize(fyne.NewSize(860, 640))

	ui := newAppUI(a, w)
	w.SetContent(ui.buildRoot())
	w.ShowAndRun()
}

// ─────────────────────────────────────────────────────────────────────────────
// UI controller
// ─────────────────────────────────────────────────────────────────────────────

type appUI struct {
	app    fyne.App
	window fyne.Window

	// Auth state
	boundaryToken        string
	boundaryAddr         string // selected environment controller URL
	boundaryAuthMethodID string // OIDC auth method ID for the selected environment
	boundaryEnvLabel     string // human-readable label of the selected environment

	// All targets from Boundary
	allTargets []boundary.Target
	// Filtered targets shown in the list
	filteredTargets []boundary.Target

	// Open sessions
	registry *sessionRegistry

	// ── Tab 1: Login ──
	loginBtn    *widget.Button
	loginStatus *widget.Label
	pasteEntry  *widget.Entry
	envSelect   *widget.Select

	// ── Tab 2: Targets ──
	searchEntry   *widget.Entry
	targetList    *widget.List
	targetStatus  *widget.Label
	connectingBtn *widget.Button

	// ── Tab 3: Sessions ──
	sessionList   *widget.List
	sessionStatus *widget.Label

	tabs *container.AppTabs
}

func newAppUI(a fyne.App, w fyne.Window) *appUI {
	// Find the auth method ID for the default environment.
	var defaultAuthMethodID string
	for _, e := range config.Cfg.Envs {
		if e.Label == config.Cfg.DefaultEnv {
			defaultAuthMethodID = e.AuthMethodID
			break
		}
	}
	return &appUI{
		app:                  a,
		window:               w,
		registry:             newSessionRegistry(),
		boundaryAddr:         config.Cfg.DefaultAddr(),
		boundaryAuthMethodID: defaultAuthMethodID,
		boundaryEnvLabel:     config.Cfg.DefaultEnv,
	}
}

func (u *appUI) buildRoot() fyne.CanvasObject {
	u.tabs = container.NewAppTabs(
		container.NewTabItem("  Login  ", u.buildLoginTab()),
		container.NewTabItem(" Targets ", u.buildTargetsTab()),
		container.NewTabItem(" Sessions", u.buildSessionsTab()),
	)
	u.tabs.SetTabLocation(container.TabLocationTop)
	return u.tabs
}

// ─────────────────────────────────────────────────────────────────────────────
// Tab 1 – Login
// ─────────────────────────────────────────────────────────────────────────────

func (u *appUI) buildLoginTab() fyne.CanvasObject {
	u.loginStatus = widget.NewLabel("")
	u.loginStatus.Wrapping = fyne.TextWrapWord

	// Environment selector – built from config.Cfg.Envs
	envLabels := make([]string, len(config.Cfg.Envs))
	for i, e := range config.Cfg.Envs {
		envLabels[i] = e.Label
	}
	u.envSelect = widget.NewSelect(envLabels, func(selected string) {
		for _, e := range config.Cfg.Envs {
			if e.Label == selected {
				u.boundaryAddr = e.Addr
				u.boundaryAuthMethodID = e.AuthMethodID
				u.boundaryEnvLabel = e.Label
				// Reset token when switching environments
				u.boundaryToken = ""
				u.loginStatus.SetText(fmt.Sprintf("Environment: %s", e.Addr))
				break
			}
		}
	})
	if len(config.Cfg.Envs) > 0 {
		u.envSelect.SetSelected(config.Cfg.DefaultEnv)
	}

	envRow := container.NewBorder(nil, nil, widget.NewLabel("Environment"), nil, u.envSelect)

	u.loginBtn = widget.NewButtonWithIcon("  Login with SSO  ", theme.LoginIcon(), func() {
		u.doBoundaryOIDCLogin()
	})
	u.loginBtn.Importance = widget.HighImportance
	if len(config.Cfg.Envs) == 0 {
		u.loginBtn.Disable()
		u.loginStatus.SetText("No environments configured. Set BOUNDARY_ENVS in your .env file.")
	}

	userHint := config.Cfg.BoundaryUser
	if userHint == "" {
		userHint = "your SSO account"
	}
	info := widget.NewLabel(fmt.Sprintf("Authenticates as %s via SSO.", userHint))
	info.Wrapping = fyne.TextWrapWord

	u.pasteEntry = widget.NewPasswordEntry()
	u.pasteEntry.SetPlaceHolder("at_xxxx… (paste existing Boundary token)")

	pasteBtn := widget.NewButtonWithIcon("  Use token  ", theme.ConfirmIcon(), func() {
		tok := strings.TrimSpace(u.pasteEntry.Text)
		if tok == "" {
			u.loginStatus.SetText("Paste a token first.")
			return
		}
		u.boundaryToken = tok
		u.loginStatus.SetText("Token accepted.")
		u.postLogin()
	})

	orLabel := widget.NewLabelWithStyle("— or paste an existing token —",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	// Centre the login button horizontally
	loginRow := container.NewCenter(u.loginBtn)

	return container.NewVBox(
		widget.NewCard("Boundary Login", "", container.NewVBox(
			envRow,
			widget.NewSeparator(),
			info,
			loginRow,
		)),
		widget.NewSeparator(),
		orLabel,
		container.NewBorder(nil, nil, nil, pasteBtn, u.pasteEntry),
		widget.NewSeparator(),
		u.loginStatus,
	)
}

func (u *appUI) doBoundaryOIDCLogin() {
	u.loginBtn.Disable()
	u.loginStatus.SetText(fmt.Sprintf("Opening browser for SSO login on %s… (up to 3 min)", u.boundaryAddr))
	addr := u.boundaryAddr
	go func() {
		result, err := boundary.AuthenticateOIDC(addr, u.boundaryAuthMethodID)
		if err != nil {
			fyne.Do(func() {
				u.loginStatus.SetText("Error: " + err.Error())
				u.loginBtn.Enable()
			})
			return
		}
		fyne.Do(func() {
			u.boundaryToken = result.Token
			u.loginStatus.SetText("Authenticated via SSO.")
			u.loginBtn.Enable()
			u.postLogin()
		})
	}()
}

func (u *appUI) postLogin() {
	u.tabs.SelectIndex(1)
	u.doListTargets()
}

// ─────────────────────────────────────────────────────────────────────────────
// Tab 2 – Targets (fuzzy search + connect)
// ─────────────────────────────────────────────────────────────────────────────

func (u *appUI) buildTargetsTab() fyne.CanvasObject {
	u.targetStatus = widget.NewLabel("Login first, then targets will load automatically.")
	u.targetStatus.Wrapping = fyne.TextWrapWord

	// Search box
	u.searchEntry = widget.NewEntry()
	u.searchEntry.SetPlaceHolder("Search  e.g. cards read  or  business write")
	u.searchEntry.OnChanged = func(q string) {
		u.applyFilter(q)
	}

	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		u.doListTargets()
	})

	searchRow := container.NewBorder(nil, nil, nil, refreshBtn, u.searchEntry)

	// Target list
	u.filteredTargets = []boundary.Target{}
	u.targetList = widget.NewList(
		func() int { return len(u.filteredTargets) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			desc := widget.NewLabel("")
			desc.TextStyle = fyne.TextStyle{Italic: true}

			btn := widget.NewButtonWithIcon("  Connect  ", theme.MediaPlayIcon(), nil)
			btn.Importance = widget.HighImportance

			// NewBorder(nil,nil,nil,btn,textBlock):
			//   Objects[0] = textBlock  (center – stretches to fill)
			//   Objects[1] = btn        (right edge)
			textBlock := container.NewVBox(name, desc)
			return container.NewBorder(nil, nil, nil, btn, textBlock)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			if id >= len(u.filteredTargets) {
				return
			}
			t := u.filteredTargets[id]
			c := item.(*fyne.Container)
			// Objects[0]=textBlock, Objects[1]=btn
			textBlock := c.Objects[0].(*fyne.Container)
			textBlock.Objects[0].(*widget.Label).SetText(t.Name)
			textBlock.Objects[1].(*widget.Label).SetText(t.Description)

			btn := c.Objects[1].(*widget.Button)
			captured := t
			btn.OnTapped = func() {
				u.doConnect(captured)
			}
		},
	)

	return container.NewBorder(
		container.NewVBox(searchRow, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), u.targetStatus),
		nil, nil,
		u.targetList,
	)
}

func (u *appUI) doListTargets() {
	if u.boundaryToken == "" {
		u.targetStatus.SetText("Not logged in – go to Login tab first.")
		return
	}
	u.targetStatus.SetText("Loading targets…")
	addr := u.boundaryAddr
	go func() {
		targets, err := boundary.ListTargets(addr, u.boundaryToken)
		if err != nil {
			fyne.Do(func() {
				u.targetStatus.SetText("Error: " + err.Error())
			})
			return
		}
		fyne.Do(func() {
			u.allTargets = targets
			u.applyFilter(u.searchEntry.Text)
			u.targetStatus.SetText(fmt.Sprintf("%d targets  ·  %d shown", len(targets), len(u.filteredTargets)))
		})
	}()
}

func (u *appUI) applyFilter(q string) {
	if q == "" {
		u.filteredTargets = make([]boundary.Target, len(u.allTargets))
		copy(u.filteredTargets, u.allTargets)
	} else {
		u.filteredTargets = u.filteredTargets[:0]
		for _, t := range u.allTargets {
			// Match against name + description combined
			haystack := t.Name + " " + t.Description
			if fuzzy.Match(q, haystack) {
				u.filteredTargets = append(u.filteredTargets, t)
			}
		}
	}
	u.targetList.Refresh()
	u.targetStatus.SetText(fmt.Sprintf("%d targets  ·  %d shown", len(u.allTargets), len(u.filteredTargets)))
}

func (u *appUI) doConnect(t boundary.Target) {
	u.targetStatus.SetText(fmt.Sprintf("Connecting to %s…", t.Name))
	t.Env = u.boundaryEnvLabel
	addr := u.boundaryAddr
	go func() {
		sess, err := boundary.Connect(addr, u.boundaryToken, t)
		if err != nil {
			fyne.Do(func() {
				u.targetStatus.SetText("Error: " + err.Error())
			})
			return
		}

		u.registry.add(sess)

		fyne.Do(func() {
			u.targetStatus.SetText(fmt.Sprintf("Connected: %s  (port %d)", t.Name, sess.Port))
			u.refreshSessionList()
			u.tabs.SelectIndex(2)

			// Auto-launch the appropriate CLI if we have credentials
			if sess.Username != "" && sess.Password != "" {
				u.launchCLI(sess)
			} else {
				dialog.ShowInformation("Connected",
					fmt.Sprintf("Session open on 127.0.0.1:%d\nNo credentials in session – connect manually.",
						sess.Port), u.window)
			}
		})
	}()
}

// ─────────────────────────────────────────────────────────────────────────────
// Tab 3 – Sessions
// ─────────────────────────────────────────────────────────────────────────────

func (u *appUI) buildSessionsTab() fyne.CanvasObject {
	u.sessionStatus = widget.NewLabel("No open sessions.")
	u.sessionStatus.Wrapping = fyne.TextWrapWord

	u.sessionList = widget.NewList(
		func() int { return u.registry.len() },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			highlights := widget.NewLabel("")
			highlights.TextStyle = fyne.TextStyle{Bold: true}
			addr := widget.NewLabel("")
			addr.TextStyle = fyne.TextStyle{Italic: true}

			// Teal accent bar on the left edge of each row.
			accent := canvas.NewRectangle(color.NRGBA{R: 0x00, G: 0xc8, B: 0xb4, A: 0xff})
			accent.SetMinSize(fyne.NewSize(4, 0))

			launchBtn := widget.NewButtonWithIcon("  pgcli  ", theme.MediaPlayIcon(), nil)
			launchBtn.Importance = widget.HighImportance
			copyBtn := widget.NewButtonWithIcon("  Copy DSN  ", theme.ContentCopyIcon(), nil)
			killBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), nil)
			killBtn.Importance = widget.DangerImportance

			// btns [0]=launchBtn [1]=copyBtn [2]=killBtn
			btns := container.NewHBox(launchBtn, copyBtn, killBtn)

			// textBlock [0]=name [1]=highlights [2]=addr
			textBlock := container.NewVBox(name, highlights, addr)

			// NewBorder(nil,nil,accent,btns,textBlock):
			//   Objects[0] = textBlock  (center – stretches to fill)
			//   Objects[1] = accent     (left edge)
			//   Objects[2] = btns       (right edge)
			return container.NewBorder(nil, nil, accent, btns, textBlock)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			// Use a snapshot so the slice is stable for this update call.
			snap := u.registry.snapshot()
			if id >= len(snap) {
				return
			}
			sess := snap[id]

			// Objects[0]=textBlock, Objects[1]=accent, Objects[2]=btns
			c := item.(*fyne.Container)
			textBlock := c.Objects[0].(*fyne.Container)
			textBlock.Objects[0].(*widget.Label).SetText(sess.Target.Name)
			textBlock.Objects[1].(*widget.Label).SetText(
				fmt.Sprintf("%s  ·  %s", sess.Target.Env, sess.Target.Database),
			)
			textBlock.Objects[2].(*widget.Label).SetText(
				fmt.Sprintf("127.0.0.1:%d", sess.Port),
			)

			btns := c.Objects[2].(*fyne.Container)
			launchBtn := btns.Objects[0].(*widget.Button)
			copyBtn := btns.Objects[1].(*widget.Button)
			killBtn := btns.Objects[2].(*widget.Button)

			// Update launch button label to reflect the actual CLI tool.
			if sess.Target.DBType == "mysql" || sess.Target.DBType == "mariadb" {
				launchBtn.SetText("  mycli  ")
			} else {
				launchBtn.SetText("  pgcli  ")
			}

			// Capture by value – immune to list re-use and index shifts.
			capturedSess := sess
			capturedID := sess.SessionID
			launchBtn.OnTapped = func() { u.launchCLI(capturedSess) }
			copyBtn.OnTapped = func() { u.copyDSN(capturedSess) }
			killBtn.OnTapped = func() { u.killSession(capturedID) }
		},
	)

	return container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), u.sessionStatus),
		nil, nil,
		u.sessionList,
	)
}

func (u *appUI) refreshSessionList() {
	u.sessionList.Refresh()
	n := u.registry.len()
	if n == 0 {
		u.sessionStatus.SetText("No open sessions.")
	} else {
		u.sessionStatus.SetText(fmt.Sprintf("%d session(s) open.", n))
	}
}

func (u *appUI) launchCLI(sess *boundary.Session) {
	if sess.Username == "" || sess.Password == "" {
		dialog.ShowError(fmt.Errorf("session %s has no credentials", sess.Target.Name), u.window)
		return
	}
	go func() {
		var err error
		switch sess.Target.DBType {
		case "mysql", "mariadb":
			err = mycli.LaunchMycli(sess.Username, sess.Password, "127.0.0.1", sess.Port, sess.Target.Database)
		default:
			err = pgcli.LaunchPgcli(sess.Username, sess.Password, "127.0.0.1", sess.Port, sess.Target.Database)
		}
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(err, u.window)
			})
		}
	}()
}

func (u *appUI) copyDSN(sess *boundary.Session) {
	var dsn string
	switch sess.Target.DBType {
	case "mysql", "mariadb":
		dsn = fmt.Sprintf("mysql://%s:%s@127.0.0.1:%d/%s",
			sess.Username, sess.Password, sess.Port, sess.Target.Database)
	default:
		dsn = fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s",
			sess.Username, sess.Password, sess.Port, sess.Target.Database)
	}
	u.window.Clipboard().SetContent(dsn)
	u.sessionStatus.SetText(fmt.Sprintf("DSN copied for %s (127.0.0.1:%d/%s)",
		sess.Target.Name, sess.Port, sess.Target.Database))
}

func (u *appUI) killSession(sessionID string) {
	go func() {
		sess := u.registry.removeByID(sessionID)
		if sess != nil {
			sess.Kill()
		}
		fyne.Do(func() {
			u.refreshSessionList()
		})
	}()
}
