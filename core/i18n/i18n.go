// Package i18n holds the few texts the core itself produces (thread notes, toasts, errors
// shown to people). UIs translate their own strings.
package i18n

import (
	"fmt"
	"os"
	"strings"
)

// Lang returns "de" or "en" from the config value or the environment (LC_ALL, LC_MESSAGES, LANG).
func Lang(configured string) string {
	for _, v := range []string{configured, os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG"), os.Getenv("LANGUAGE")} {
		v = strings.ToLower(v)
		if strings.HasPrefix(v, "de") {
			return "de"
		}
		if strings.HasPrefix(v, "en") {
			return "en"
		}
	}
	return "en"
}

var catalog = map[string][2]string{ // key → {de, en}
	"written":          {"%s in den Tresor geschrieben", "%s written to the vault"},
	"skipped":          {"%s: alles abgelehnt, nichts geschrieben", "%s: everything rejected, nothing written"},
	"undone":           {"%s zurückgesetzt", "%s reverted"},
	"batchDone":        {"Batch abgeschlossen: %s", "Batch finished: %s"},
	"stale":            {"%s wurde seit dem Vorschlag geändert und die Änderung passt nicht mehr. Neu vorschlagen lassen.", "%s changed since the proposal and the change no longer fits. Ask for a new proposal."},
	"changedSince":     {"%s wurde nach dem Schreiben geändert, Rückgängig würde das überschreiben.", "%s changed after it was written; undo would overwrite that."},
	"interrupted":      {"Unterbrochen (Hansei wurde beendet).", "Interrupted (Hansei was closed)."},
	"failed":           {"KI-Lauf fehlgeschlagen: %s", "AI run failed: %s"},
	"noProposal":       {"Die KI hat keine Änderung vorgeschlagen.", "The AI proposed no changes."},
	"regenerate":       {"Bitte diese Änderung neu vorschlagen.", "Please propose this change again."},
	"ruleTask":         {"Nimm diese Konvention knapp als eine Zeile in %s auf, im passenden Abschnitt: %s", "Add this convention as one short line to %s, in the fitting section: %s"},
	"ruleTitle":        {"Regel aus Feedback", "Rule from feedback"},
	"repeated":         {"Du hast %d-mal Ähnliches angemerkt: „%s“. Als Regel aufnehmen?", "You gave similar feedback %d times: “%s”. Make it a rule?"},
	"findingTask":      {"Behebe diese Befunde der Prüfregel „%s“ in den genannten Notizen:\n%s", "Fix these findings of the check “%s” in the listed notes:\n%s"},
	"codenameTitle":    {"Codenamen ersetzen", "Replace code names"},
	"codenameSummary":  {"Alte Codenamen durch die aktuellen Namen ersetzt.", "Replaced old code names with the current names."},
	"codenameReason":   {"%s heißt jetzt %s.", "%s is now called %s."},
	"importSummary":    {"Aus review-queue/ übernommen.", "Imported from review-queue/."},
	"revisedReply":     {"Überarbeitet.", "Revised."},
	"localOnly":        {"%s darf nur an lokale Modelle gehen.", "%s may only be sent to local models."},
	"busy":             {"Die KI arbeitet noch an diesem Batch.", "The AI is still working on this batch."},
	"notOpen":          {"Die Datei ist schon abgeschlossen.", "The file is already finished."},
	"noRulebook":       {"Für diesen Ordner ist kein bearbeitbares Regelwerk eingetragen.", "No editable rulebook is set for this folder."},
	"statusNoteTitle":  {"Hansei-Status", "Hansei status"},
	"noNewVersion":     {"Keine neue Version.", "No new version."},
	"unchangedFile":    {"%s unverändert", "%s unchanged"},
	"newVersion":       {"v%d · %s: %d Änderungen", "v%d · %s: %d changes"},
	"rule.secret":      {"Klartext-Secrets", "Plain-text secrets"},
	"rule.codename":    {"Alte Codenamen", "Old code names"},
	"rule.frontmatter": {"Pflicht-Frontmatter fehlt", "Required frontmatter missing"},
	"rule.review":      {"Prüfdatum zu alt", "Review date too old"},
	"rule.dead-link":   {"Tote Wikilinks", "Dead wikilinks"},
	"rule.compose":     {"Compose-Kopie im Text", "Compose copy in the text"},
}

// T formats a catalog text.
func T(lang, key string, args ...any) string {
	pair, ok := catalog[key]
	if !ok {
		return key
	}
	text := pair[1]
	if lang == "de" {
		text = pair[0]
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}
