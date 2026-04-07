// Package mycli launches mycli in a new terminal window.
package mycli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// LaunchMycli opens a new terminal window and runs mycli with the given credentials.
// Works for both MySQL and MariaDB targets.
func LaunchMycli(user, password, host string, port int, database string) error {
	// mycli connection flags – avoids embedding credentials in a URI so special
	// characters in passwords don't need escaping.
	args := []string{
		"-u", user,
		"-p", password,
		"-h", host,
		"-P", fmt.Sprintf("%d", port),
	}
	if database != "" {
		args = append(args, database)
	}

	switch runtime.GOOS {
	case "darwin":
		return launchMacOS(args)
	case "linux":
		return launchLinux(args)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func launchMacOS(args []string) error {
	cmd := "mycli " + shellJoin(args)
	// Prefer iTerm2 if installed, fall back to Terminal.app.
	if isAppInstalled("iTerm") {
		return launchITerm2(cmd)
	}
	return launchTerminalApp(cmd)
}

// isAppInstalled checks whether an application bundle is present in /Applications.
func isAppInstalled(name string) bool {
	_, err := exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application "Finder" to return exists application file id (id of application "%s")`, name),
	).Output()
	if err == nil {
		return true
	}
	_, ferr := exec.Command("test", "-d", "/Applications/"+name+".app").Output()
	return ferr == nil
}

func launchITerm2(shellCmd string) error {
	script := fmt.Sprintf(`tell application "iTerm"
	activate
	tell current window
		create tab with default profile
		tell current session
			write text "%s"
		end tell
	end tell
end tell`, escapeDoubleQuotes(shellCmd))

	c := exec.Command("osascript", "-e", script)
	if out, err := c.CombinedOutput(); err != nil {
		// iTerm may not have an open window yet; create a new one
		script2 := fmt.Sprintf(`tell application "iTerm"
	activate
	set newWindow to (create window with default profile)
	tell current session of newWindow
		write text "%s"
	end tell
end tell`, escapeDoubleQuotes(shellCmd))
		c2 := exec.Command("osascript", "-e", script2)
		if out2, err2 := c2.CombinedOutput(); err2 != nil {
			return fmt.Errorf("osascript iTerm2: %w\n%s\n%s", err, string(out), string(out2))
		}
	}
	return nil
}

func launchTerminalApp(shellCmd string) error {
	script := fmt.Sprintf(`tell application "Terminal"
	activate
	do script "%s"
end tell`, escapeDoubleQuotes(shellCmd))

	c := exec.Command("osascript", "-e", script)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript: %w\n%s", err, string(out))
	}
	return nil
}

func launchLinux(args []string) error {
	fullArgs := append([]string{"mycli"}, args...)
	terminals := []struct {
		name   string
		prefix []string
	}{
		{"gnome-terminal", []string{"gnome-terminal", "--"}},
		{"xterm", []string{"xterm", "-e"}},
		{"konsole", []string{"konsole", "-e"}},
		{"xfce4-terminal", []string{"xfce4-terminal", "-e"}},
	}
	for _, t := range terminals {
		if _, err := exec.LookPath(t.name); err == nil {
			cmdArgs := append(t.prefix, fullArgs...)
			c := exec.Command(cmdArgs[0], cmdArgs[1:]...)
			if err2 := c.Start(); err2 == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no supported terminal emulator found (tried gnome-terminal, xterm, konsole, xfce4-terminal)")
}

// shellJoin builds a single shell-safe argument string from a slice.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func escapeDoubleQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
