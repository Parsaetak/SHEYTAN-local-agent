package lab

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrPolicyDenied         = errors.New("lab: command denied by policy")
	ErrNetworkDenied        = errors.New("lab: network access denied by policy")
	ErrDangerousCommand     = errors.New("lab: dangerous command denied by policy")
	ErrCommandTooLong       = errors.New("lab: command exceeds policy length limit")
	ErrInteractiveCommand   = errors.New("lab: interactive command denied by policy")
	ErrWorkspaceEscape      = errors.New("lab: command may escape workspace boundary")
)

// Policy controls which commands the autonomous Coding Lab may execute.
//
// Policy is intentionally a defense-in-depth layer. It does not replace OS
// isolation, filesystem permissions, containers, Windows Job Objects, or any
// future stronger sandbox implementation.
type Policy struct {
	// AllowNetwork permits commands that are recognized as network-capable.
	AllowNetwork bool

	// MaxCommandLength prevents extremely large shell command payloads.
	// Zero uses the default of 16 KiB.
	MaxCommandLength int

	// AllowInteractive permits commands that appear to require interactive
	// terminal input. This should normally remain false for autonomous runs.
	AllowInteractive bool

	// AllowDangerous permits commands classified as destructive or system-level.
	// This should normally remain false.
	AllowDangerous bool

	// AllowWorkspaceEscape permits commands containing obvious path traversal
	// patterns. This should normally remain false.
	AllowWorkspaceEscape bool
}

// DefaultPolicy returns the conservative Coding Lab policy.
func DefaultPolicy() Policy {
	return Policy{
		AllowNetwork:         false,
		MaxCommandLength:     16 * 1024,
		AllowInteractive:     false,
		AllowDangerous:       false,
		AllowWorkspaceEscape: false,
	}
}

// Evaluate checks whether a command is permitted.
//
// The command remains a shell command and therefore cannot be made perfectly
// safe through string inspection alone. This function deliberately focuses on
// high-confidence dangerous patterns and should be combined with workspace
// isolation and process-level controls.
func (p Policy) Evaluate(command string) error {
	command = strings.TrimSpace(command)

	if command == "" {
		return ErrCommandEmpty
	}

	if len(command) > p.maxCommandLength() {
		return ErrCommandTooLong
	}

	normalized := normalizeCommand(command)

	if !p.AllowDangerous && containsDangerousPattern(normalized) {
		return fmt.Errorf("%w: %s", ErrDangerousCommand, summarize(command))
	}

	if !p.AllowNetwork && containsNetworkPattern(normalized) {
		return fmt.Errorf("%w: %s", ErrNetworkDenied, summarize(command))
	}

	if !p.AllowInteractive && containsInteractivePattern(normalized) {
		return fmt.Errorf("%w: %s", ErrInteractiveCommand, summarize(command))
	}

	if !p.AllowWorkspaceEscape && containsWorkspaceEscapePattern(normalized) {
		return fmt.Errorf("%w: %s", ErrWorkspaceEscape, summarize(command))
	}

	return nil
}

// EvaluateForWorkspace performs policy validation with additional checks for
// an explicitly supplied working directory.
//
// The actual filesystem boundary is still enforced by Workspace.PathFor.
func (p Policy) EvaluateForWorkspace(command, workingDir string) error {
	if err := p.Evaluate(command); err != nil {
		return err
	}

	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return nil
	}

	if filepath.IsAbs(workingDir) {
		return fmt.Errorf("%w: absolute working directory %q", ErrWorkspaceEscape, workingDir)
	}

	cleaned := filepath.Clean(workingDir)
	if cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: working directory %q", ErrWorkspaceEscape, workingDir)
	}

	return nil
}

func (p Policy) maxCommandLength() int {
	if p.MaxCommandLength <= 0 {
		return 16 * 1024
	}

	return p.MaxCommandLength
}

func normalizeCommand(command string) string {
	command = strings.ToLower(command)

	// Normalize common separators so pattern matching catches both Windows
	// and Unix-style forms.
	command = strings.ReplaceAll(command, "\\", "/")

	// Collapse repeated whitespace.
	command = strings.Join(strings.Fields(command), " ")

	return command
}

func containsDangerousPattern(command string) bool {
	patterns := []string{
		// Filesystem destruction.
		"rm -rf /",
		"rm -r /",
		"rm --recursive --force /",
		"rmdir /s /q",
		"del /f /s /q",
		"rd /s /q",
		"format ",
		"format.com",
		"diskpart",
		"mkfs",
		"dd if=",

		// System shutdown/reboot/power operations.
		"shutdown ",
		"shutdown.exe",
		"restart-computer",
		"stop-computer",
		"halt",
		"poweroff",
		"reboot",

		// Windows service/system-management destruction.
		"sc delete",
		"sc.exe delete",
		"bcdedit",
		"takeown ",
		"icacls ",
		"cipher /w",
		"wevtutil cl",

		// Registry destruction.
		"reg delete",
		"reg.exe delete",

		// Process-wide destructive actions.
		"taskkill /f /im",
		"taskkill.exe /f /im",
		"kill -9 -1",
		"killall",

		// Git operations that can destroy or publish source state.
		"git reset --hard",
		"git clean -fd",
		"git clean -xdf",
		"git push",
		"git push --force",
		"git push -f",

		// Generic privilege escalation.
		"sudo ",
		"runas ",
		"psexec ",
	}

	for _, pattern := range patterns {
		if matchesTokenLike(command, pattern) {
			return true
		}
	}

	return false
}

