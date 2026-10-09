package cli

import (
	"strings"
	"testing"

	"github.com/Mvnshi/legatus/internal/agent/claude"
)

func TestDoctorSaysWhichClaudeCodeVersionWasRunForReal(t *testing.T) {
	mark, text := claudeTrial(claude.VerifiedVersion+" (Claude Code)", "linux")
	if mark != "ok" || !strings.Contains(text, "run for real") || strings.Contains(text, "trial") {
		t.Fatalf("the verified version on Linux: %q %q", mark, text)
	}
	for _, c := range []struct{ version, goos string }{
		{"2.0.1 (Claude Code)", "linux"},
		{claude.VerifiedVersion + " (Claude Code)", "windows"},
		{claude.VerifiedVersion + " (Claude Code)", "darwin"},
		{"", "linux"},
	} {
		mark, text := claudeTrial(c.version, c.goos)
		if mark != "--" || !strings.Contains(text, "trial") || !strings.Contains(text, claude.VerifiedVersion) {
			t.Errorf("%+v must be reported as a trial that names the verified version: %q %q", c, mark, text)
		}
	}
}
