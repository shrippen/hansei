// Package redact finds secrets in notes, masks them for display and swaps them for
// placeholders before text leaves the machine.
//
//	note text ──Redact──▶ "Passwort: ⟦GEHEIM_1⟧" ──AI──▶ answer ──Restore──▶ note text
package redact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Kind names what a match looks like.
type Kind string

const (
	KindKeyValue   Kind = "key-value"   // "Passwort: xyz", "API_KEY=xyz"
	KindToken      Kind = "token"       // known token formats (sk-ant-…, ghp_…, AKIA…)
	KindPrivateKey Kind = "private-key" // PEM private key blocks
	KindURLCreds   Kind = "url-creds"   // https://user:pass@host
	KindJWT        Kind = "jwt"
	KindTable      Kind = "table" // "| Passwort | xyz |"
)

const (
	maskRune       = '•'
	placeholderFmt = "⟦GEHEIM_%d⟧"
	minValueLen    = 4
)

// Match is one secret value in a text (byte offsets).
type Match struct {
	Start int  `json:"start"`
	End   int  `json:"end"`
	Kind  Kind `json:"kind"`
}

var (
	// keyValueRe finds "key: value" or "KEY=value"; the value is the rest of the line (see lineValue).
	keyValueRe = regexp.MustCompile(`(?im)(?:^|[\s\-\*\|>"'(,;])((?:[a-z0-9_]*[_\- ]?)?(?:passwor[dt]|passphrase|kennwort|passwd|pwd|pass|secret|geheimnis|token|api[_\- ]?key|apikey|app[_\- ]?key|client[_\- ]?secret|access[_\- ]?key|private[_\- ]?key|auth[_\- ]?key|webhook[_\- ]?url)[a-z0-9_]*)[ \t]*(?:\*\*)?[ \t]*[:=][ \t]*(?:\*\*)?[ \t]*([^\n]*)`)
	tableRe    = regexp.MustCompile(`(?im)\|\s*(?:\*\*)?[^|\n]*(?:passwor[dt]|passphrase|kennwort|passwd|pwd|secret|geheimnis|token|api[_\- ]?key|app[_\- ]?key|client[_\- ]?secret|private[_\- ]?key)[^|\n]*?(?:\*\*)?\s*\|\s*` + "`?" + `([^|\n` + "`" + `]*?)` + "`?" + `\s*\|`)
	tokenRe    = regexp.MustCompile(`\b(?:sk-ant-[A-Za-z0-9_\-]{16,}|sk-[A-Za-z0-9]{32,}|gh[pousr]_[A-Za-z0-9]{30,}|glpat-[A-Za-z0-9_\-]{20,}|xox[baprs]-[A-Za-z0-9\-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_\-]{35})\b`)
	jwtRe      = regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`)
	urlCredsRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://[^\s:/@]+:([^\s@/]+)@`)
	pemRe      = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	// Keys that name a setting about a secret rather than the secret itself (…_URL, …_METHOD, …_FILE).
	settingKeyRe = regexp.MustCompile(`(?i)(?:ur[il]|endpoint|method|type|path|file|dir|name|user|username|mode|length|lifetime|expiry|expires|timeout|header|enabled?|algorithm|provider|scope)$`)
	webhookRe    = regexp.MustCompile(`(?i)webhook`)
	// Values that are references, not secrets: ${ENV}, <…>, "siehe Vaultwarden …", masks.
	referenceRe = regexp.MustCompile(`(?i)^(?:\$\{[^}]*\}?|\$[A-Z_]+|<[^>]*>?|\[\[.*|siehe|see|vaultwarden|bitwarden|keepass|keychain|kwallet|env|n/?a|none|keins?|leer|empty|x+|\*+|•+|\.\.\.|…|⟦.*|true|false|ja|nein|yes|no|required|erforderlich|optional|ssh|oauth2?|oidc|sso|totp|2fa|mfa|hash(ed)?|bcrypt|argon2|—|-+)`)
)

