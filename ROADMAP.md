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

## Stand

| Phase | Inhalt | Stand |
|---|---|---|
| 00 Fundament | Go-Modul, Tresor mit Freigabe/Sperrliste, Index (Frontmatter, Überschriften, Wikilinks, Backlinks), Import `review-queue/` | ✓ |
| 01 Review | Diff pro Hunk (zeilen- und wortweise), Annehmen/Ablehnen/Bearbeiten, Schreiben beim letzten Hunk, Journal/Rückgängig, Rebase bei geänderten Notizen, TUI, Dienst, KDE-App | ✓ |
| 02 KI | Anbieter-Schicht, Geheimnis-Filter, Werkzeuge, Regelwerk pro Ordner, Aufträge, Rückfragen, Feedback-Runden, Kostenschätzung | ✓ |
| 03 Prüfregeln | Secrets, Codenamen, Pflicht-Frontmatter, Prüfdatum, tote Links, Compose-Kopien; Befund → Batch; Quote, Serie, Abschluss-Moment | ✓ |
| 04 Automatisierung | `hansei auftrag`, systemd-Timer (`contrib/`), Statusnotiz für andon (`status_note`) | ✓ Hansei-Seite |

## Offen

- **andon:** Quelle „Obsidian/Hansei“ in andon, die die Statusnotiz und das Frontmatter aus dem
  `ObsidianPrivat`-Repo liest (Widget „Batches warten“, Abgleich Doku ↔ Snipe-IT/Compose). Gehört ins andon-Repo.
- **Compose → Doku:** Hansei erzeugt aus den `compose.yaml` der Repos `docker-compose-eredin`, `-regis` und
  `-ploetze` je Dienst eine Notiz im Tresor (Image und Version, Ports, Volumes, Netzwerke, `depends_on`, Host)
  und legt sie als Batch zur Review vor. Nur Hansei schreibt; die Compose-Repos werden nur gelesen. Von Hand
  gepflegte Abschnitte bleiben erhalten (Marker). Ein neuer Image-Tag, etwa durch Renovate, erzeugt einen neuen
  Batch. Gegenstück in andon: Abgleich Doku ↔ Compose, nur lesend.
- **KRunner:** „Hansei: Auftrag …“ direkt aus KRunner.
- **Landing Page:** `docs/index.html` nach Kante-Vorlage, Screenshots über `demo/shots.json`.
- **Server/Web:** `hansei serve` (HTTP + WebSocket über dasselbe Protokoll) und eine Web-Oberfläche.
- **Mehrbenutzer:** nicht geplant, solange der Tresor persönlich ist.
