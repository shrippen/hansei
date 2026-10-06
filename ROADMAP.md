# Hansei – Roadmap

Konzept und Entwürfe: Artifact „Hansei“ (Konzeptstudie, 27.09.2026). Entscheidungen dort; hier der Stand.

## Entscheidungen

| Frage | Entscheidung |
|---|---|
| Form | Eigenständig, kein Obsidian-Plugin, keine Erweiterung von andon (andon bleibt lesend) |
| Technik | Go-Kern ohne cgo (andon kann ihn importieren), KDE-App C++/Qt6/Kirigami als Client über JSON-RPC, TUI in Go |
| Oberfläche | Review-Strecke als Kern, Werkbank als zweite Sicht auf dieselben Batches |
| Feedback | Immer möglich: pro Hunk, Datei, Batch; Versionen pro Datei; wiederholtes Feedback → Regel-Vorschlag |
| Diff | Standard nebeneinander, beide Seiten gekoppelt, Füllzeilen gegenüber Löschungen |
| Stile | System (Standard), Kante Light, Kante – opt-in wie in der Kante-README |
| KI | Offen: Claude (SDK), OpenAI-kompatibel (Ollama, LM Studio); manuell, Automatisierung optional |
| Kontext | Index + gezieltes Lesen über Werkzeuge statt Vault-Dump |
| Reichweite | Freigabe pro Ordner, Sperrliste schlägt alles, „nur lokal“ pro Ordner; Regelwerk pro Ordner |
| Schreiben | In den lokalen Tresor (Fast Note Sync verteilt), eigenes Journal zum Rückgängigmachen |
| Doku-Abgleich | Erkennung von Lücken Doku ↔ Compose liegt in andon (kennt Komodo, Gitea, Docker); Hansei erkennt nicht doppelt, sondern holt die Befunde und macht Batches daraus (01.10.2026) |

## Stand

| Phase | Inhalt | Stand |
|---|---|---|
| 00 Fundament | Go-Modul, Tresor mit Freigabe/Sperrliste, Index (Frontmatter, Überschriften, Wikilinks, Backlinks), Import `review-queue/` | ✓ |
| 01 Review | Diff pro Hunk (zeilen- und wortweise), Annehmen/Ablehnen/Bearbeiten, Schreiben beim letzten Hunk, Journal/Rückgängig, Rebase bei geänderten Notizen, TUI, Dienst, KDE-App | ✓ |
| 02 KI | Anbieter-Schicht, Geheimnis-Filter, Werkzeuge, Regelwerk pro Ordner, Aufträge, Rückfragen, Feedback-Runden, Kostenschätzung | ✓ |
| 03 Prüfregeln | Secrets, Codenamen, Pflicht-Frontmatter, Prüfdatum, tote Links, Compose-Kopien; Befund → Batch; Quote, Serie, Abschluss-Moment | ✓ |
| 04 Automatisierung | `hansei auftrag`, systemd-Timer (`contrib/`), Statusnotiz für andon (`status_note`) | ✓ Hansei-Seite |
| 05 Doku-Abgleich mit andon | Feld `Compose`, Bestand nachziehen, Befund-Quelle `andon`, Stand an andon senden (unten) | andon-Seite fertig (07.10.2026), Hansei-Seite offen |
| 06 Oberfläche | Kante: eigene Chips, Sparkline, Fortschrittsstreifen und Abschnittslabels durch Kante ersetzt, Gespräch als `KanteMessage`; Landing Page `docs/index.html` nach Kante-Vorlage | ✓ |

## 05 Doku-Abgleich mit andon (geplant 01.10.2026, andon-Seite fertig 07.10.2026)

Gegenstück zu Phase 15 in `andon/ROADMAP.md`: andon vergleicht die Compose-Repos
(`docker-compose-{regis,eredin,ploetze}`) mit dem Frontmatter in `ObsidianPrivat` und zeichnet die Doku
in Homelable. Hansei bleibt das einzige Werkzeug, das in den Vault schreibt.

