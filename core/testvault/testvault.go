// Package testvault builds small vaults and configs for tests.
package testvault

import (
	"os"
	"path/filepath"
	"testing"

	"git.arianw.de/shrippen/hansei/core/config"
)

// Files is a small IT documentation with one problem of each kind.
var Files = map[string]string{
	"IT/Design.md":                      "# Konventionen\n\nHostnamen: Leuchtturm-Namen. Keine Klartext-Secrets.\n",
	"agent.md":                          "# Agent\n\nÄndere nur, was der Auftrag verlangt.\n",
	"IT/Dienste/Nebelhorn/Nextcloud.md": "---\nGerät: \"[[Nebelhorn]]\"\nletzte Prüfung: 2025-01-10\n---\n# Nextcloud\n\nLäuft auf SW-NAS01.\n\n## Zugang\n- Passwort: Elbfeuer-2026!\n\n## Backup\nRestic nach [[Borg Backup Server]].\n",
	"IT/Dienste/Nebelhorn/Immich.md":    "---\nGerät: \"[[Nebelhorn]]\"\nBackup via: Borg\nletzte Prüfung: 2026-09-01\n---\n# Immich\n\n```yaml\nservices:\n  immich:\n    image: ghcr.io/immich-app/immich-server:release\n```\n",
	"IT/Geräte/Nebelhorn.md":            "---\nIPs: [192.168.10.5]\n---\n# Nebelhorn\n\nNAS im Studio. Früher SW-NAS01.\n",
	"IT/Dienste/deprecated/Alt.md":      "---\ndeprecated: true\n---\n# Alt\n\nLief auf SW-NAS01.\n",
	"Projekte/Harbour Lights.md":        "# Harbour Lights\n\nKein IT-Thema.\n",
	"Tagebuch/2026-09-20.md":            "# Tagebuch\n\nPrivat. Passwort: nichtlesen123\n",
	"IT/Anleitungen/Restore.md":         "# Restore\n\nSiehe [[Harbour Lights]] und [[2026-09-20]] und [[Gibt es nicht]].\n",
}

// New writes Files into a temp dir and returns a matching config.
func New(t testing.TB) *config.Config {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "vault")
	for rel, content := range Files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := config.New(filepath.Join(dir, "config.yaml"), root)
	c.Allow = []string{"IT"}
	c.Block = []string{"Tagebuch"}
	c.DefaultRules = []string{"agent.md"}
	c.Rulebooks = []config.Rulebook{{Folder: "IT", Files: []string{"IT/Design.md", "agent.md"}}}
	c.Checks.Codenames = map[string]string{"SW-NAS01": "Nebelhorn"}
	c.Checks.Required = map[string][]string{"IT/Dienste": {"Gerät", "Backup via"}}
	c.Checks.ReviewField = "letzte Prüfung"
	c.Checks.ReviewDays = 180
	return c
}
