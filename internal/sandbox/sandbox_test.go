package sandbox

import (
	"runtime"
	"strings"
	"testing"
)

// The secrets below are built from pieces so that no secret scanner flags this file.
var (
	ghToken  = "gh" + "p_" + strings.Repeat("a1B2", 10)
	awsKey   = "AK" + "IA" + "ABCDEFGHIJKLMNOP"
	apiKey   = "sk" + "-proj-" + strings.Repeat("Zz9", 12)
	jwt      = "ey" + "Jhbgciojhhh1234." + "ey" + "Jzdwiioxhh12345." + "abcDEF123456xyz"
	pemBlock = "-----BEGIN RSA " + "PRIVATE KEY-----\nMIIEvQIBADANBgkq\nabc\n-----END RSA " + "PRIVATE KEY-----"
)

func TestRedactRemovesKnownSecretsAndKeepsTheRest(t *testing.T) {
	in := "Fix the login. Use " + ghToken + " for the API; aws " + awsKey + ", key " + apiKey + "\n" + pemBlock + "\nthen " + jwt
	out, findings := Redact(in, false)
	for _, secret := range []string{ghToken, awsKey, apiKey, "MIIEvQIBADANBgkq", jwt} {
		if strings.Contains(out, secret) {
			t.Errorf("secret %q survived:\n%s", secret[:8], out)
		}
	}
	if !strings.HasPrefix(out, "Fix the login. Use [REDACTED:github-token] for the API") {
		t.Errorf("the surrounding text changed: %q", out)
	}
	got := map[string]int{}
	for _, f := range findings {
		got[f.Kind] = f.Count
	}
	for _, kind := range []string{"github-token", "aws-access-key", "api-key", "private-key", "jwt"} {
		if got[kind] != 1 {
			t.Errorf("findings[%s] = %d, want 1 (%v)", kind, got[kind], findings)
		}
	}
}

func TestRedactAssignmentsReplaceOnlyTheValue(t *testing.T) {
	out, findings := Redact(`config: password = "hunter2hunter2hunter2" and API_KEY: abcdefghijklmnop12`, false)
	if strings.Contains(out, "hunter2") || strings.Contains(out, "abcdefghijklmnop12") {
		t.Fatalf("value survived: %s", out)
	}
	if !strings.Contains(out, "password") || !strings.Contains(out, "API_KEY") {
		t.Fatalf("the names should stay readable: %s", out)
	}
	if len(findings) != 1 || findings[0].Count != 2 {
		t.Fatalf("findings = %v", findings)
	}
}

func TestRedactLeavesOrdinaryTextAndShortValuesAlone(t *testing.T) {
	in := "Rename token_count to total_tokens; the password field is 8 chars; see sk-learn docs and the README."
	out, findings := Redact(in, false)
	if out != in || len(findings) != 0 {
		t.Fatalf("changed ordinary text: %q %v", out, findings)
	}
}

func TestRedactIsIdempotent(t *testing.T) {
	once, _ := Redact("token="+ghToken, false)
	twice, findings := Redact(once, false)
	if once != twice || len(findings) != 0 {
		t.Fatalf("second pass changed %q to %q (%v)", once, twice, findings)
	}
}

func TestEmailsOnlyWhenAsked(t *testing.T) {
	in := "Ask alice@example.com about it."
	if out, _ := Redact(in, false); out != in {
		t.Fatalf("emails were redacted without pii: %q", out)
	}
	out, findings := Redact(in, true)
	if out != "Ask [REDACTED:email] about it." || len(findings) != 1 {
		t.Fatalf("pii = %q %v", out, findings)
	}
}

func TestEnvDropsSecretsKeepsBaseAndHonoursOptIn(t *testing.T) {
	parent := []string{
		"PATH=/bin", "HOME=/home/u", "GITHUB_TOKEN=abc", "AWS_SECRET_ACCESS_KEY=xyz", "OPENAI_API_KEY=k",
		"MY_SERVICE_TOKEN=t", "LANG=C", "=C:=C:\\work",
	}
	env := Env(parent, []string{"MY_SERVICE_TOKEN"}, map[string]string{"CODEX_HOME": "/pool/a"})
	has := func(name string) bool {
		for _, kv := range env {
			n, _, _ := strings.Cut(kv, "=")
			if n == name {
				return true
			}
		}
		return false
	}
	for _, name := range []string{"PATH", "HOME", "LANG", "MY_SERVICE_TOKEN", "CODEX_HOME"} {
		if !has(name) {
			t.Errorf("%s should be present: %v", name, env)
		}
	}
	for _, name := range []string{"GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "OPENAI_API_KEY"} {
		if has(name) {
			t.Errorf("%s leaked into the agent's environment", name)
		}
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "=") {
			t.Errorf("malformed entry kept: %q", kv)
		}
	}
}

func TestExplicitValuesWinOverInheritedOnes(t *testing.T) {
	env := Env([]string{"HOME=/old"}, nil, map[string]string{"HOME": "/new"})
	if len(env) != 1 || env[0] != "HOME=/new" {
		t.Fatalf("env = %v", env)
	}
}

func TestWindowsNamesAreCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows environment names only")
	}
	env := Env([]string{"Path=C:\\Windows", "SystemRoot=C:\\Windows", "github_token=x"}, nil, nil)
	if len(env) != 2 {
		t.Fatalf("env = %v", env)
	}
}
