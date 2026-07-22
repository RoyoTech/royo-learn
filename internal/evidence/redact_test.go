package evidence

import (
	"bytes"
	"testing"
)

func TestRedactBasic(t *testing.T) {
	t.Parallel()

	input := []byte("OPENAI_API_KEY=sk-1234567890abcdef")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED")) {
		t.Errorf("expected redacted output to contain [REDACTED], got %q", string(redacted))
	}
	if bytes.Contains(redacted, []byte("sk-1234567890abcdef")) {
		t.Error("secret was NOT redacted")
	}
}

func TestRedactWithKnownSecrets(t *testing.T) {
	t.Parallel()

	knownSecrets := []string{"my-super-secret-token", "password123"}
	input := []byte("Using token my-super-secret-token and password123 to login")

	redacted := Redact(input, knownSecrets)

	if !bytes.Contains(redacted, []byte("[REDACTED:known]")) {
		t.Errorf("expected [REDACTED:known], got %q", string(redacted))
	}
	if bytes.Contains(redacted, []byte("my-super-secret-token")) {
		t.Error("known secret was NOT redacted")
	}
	if bytes.Contains(redacted, []byte("password123")) {
		t.Error("known secret was NOT redacted")
	}
}

func TestRedactOpenAIKey(t *testing.T) {
	t.Parallel()

	input := []byte(`Authorization: Bearer sk-proj-abc123def456`)
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:openai_key]")) {
		t.Errorf("expected [REDACTED:openai_key], got %q", string(redacted))
	}
}

func TestRedactGitHubToken(t *testing.T) {
	t.Parallel()

	input := []byte("GITHUB_TOKEN=ghp_1234567890abcdefghijklmnop")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:github_token]")) {
		t.Errorf("expected [REDACTED:github_token], got %q", string(redacted))
	}
}

func TestRedactAWSAccessKey(t *testing.T) {
	t.Parallel()

	input := []byte("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE with secret")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:aws_key]")) {
		t.Errorf("expected [REDACTED:aws_key], got %q", string(redacted))
	}
}

func TestRedactPrivateKey(t *testing.T) {
	t.Parallel()

	input := []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:private_key]")) {
		t.Errorf("expected [REDACTED:private_key], got %q", string(redacted))
	}
}

func TestRedactQuotedColonAssignmentsAndEncryptedPrivateKey(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`TOKEN: "quoted-secret-value"`,
		`"api_key": "json-secret-value"`,
		`PASSWORD = 'single-quoted-secret'`,
		"-----BEGIN ENCRYPTED PRIVATE KEY-----\nbase64-private-material\n-----END ENCRYPTED PRIVATE KEY-----",
	}
	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			redacted := Redact([]byte(input), nil)
			if bytes.Equal(redacted, []byte(input)) {
				t.Fatalf("input was not redacted: %q", input)
			}
			if bytes.Contains(redacted, []byte("quoted-secret-value")) ||
				bytes.Contains(redacted, []byte("json-secret-value")) ||
				bytes.Contains(redacted, []byte("single-quoted-secret")) ||
				bytes.Contains(redacted, []byte("base64-private-material")) {
				t.Fatalf("secret material reached output: %q", redacted)
			}
		})
	}
}

func TestRedactPasswordsInURL(t *testing.T) {
	t.Parallel()

	input := []byte("Database URL: postgres://user:secretpassword@localhost:5432/db")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:password_url]")) {
		t.Errorf("expected [REDACTED:password_url], got %q", string(redacted))
	}
}

func TestRedactBearerToken(t *testing.T) {
	t.Parallel()

	input := []byte("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgN-c-7MGj1w6P")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:bearer_token]")) {
		t.Errorf("expected [REDACTED:bearer_token], got %q", string(redacted))
	}
}

func TestRedactConnectionString(t *testing.T) {
	t.Parallel()

	input := []byte(`server=localhost;database=mydb;user=sa;password=S0m3$ecureP@ss`)
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:connection_string]")) {
		t.Errorf("expected [REDACTED:connection_string], got %q", string(redacted))
	}
}

func TestRedactCookies(t *testing.T) {
	t.Parallel()

	input := []byte("Cookie: session_id=super-secret; theme=dark")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:cookie]")) {
		t.Errorf("expected [REDACTED:cookie], got %q", string(redacted))
	}
	if bytes.Contains(redacted, []byte("super-secret")) {
		t.Error("cookie value was NOT redacted")
	}
}

func TestRedactPrivateTags(t *testing.T) {
	t.Parallel()

	input := []byte("before <private>reasoning and secret data</private> after")
	redacted := Redact(input, nil)

	if !bytes.Contains(redacted, []byte("[REDACTED:private_tag]")) {
		t.Errorf("expected [REDACTED:private_tag], got %q", string(redacted))
	}
	if bytes.Contains(redacted, []byte("reasoning and secret data")) {
		t.Error("private tag content was NOT redacted")
	}
}

func TestRedactSafeContentUnchanged(t *testing.T) {
	t.Parallel()

	input := []byte("This is normal content without any secrets.")
	redacted := Redact(input, nil)

	if !bytes.Equal(redacted, input) {
		t.Errorf("safe content was modified: %q", string(redacted))
	}
}

func TestDetectSecrets(t *testing.T) {
	t.Parallel()

	secrets := DetectSecrets([]byte(`
		OPENAI_API_KEY=sk-proj-test12345
		GITHUB_TOKEN=ghp_abcdef1234567890
		AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
	`))

	if len(secrets) < 3 {
		t.Fatalf("DetectSecrets returned %d secrets, want at least 3", len(secrets))
	}
}

func TestDetectSecretsEmpty(t *testing.T) {
	t.Parallel()

	secrets := DetectSecrets([]byte("No secrets here, just normal text."))
	if len(secrets) != 0 {
		t.Errorf("DetectSecrets found %d secrets in safe content", len(secrets))
	}
}