**Stand 07.10.2026:** andon liest Stacks und Frontmatter und bietet beide Schnittstellen an (andon Phase 15,
PRs #63–#72). Die Demowelt hat Compose-Stacks, `Compose`-Felder in sieben `it_docs`-Notizen und Hansei's Stand
unter `code.hansei` (shrippen.github.io #25–#28, hier #8–#10). Im echten Vault hat noch keine Notiz ein
`Compose`-Feld; andon meldet daher alle 118 Stacks (Regis 79, Eredin 28, Plötze 11) als undokumentiert.

Schnittstelle (Einzelheiten in `andon/ROADMAP.md`, Phase 15):
- **Befunde holen:** `GET <andon>/api/docs?token=…` (andon-Lese-Token) →
  `{"complete": bool, "findings": [{"id", "rule", "host", "stack", "note", "path", "link", "note_url",
  "compose", "services": [{"name", "image", "ports"}]}]}`. `complete: false`: andon hat Stacks oder Notizen
  nicht vollständig gelesen, fehlende Befunde nicht als erledigt werten. Der Auszug hat kein Environment und
  keine Labels.
- **IDs:** `docs.missing:<host>/<stack>`, `docs.orphan:<Notizpfad>`, `docs.deprecated_live:<host>/<stack>`.
- **Stand senden:** `POST <Webhook-Adresse der andon-Verbindung „Hansei“>` mit
  `{"state": {"review", "feedback", "done", "conformity" (0..1), "claimed": [IDs]}}`. Ersetzt den letzten
  Stand; höchstens 60 Aufrufe je Minute, Körper bis 128 KiB. `claimed` = IDs, an denen ein offener Batch
  arbeitet: andon lässt deren Hinweise warten. Erledigt ist ein Befund erst, wenn der Vault ihn nicht mehr
  zeigt, nicht durch eine Meldung von Hansei.

- [ ] **Batch `IT/Design.md`:** Feld `Compose` (URL zur `compose.yaml`, als Liste erlaubt) in die
  Dienstnotiz-Konvention; Infrastruktur-Stacks (`komodo_periphery*`, `newt-*`, `glances-*`, `tailscale-*`,
  `caddy-*` …) im `Compose`-Feld der Gerätenotiz; Netz-Notizen (Tailscale, Pangolin …) führen ihre Stacks
  ebenfalls unter `Compose`, Kennzeichnung als Netz festlegen; offenen Punkt „Live-Systeme auslesen“ als
  entschieden eintragen; Freigabeweg von Diff-Review-Tool/`review-queue/` auf Hansei umstellen; Homelable
  als reine Ansicht unter „Quellen der Wahrheit“
- [ ] **Batch Bestand:** `Compose` in den aktiven Dienst-, Geräte- und Netz-Notizen nachtragen. Vorschlag aus
  dem Namensabgleich vom 01.10.2026; abweichende Namen (`kometa` = Plex-Meta-Manager, `seerr` = Jellyseer,
  `digikam_db`, `postgres_davinci`) im Diff prüfen
- [ ] **Config:** `Compose` als Pflichtfeld für `IT/Dienste/{Regis,Eredin,Plötze}` (Triss, Emhyr, Extern haben
  kein Compose-Repo); prüfen, ob die Pflichtfeld-Regel deprecated-Notizen ausnimmt, sonst ergänzen
- [ ] **Befund-Quelle `andon`:** Config `andon: {url}`, Lese-Token und Webhook-Adresse in KWallet
  (`hansei key set andon`); holt die `docs.*`-Befunde über `/api/docs` (fehlende Notiz, verwaiste Notiz,
  deprecated aber im Repo, später Abweichungen) und macht daraus Batches, mit dem Compose-Auszug von andon als
  Kontext. Hansei braucht keinen eigenen Gitea-Zugang. `hansei befunde` und der Timer in `contrib/` beziehen
  die Quelle ein
- [ ] **Stand an andon:** nach jeder Änderung an Batches oder Konformität und beim Start den ganzen Stand an
  die Webhook-Adresse senden (`claimed` aus den offenen Batches mit andon-Befunden); war andon nicht
  erreichbar, beim nächsten Anlass erneut. Ersetzt die Statusnotiz: `status_note` und `writeStatusNote`
  entfernen (andon liest sie nicht mehr)
- [ ] **Neue Dienste:** Batch „Notiz für Stack X anlegen“ nach dem Dienstnotiz-Template, Frontmatter aus der
  Compose-Datei vorbefüllt, Fallen/Restore als Rückfrage an mich statt geraten
- [ ] Demo: Befunde aus einer `demo://`-Quelle mit Studio-Weber-Daten (nur `-tags demo`); passend zur Welt: Stacks unter `code.gitea.stacks`, `Compose` in den `it_docs`-Notizen, Stand unter `code.hansei`

## Offen

- **andon:** Abgleich Doku ↔ Compose umgesetzt (andon Phase 15), Homelable dort noch offen; Gegenstück hier in
  Phase 05. Abgleich mit Snipe-IT später dort.
- **KRunner:** „Hansei: Auftrag …“ direkt aus KRunner.
- **Server/Web:** `hansei serve` (HTTP + WebSocket über dasselbe Protokoll) und eine Web-Oberfläche.
- **Mehrbenutzer:** nicht geplant, solange der Tresor persönlich ist.
