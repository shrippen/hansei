package redact

import (
	"strings"
	"testing"
)

func TestFindKeyValue(t *testing.T) {
	cases := map[string]bool{
		"- Passwort: geheim123":                          true,
		"APP_KEY=base64:abcdefghijkl":                    true,
		"| Admin-Passwort | Elbfeuer-2026! |":            true,
		"| Passwort | siehe Vaultwarden |":               false,
		"| Passwort | Wird beim Setup gesetzt |":         false,
		"**Passwort:** Elbfeuer-2026!":                   true,
		"- Passwort: siehe Vaultwarden: Nextcloud":       false,
		"- APP_KEY=${APP_KEY}":                           false,
		"Token: <aus Gitea>":                             false,
		"password: ****":                                 false,
		"db_password: \"hunter2hunter2\"":                true,
		"Kein Geheimnis hier, nur Text über Passwörter.": false,
	}
	for line, want := range cases {
		got := len(Find(line)) > 0
		if got != want {
			t.Errorf("%q: got %v, want %v (%v)", line, got, want, Find(line))
		}
	}
}

func TestFindFormats(t *testing.T) {
	text := "key sk-ant-api03-abcdefghijklmnopqrstuvwx and https://admin:s3cretpw@nas.local/share\n" +
		"-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----"
	m := Find(text)
	if len(m) != 3 {
		t.Fatalf("want 3 matches, got %+v", m)
	}
	if text[m[1].Start:m[1].End] != "s3cretpw" {
		t.Errorf("url creds match %q", text[m[1].Start:m[1].End])
	}
}

func TestMask(t *testing.T) {
	got := Mask("- Passwort: geheim123")
	if got != "- Passwort: •••••••••" {
		t.Errorf("mask = %q", got)
	}
}

func TestRedactRestore(t *testing.T) {
	r := New()
	text := "Passwort: geheim123\nnochmal Passwort: geheim123\nToken: abcdefgh99"
	red := r.Redact(text)
	if strings.Contains(red, "geheim123") || strings.Contains(red, "abcdefgh99") {
		t.Fatalf("secret leaked: %s", red)
	}
	if r.Count() != 2 {
		t.Errorf("count = %d", r.Count())
	}
	if !strings.Contains(red, "⟦GEHEIM_1⟧") || strings.Count(red, "⟦GEHEIM_1⟧") != 2 {
		t.Errorf("placeholders: %s", red)
	}
	if back := r.Restore(red); back != text {
		t.Errorf("restore = %q", back)
	}
	if !r.Placeholder("x ⟦GEHEIM_9⟧") || r.Placeholder("x ⟦GEHEIM_1⟧") {
		t.Error("unknown placeholder detection wrong")
	}
}

func TestValueToEndOfLine(t *testing.T) {
	got := Mask("MAIL_SMTP_PWD=abcd einn nzyk mkcv ozx")
	if strings.Contains(got, "einn") || strings.Contains(got, "ozx") {
		t.Errorf("value with spaces only partly masked: %q", got)
	}
	got = Mask("EMAIL_HOST_PASSWORD = 'hunter2hunter2'    # password")
	if !strings.HasSuffix(got, "# password") || strings.Contains(got, "hunter2") {
		t.Errorf("comment handling: %q", got)
	}
}

func TestEmptyValueNextLine(t *testing.T) {
	text := "DB_PASSWD=\nport: 5432\nNEXTAUTH_SECRET_VALUE=\nDB_USER=kaizoku"
	if m := Find(text); len(m) != 0 {
		t.Errorf("empty values matched the next line: %+v", m)
	}
}

func TestURLCredentialReference(t *testing.T) {
	if m := Find("DATABASE_URL: postgresql://${DB_USER}:${DB_PASSWORD}@db:5432/x"); len(m) != 0 {
		t.Errorf("reference in URL credentials matched: %+v", m)
	}
	if m := Find("DATABASE_URL: mysql://umami:s3cretpass@db/umami"); len(m) != 1 {
		t.Errorf("URL password missed: %+v", m)
	}
}

func TestSettingKeysAreNotSecrets(t *testing.T) {
	for _, line := range []string{
		"token_endpoint_auth_method: client_secret_post",
		"OIDC_TOKEN_URI=https://auth.example.test/application/o/token/",
		"token_url = https://auth.example.test/token",
		"PASSWORD_FILE=/run/secrets/db_password",
		"SECRET_KEY_LENGTH=64",
	} {
		if m := Find(line); len(m) != 0 {
			t.Errorf("%q matched: %+v", line, m)
		}
	}
	if m := Find("DISCORD_WEBHOOK_URL=https://discord.example.test/api/webhooks/123/abcdef"); len(m) != 1 {
		t.Error("webhook URL is a secret")
	}
}
