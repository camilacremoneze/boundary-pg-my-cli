// Package boundary wraps the boundary CLI for authentication and session management.
package boundary

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AuthResult holds the token returned after authentication.
type AuthResult struct {
	Token string
}

// Target represents a Boundary target.
type Target struct {
	ID          string
	Name        string
	Description string
	Type        string
	Address     string
	DefaultPort int
	Database    string // parsed from description "db: <name>"
	DBType      string // "postgres", "mysql", or "mariadb" – parsed from description "type: <value>"
	Env         string // the UI-selected environment label (e.g. "staging", "production")
}

// Session represents an active boundary connect session with credentials.
type Session struct {
	Port      int
	SessionID string
	Username  string
	Password  string
	Target    Target
	Cmd       *exec.Cmd
}

// AuthenticateOIDC runs `boundary authenticate oidc` which opens a browser window
// for SSO. It blocks until the browser callback completes and returns the token.
func AuthenticateOIDC(boundaryAddr, authMethodID string) (*AuthResult, error) {
	args := []string{
		"authenticate", "oidc",
		"-addr", boundaryAddr,
		"-auth-method-id", authMethodID,
		"-format", "json",
	}
	cmd := exec.Command("boundary", args...)
	cmd.Env = pathEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start boundary authenticate oidc: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			return nil, fmt.Errorf("boundary authenticate oidc: %w\n%s", err, stderr.String())
		}
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("timeout waiting for OIDC browser callback (3 min)")
	}

	token, err := extractToken(extractJSON(stdout.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("%w\nraw: %s", err, stdout.String())
	}
	return &AuthResult{Token: token}, nil
}

// ListTargets fetches all targets visible to the authenticated user.
func ListTargets(boundaryAddr, token string) ([]Target, error) {
	args := []string{
		"targets", "list",
		"-addr", boundaryAddr,
		"-token", "env://BOUNDARY_TOKEN",
		"-format", "json",
		"-recursive",
	}
	cmd := exec.Command("boundary", args...)
	cmd.Env = append(pathEnv(), "BOUNDARY_TOKEN="+token)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("boundary targets list: %w\n%s", err, stderr.String())
	}

	jsonData := extractJSON(stdout.Bytes())
	var resp struct {
		Items []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Type        string `json:"type"`
			Address     string `json:"address"`
			Attributes  struct {
				DefaultPort int `json:"default_port"`
			} `json:"attributes"`
		} `json:"items"`
	}
	if err := json.Unmarshal(jsonData, &resp); err != nil {
		return nil, fmt.Errorf("parse targets response: %w\nraw: %s", err, stdout.String())
	}

	targets := make([]Target, 0, len(resp.Items))
	for _, item := range resp.Items {
		targets = append(targets, Target{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Type:        item.Type,
			Address:     item.Address,
			DefaultPort: item.Attributes.DefaultPort,
			Database:    parseDB(item.Description),
			DBType:      parseDBType(item.Description),
		})
	}
	return targets, nil
}

// Connect runs `boundary connect` for the given target. It reads the first JSON
// line from stdout (which contains port + credentials), then keeps the process
// running in the background. The caller must call Session.Kill() when done.
func Connect(boundaryAddr, token string, target Target) (*Session, error) {
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}

	args := []string{
		"connect",
		"-addr", boundaryAddr,
		"-token", "env://BOUNDARY_TOKEN",
		"-target-id", target.ID,
		"-listen-port", strconv.Itoa(port),
		"-format", "json",
	}
	cmd := exec.Command("boundary", args...)
	cmd.Env = append(pathEnv(), "BOUNDARY_TOKEN="+token)
	cmd.Stderr = nil // discard stderr deprecation warnings

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start boundary connect: %w", err)
	}

	// Read the first JSON line boundary prints when the session is established.
	// It contains port, session_id and credentials.
	type connectOutput struct {
		Address     string `json:"address"`
		Port        int    `json:"port"`
		SessionID   string `json:"session_id"`
		Credentials []struct {
			Credential struct {
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"credential"`
		} `json:"credentials"`
	}

	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "{") {
				lineCh <- line
				return
			}
		}
		close(lineCh)
	}()

	var out connectOutput
	select {
	case line, ok := <-lineCh:
		if !ok {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("boundary connect: no JSON output received")
		}
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("parse connect output: %w\nraw: %s", err, line)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("timeout waiting for boundary connect to establish session (30s)")
	}

	// Use the port boundary actually allocated (may differ from requested).
	if out.Port != 0 {
		port = out.Port
	}

	// Wait for the port to be reachable.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	sess := &Session{
		Port:      port,
		SessionID: out.SessionID,
		Target:    target,
		Cmd:       cmd,
	}
	if len(out.Credentials) > 0 {
		sess.Username = out.Credentials[0].Credential.Username
		sess.Password = out.Credentials[0].Credential.Password
	}
	return sess, nil
}

// Kill terminates the boundary connect process for this session.
func (s *Session) Kill() {
	if s.Cmd != nil && s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
		_ = s.Cmd.Wait()
	}
}

// extractToken pulls the token out of the JSON response from `boundary authenticate`.
func extractToken(data []byte) (string, error) {
	var resp struct {
		Attributes struct {
			Token string `json:"token"`
		} `json:"attributes"`
		Item struct {
			Attributes struct {
				Token string `json:"token"`
			} `json:"attributes"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse boundary response: %w", err)
	}
	token := resp.Attributes.Token
	if token == "" {
		token = resp.Item.Attributes.Token
	}
	if token == "" {
		return "", fmt.Errorf("no token in boundary response")
	}
	return token, nil
}

// extractJSON skips any non-JSON preamble lines (e.g. deprecation warnings
// printed to stdout before the JSON payload) and returns the first line that
// starts with '{' or '['.
func extractJSON(data []byte) []byte {
	for _, line := range bytes.Split(data, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			return trimmed
		}
	}
	return data
}

// freePort asks the OS for a free TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func pathEnv() []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin",
		"HOME=" + homeDir(),
	}
}

func homeDir() string {
	out, err := exec.Command("sh", "-c", "echo $HOME").Output()
	if err != nil {
		return "/tmp"
	}
	return strings.TrimSpace(string(out))
}

// parseDB extracts the database name from a description like:
// "type: postgres, name: foo, port: 5432, db: mydb"
func parseDB(desc string) string {
	re := regexp.MustCompile(`(?i)\bdb:\s*([^\s,]+)`)
	if m := re.FindStringSubmatch(desc); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// parseDBType extracts the database engine from a description like:
// "type: mysql, ..." or "type: mariadb, ..." or "type: postgres, ..."
// Returns "mysql", "mariadb", or "postgres". Defaults to "postgres" when absent.
func parseDBType(desc string) string {
	re := regexp.MustCompile(`(?i)\btype:\s*([^\s,]+)`)
	if m := re.FindStringSubmatch(desc); len(m) >= 2 {
		t := strings.ToLower(strings.TrimSpace(m[1]))
		switch t {
		case "mysql", "mariadb":
			return t
		}
	}
	return "postgres"
}
