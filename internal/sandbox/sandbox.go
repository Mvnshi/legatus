// Package sandbox decides what an agent process is allowed to see: which environment variables it
// inherits, and which secrets are removed from a prompt before it leaves the machine.
//
// This is a policy layer, not an operating-system sandbox. It does not stop an agent that is allowed to
// run commands from reading files the account can read; that needs the agent's own sandbox (which the
// backends turn on) or a container.
package sandbox

import (
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Finding is one kind of secret that was removed, and how many times. It never holds the secret.
type Finding struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type rule struct {
	kind string
	re   *regexp.Regexp
	// group is the capture group to replace; 0 replaces the whole match.
	group int
}

var rules = []rule{
	{"private-key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), 0},
	{"github-token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`), 0},
	{"github-token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`), 0},
	{"aws-access-key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), 0},
	{"api-key", regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{20,}\b`), 0},
	{"stripe-key", regexp.MustCompile(`\b[sr]k_(?:live|test)_[0-9a-zA-Z]{20,}\b`), 0},
	{"slack-token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), 0},
	{"google-api-key", regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`), 0},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`), 0},
	{"bearer-token", regexp.MustCompile(`(?i)\bbearer\s+([A-Za-z0-9\-._~+/]{20,}=*)`), 1},
	// name = value, where the name says it is a secret. Only the value is replaced.
	{"secret-assignment", regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret(?:[_-]?key)?|access[_-]?token|auth[_-]?token|token|passw(?:or)?d|client[_-]?secret)\b["']?\s*[:=]\s*["']?([A-Za-z0-9_\-./+=]{12,})`), 1},
}

var emailRule = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)

// Redact replaces secrets in text with [REDACTED:<kind>] and reports what it replaced. With pii set it
// also replaces email addresses.
func Redact(text string, pii bool) (string, []Finding) {
	counts := map[string]int{}
	apply := func(kind string, re *regexp.Regexp, group int) {
		text = re.ReplaceAllStringFunc(text, func(m string) string {
			if strings.Contains(m, "[REDACTED:") {
				return m
			}
			counts[kind]++
			if group == 0 {
				return "[REDACTED:" + kind + "]"
			}
			sub := re.FindStringSubmatchIndex(m)
			if sub == nil || len(sub) < 2*(group+1) || sub[2*group] < 0 {
				return "[REDACTED:" + kind + "]"
			}
			return m[:sub[2*group]] + "[REDACTED:" + kind + "]" + m[sub[2*group+1]:]
		})
	}
	for _, r := range rules {
		apply(r.kind, r.re, r.group)
	}
	if pii {
		apply("email", emailRule, 0)
	}
	var findings []Finding
	for kind, n := range counts {
		findings = append(findings, Finding{Kind: kind, Count: n})
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Kind < findings[j].Kind })
	return text, findings
}

// baseEnv is what a coding agent needs to start and find its tools; nothing in it is a secret.
var baseEnv = []string{
	"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC", "TEMP", "TMP", "TMPDIR", "HOME", "USERPROFILE",
	"HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)",
	"USER", "USERNAME", "LOGNAME", "SHELL", "LANG", "LC_ALL", "TERM", "COLORTERM", "NUMBER_OF_PROCESSORS",
	"PROCESSOR_ARCHITECTURE", "OS", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS",
}

var secretName = regexp.MustCompile(`(?i)(token|secret|passw|credential|private|api[_-]?key|auth)`)

func sameName(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Env builds the environment for an agent process: the base variables, then any names the step opted in
// to (even secret-looking ones, because the user asked), then the explicit values in set, which win. Any
// other inherited variable is dropped, in particular GITHUB_TOKEN and cloud credentials.
func Env(parent []string, optIn []string, set map[string]string) []string {
	allowed := func(name string) bool {
		for _, n := range baseEnv {
			if sameName(n, name) && !secretName.MatchString(name) {
				return true
			}
		}
		for _, n := range optIn {
			if sameName(n, name) {
				return true
			}
		}
		return false
	}
	var out []string
	index := map[string]int{}
	put := func(name, value string) {
		key := name
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(name)
		}
		if i, ok := index[key]; ok {
			out[i] = name + "=" + value
			return
		}
		index[key] = len(out)
		out = append(out, name+"="+value)
	}
	for _, kv := range parent {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue // Windows keeps entries like "=C:=C:\\" for drive directories
		}
		if allowed(name) {
			put(name, value)
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		put(name, set[name])
	}
	return out
}

// Summary is a one-line description of redaction findings for logs.
func Summary(f []Finding) string {
	if len(f) == 0 {
		return "nothing redacted"
	}
	parts := make([]string, len(f))
	for i, x := range f {
		parts[i] = fmt.Sprintf("%s ×%d", x.Kind, x.Count)
	}
	return strings.Join(parts, ", ")
}
