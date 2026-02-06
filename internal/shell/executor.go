package shell

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// blockedPatterns are substrings that are never allowed in commands.
var blockedPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	"mkfs",
	"dd if=",
	":(){",           // fork bomb
	"chmod -R 777",
	"wget", "curl",   // no downloading
	"> /dev/sd",
	"shutdown",
	"reboot",
	"halt",
	"init 0",
	"init 6",
	"passwd",
	"adduser",
	"useradd",
	"userdel",
	"visudo",
	"iptables",
	"nft ",
	"systemctl disable",
	"systemctl mask",
}

// Executor runs shell commands with safety checks and timeouts.
type Executor struct {
	timeout   time.Duration
	maxOutput int
}

// New creates a shell executor.
func New(timeout time.Duration, maxOutput int) *Executor {
	return &Executor{
		timeout:   timeout,
		maxOutput: maxOutput,
	}
}

// Run executes a command and returns its combined output, truncated to maxOutput.
func (e *Executor) Run(ctx context.Context, command string) (string, error) {
	if blocked := checkBlocked(command); blocked != "" {
		return "", fmt.Errorf("blocked command pattern: %q", blocked)
	}

	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()

	result := string(out)
	if len(result) > e.maxOutput {
		result = result[:e.maxOutput] + "\n... [output truncated]"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %s", e.timeout)
	}

	if err != nil {
		return result, fmt.Errorf("command failed: %w", err)
	}

	return result, nil
}

func checkBlocked(command string) string {
	lower := strings.ToLower(command)
	for _, pattern := range blockedPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return pattern
		}
	}
	return ""
}
