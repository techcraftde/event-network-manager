# Event Network Manager

Lokale, offlinefähige macOS-App zur Verwaltung von Event-Netzwerken mit Cisco
SG350-28 und SG350-28P. Das React-Frontend läuft in einem Tauri-Wrapper; ein
lokaler Go-Agent liest und konfiguriert die Switches per HTTPS und SSH und hält
Snapshots in SQLite.

## Funktionen

- Rollenbasierte Portkonfiguration für Dante/Audio, Control, Lighting, Video,
  Internet, Trunk und Switch-Management
- Verständliche, frei wählbare Port- und Switch-Namen; VLAN, PVID, QoS, IGMP,
  EEE und PoE werden aus zentralen Rollenprofilen abgeleitet
- Multi-Switch-Topologie mit LLDP-/CDP- und MAC-/ARP-Endgeräteerkennung
- 28-Port-Ansicht mit Link, Speed, RX/TX, Fehlern und PoE
- Switchübergreifende Auslastungsübersicht mit Kapazitätsbalken, Sitzungsspitzen,
  Sortierung und Filtern nach Switch, Rolle und Linkstatus
- Event-Check und Alarmsystem für Erreichbarkeit, Auslastung, Dante-Link-Speed,
  Paketfehler, Temperatur und PoE-Budget
- Dante-Zustandsprüfung für IGMP Snooping, DSCP QoS und EEE
- verständliche Bestätigung; Cisco-Befehle bleiben als optionale Details verfügbar
- vollständiger Running-Config-Snapshot vor jeder Änderung
- persistenter inverser Rollback-Plan inklusive lokaler Rollenzuweisungen und
  Speicherung in Startup Config
- SSH-Host-Key-Pinning; Passwörter bleiben im macOS-Schlüsselbund
- lokale HTTP-API als Grenze für einen späteren Proxmox-Remote-Agent

## macOS-Build

Freigegebene lokale Artefakte werden in `outputs/` erzeugt. Sie sind absichtlich
nicht im Repository eingecheckt. Der aktuelle Build ist für Apple Silicon
ad-hoc signiert; eine Apple-Notarisierung ist noch nicht enthalten.

Beim gebündelten Testprofil wird das Ziel `192.168.250.55` mit Benutzer `admin`
verwendet. Das Passwort wird unter dem Dienst `app.eventnetwork.manager` und dem
Switch als Account im macOS-Schlüsselbund gesucht. Es wird weder in Dateien noch
in Git oder SQLite gespeichert.

## Entwicklung

Voraussetzungen: Go, Node.js/npm, Rust und Tauri CLI.

```bash
make test
make dev-backend
make dev-frontend
```

Echter Einzel-Switch-Betrieb:

```bash
export ENM_SWITCH_ADDRESS=192.168.250.55
export ENM_SWITCH_USERNAME=admin
read -s ENM_SWITCH_PASSWORD && export ENM_SWITCH_PASSWORD
make dev-real
```

Mehrere Switches werden über `ENM_SWITCHES_JSON` angebunden. Credentials sollten
in Produktionsumgebungen durch einen Credential-Provider injiziert werden.

## Sicherheitsmodell

- API-Bindung ausschließlich an `127.0.0.1`
- feste CORS-Origin-Liste für Tauri und lokale Entwicklung
- explizite Prüfung und Speicherung des SSH-Hostschlüssels
- enge Allowlist zulässiger SG350-Konfigurationsbefehle
- Snapshot und Rollback-Plan vor dem ersten Schreibbefehl
- serverseitiger Neuaufbau und Abgleich jedes Rollenplans vor dem Anwenden
- automatische Rücknahme teilweise angewendeter Befehle bei CLI- oder
  Verifikationsfehlern
- keine Ausgabe von Running Configs über die HTTP-API

## Projektstruktur

- `frontend/` – React, TypeScript und Vite
- `backend/` – Go-API, Cisco HTTPS/SSH und SQLite
- `src-tauri/` – macOS-Wrapper und Sidecar-Lifecycle
- `docs/` – Architektur und Roadmap

Details zu Änderungen stehen in [CHANGELOG.md](CHANGELOG.md).