func containsNetworkPattern(command string) bool {
	patterns := []string{
		// HTTP clients.
		"curl ",
		"curl.exe",
		"wget ",
		"wget.exe",
		"invoke-webrequest",
		"invoke-restmethod",
		"irm ",
		"iwr ",

		// Git network operations.
		"git clone",
		"git fetch",
		"git pull",
		"git push",
		"git remote add",

		// Package managers frequently requiring network access.
		"npm install",
		"npm i ",
		"npm update",
		"npm ci",
		"pnpm install",
		"pnpm add",
		"pnpm update",
		"yarn install",
		"yarn add",
		"pip install",
		"pip3 install",
		"python -m pip install",
		"python3 -m pip install",
		"go get ",
		"go install ",
		"cargo install",
		"cargo add",

		// Common package/download utilities.
		"powershell -command",
		"powershell.exe -command",
		"bitsadmin",
		"certutil -urlcache",
	}

	for _, pattern := range patterns {
		if matchesTokenLike(command, pattern) {
			return true
		}
	}

	// Explicit URL schemes are strong evidence of network access.
	for _, scheme := range []string{
		"http://",
		"https://",
		"ftp://",
		"ssh://",
		"git://",
	} {
		if strings.Contains(command, scheme) {
			return true
		}
	}

	return false
}

func containsInteractivePattern(command string) bool {
	patterns := []string{
		// Interactive Git flows.
		"git add -p",
		"git commit",
		"git rebase -i",
		"git merge --continue",
		"git cherry-pick --continue",

		// Shell/terminal editors.
		"vim ",
		"vi ",
		"nvim ",
		"nano ",
		"emacs ",
		"notepad ",
		"notepad.exe",

		// Windows interactive shells.
		"cmd.exe /k",
		"powershell -interactive",
		"powershell.exe -interactive",

		// Common interactive programs.
		"ssh ",
		"telnet ",
		"ftp ",
		"mysql ",
		"psql ",
		"sqlite3 ",

		// Explicit input-oriented constructs.
		"read -p",
		"read ",
	}

	for _, pattern := range patterns {
		if matchesTokenLike(command, pattern) {
			return true
		}
	}

	return false
}

func containsWorkspaceEscapePattern(command string) bool {
	if runtime.GOOS == "windows" {
		// Windows shell commands can switch drives or use absolute paths.
		if containsDrivePath(command) {
			return true
		}
	}

	// Catch common parent traversal sequences. This is intentionally
	// conservative; commands that genuinely need traversal should explicitly
	// opt into a stronger policy later rather than bypass this layer silently.
	if strings.Contains(command, "../") ||
		strings.Contains(command, "..\\") ||
		strings.Contains(command, "/..") ||
		strings.Contains(command, "\\..") {
		return true
	}

	// Common Unix absolute filesystem roots.
	for _, prefix := range []string{
		" /etc/",
		" /usr/",
		" /var/",
		" /home/",
		" /root/",
		" /tmp/",
		" /dev/",
		" /proc/",
		" /sys/",
		" >/etc/",
		" >/usr/",
		" >/var/",
		" >/home/",
		" >/root/",
	} {
		if strings.Contains(" "+command, prefix) {
			return true
		}
	}

	// Obvious Windows system locations.
	for _, path := range []string{
		"c:/windows/",
		"c:/program files/",
		"c:/program files (x86)/",
		"c:/users/",
	} {
		if strings.Contains(command, path) {
			return true
		}
	}

	return false
}

func containsDrivePath(command string) bool {
	for i := 0; i+2 < len(command); i++ {
		ch := command[i]

		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
			continue
		}

		if command[i+1] == ':' &&
			(command[i+2] == '/' || command[i+2] == '\\') {
			return true
		}
	}

	return false
}

func matchesTokenLike(command, pattern string) bool {
	pattern = strings.TrimSpace(pattern)

	if pattern == "" {
		return false
	}

	if command == pattern {
		return true
	}

	if strings.HasPrefix(command, pattern+" ") ||
		strings.HasPrefix(command, pattern+";") ||
		strings.HasPrefix(command, pattern+"&&") ||
		strings.HasPrefix(command, pattern+"||") ||
		strings.HasPrefix(command, commandSeparator(pattern)) {
		return true
	}

	// Catch commands appearing after shell separators.
	separators := []string{
		"&&",
		"||",
		";",
		"|",
		"(",
		")",
		">",
		"<",
	}

	for _, separator := range separators {
		candidate := separator + pattern
		if strings.Contains(command, candidate) {
			return true
		}

		candidate = separator + " " + pattern
		if strings.Contains(command, candidate) {
			return true
		}
	}

	// Also catch a quoted command beginning.
	if strings.Contains(command, "\""+pattern+"\"") ||
		strings.Contains(command, "'"+pattern+"'") {
		return true
	}

	return false
}

func commandSeparator(pattern string) string {
	if strings.HasSuffix(pattern, " ") {
		return strings.TrimSpace(pattern)
	}

	return pattern
}

func summarize(command string) string {
	command = strings.TrimSpace(command)

	const max = 120

	if len(command) <= max {
		return command
	}

	return command[:max] + "..."
}
