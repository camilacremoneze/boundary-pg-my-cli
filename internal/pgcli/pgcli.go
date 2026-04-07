// Package pgcli launches pgcli in a new terminal window.
package pgcli

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// LaunchPgcli opens a new terminal window and runs pgcli with the given DSN.
// DSN format: postgres://user:password@127.0.0.1:port/database
// If database is empty the database segment is omitted.
func LaunchPgcli(user, password, host string, port int, database string) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d",
		url.PathEscape(user),
		url.PathEscape(password),
		host,
		port,
	)
	if database != "" {
		dsn += "/" + url.PathEscape(database)
	}

	switch runtime.GOOS {
	case "darwin":
		return launchMacOS(dsn)
	case "linux":
		return launchLinux(dsn)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func launchMacOS(dsn string) error {
	// Prefer iTerm2 if installed, fall back to Terminal.app.
	if _, err := exec.LookPath("open"); err == nil {
		if isAppInstalled("iTerm") {
			return launchITerm2(dsn)
		}
	}
	return launchTerminalApp(dsn)
}

// isAppInstalled checks whether an application bundle is present in /Applications.
func isAppInstalled(name string) bool {
	_, err := exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application "Finder" to return exists application file id (id of application "%s")`, name),
	).Output()
	if err == nil {
		return true
	}
	// Fallback: check /Applications directly
	_, ferr := exec.Command("test", "-d", "/Applications/"+name+".app").Output()
	return ferr == nil
}

func launchITerm2(dsn string) error {
	script := fmt.Sprintf(`tell application "iTerm"
	activate
	tell current window
		create tab with default profile
		tell current session
			write text "pgcli '%s'"
		end tell
	end tell
end tell`, escapeSingleQuotes(dsn))

	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		// iTerm may not have an open window yet; create a new one
		script2 := fmt.Sprintf(`tell application "iTerm"
	activate
	set newWindow to (create window with default profile)
	tell current session of newWindow
		write text "pgcli '%s'"
	end tell
end tell`, escapeSingleQuotes(dsn))
		cmd2 := exec.Command("osascript", "-e", script2)
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return fmt.Errorf("osascript iTerm2: %w\n%s\n%s", err, string(out), string(out2))
		}
	}
	return nil
}

func launchTerminalApp(dsn string) error {
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "pgcli '%s'"
end tell`, escapeSingleQuotes(dsn))

	cmd := exec.Command("osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w\n%s", err, string(out))
	}
	return nil
}

func launchLinux(dsn string) error {
	// Try common terminal emulators in order
	terminals := [][]string{
		{"gnome-terminal", "--", "pgcli", dsn},
		{"xterm", "-e", "pgcli " + shellQuote(dsn)},
		{"konsole", "-e", "pgcli " + shellQuote(dsn)},
		{"xfce4-terminal", "-e", "pgcli " + shellQuote(dsn)},
	}
	for _, t := range terminals {
		if _, err := exec.LookPath(t[0]); err == nil {
			cmd := exec.Command(t[0], t[1:]...)
			if err2 := cmd.Start(); err2 == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no supported terminal emulator found (tried gnome-terminal, xterm, konsole, xfce4-terminal)")
}

func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func shellQuote(s string) string {
	return "'" + escapeSingleQuotes(s) + "'"
}