// Find returns all secret values in text, sorted and without overlaps.
func Find(text string) []Match {
	var out []Match
	for _, m := range pemRe.FindAllStringIndex(text, -1) {
		out = append(out, Match{m[0], m[1], KindPrivateKey})
	}
	for _, m := range tokenRe.FindAllStringIndex(text, -1) {
		out = append(out, Match{m[0], m[1], KindToken})
	}
	for _, m := range jwtRe.FindAllStringIndex(text, -1) {
		out = append(out, Match{m[0], m[1], KindJWT})
	}
	for _, m := range urlCredsRe.FindAllStringSubmatchIndex(text, -1) {
		if referenceRe.MatchString(text[m[2]:m[3]]) {
			continue
		}
		out = append(out, Match{m[2], m[3], KindURLCreds})
	}
	for _, m := range keyValueRe.FindAllStringSubmatchIndex(text, -1) {
		key := strings.TrimSpace(text[m[2]:m[3]])
		if settingKeyRe.MatchString(key) && !webhookRe.MatchString(key) {
			continue
		}
		from, to := lineValue(text, m[4], m[5])
		value := text[from:to]
		if len(value) < minValueLen || referenceRe.MatchString(value) {
			continue
		}
		out = append(out, Match{from, to, KindKeyValue})
	}
	for _, m := range tableRe.FindAllStringSubmatchIndex(text, -1) {
		value := strings.TrimSpace(text[m[2]:m[3]])
		if len(value) < minValueLen || referenceRe.MatchString(value) || strings.Contains(value, " ") {
			continue
		}
		out = append(out, Match{m[2], m[3], KindTable})
	}
	return merge(out)
}

// lineValue narrows the rest of a line to the value: without a trailing " # comment" or table
// cell border, without surrounding quotes. A value with spaces (app passwords) stays whole.
// Example: `'hunter2'    # password` → hunter2.
func lineValue(text string, from, to int) (int, int) {
	v := text[from:to]
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	if i := strings.Index(v, " |"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimRight(v, " \t\r")
	end := from + len(v)
	for _, q := range []string{"\"", "'", "`"} {
		if strings.HasPrefix(v, q) {
			if j := strings.Index(v[1:], q); j >= 0 {
				return from + 1, from + 1 + j
			}
			return from + 1, end
		}
	}
	return from, end
}

// merge sorts matches and drops those inside an earlier one.
func merge(in []Match) []Match {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Start != in[j].Start {
			return in[i].Start < in[j].Start
		}
		return in[i].End > in[j].End
	})
	var out []Match
	for _, m := range in {
		if n := len(out); n > 0 && m.Start < out[n-1].End {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Mask replaces every secret character with a dot, for display.
// Example: "Passwort: geheim123" → "Passwort: •••••••••".
func Mask(text string) string {
	matches := Find(text)
	if len(matches) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(text[last:m.Start])
		n := len([]rune(text[m.Start:m.End]))
		if m.Kind == KindPrivateKey {
			n = 12
		}
		b.WriteString(strings.Repeat(string(maskRune), n))
		last = m.End
	}
	b.WriteString(text[last:])
	return b.String()
}

// Redactor swaps secrets for numbered placeholders and back. One Redactor is used for
// a whole AI task, so the same secret always gets the same placeholder.
type Redactor struct {
	mu     sync.Mutex
	byVal  map[string]string
	byName map[string]string
}

// New returns an empty Redactor.
func New() *Redactor {
	return &Redactor{byVal: map[string]string{}, byName: map[string]string{}}
}

// Redact replaces secrets in text with placeholders.
func (r *Redactor) Redact(text string) string {
	matches := Find(text)
	if len(matches) == 0 {
		return text
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(text[last:m.Start])
		b.WriteString(r.name(text[m.Start:m.End]))
		last = m.End
	}
	b.WriteString(text[last:])
	return b.String()
}

func (r *Redactor) name(value string) string {
	if n, ok := r.byVal[value]; ok {
		return n
	}
	n := fmt.Sprintf(placeholderFmt, len(r.byVal)+1)
	r.byVal[value] = n
	r.byName[n] = value
	return n
}

// Restore puts the original values back where the AI kept a placeholder.
func (r *Redactor) Restore(text string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, value := range r.byName {
		text = strings.ReplaceAll(text, name, value)
	}
	return text
}

// Count returns how many distinct secrets were replaced.
func (r *Redactor) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byVal)
}

// Placeholder reports whether text still contains an unknown placeholder (the AI invented one).
func (r *Redactor) Placeholder(text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range placeholderRe.FindAllString(text, -1) {
		if _, ok := r.byName[m]; !ok {
			return true
		}
	}
	return false
}

var placeholderRe = regexp.MustCompile(`⟦GEHEIM_\d+⟧`)
