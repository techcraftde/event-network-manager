# Changelog

Alle wesentlichen Änderungen an Event Network Manager werden hier dokumentiert.

## 0.2.0 – 2026-08-22

### Hinzugefügt

- Interaktiver VLAN-Porteditor direkt vom ausgewählten Topologie-Switch aus
- Mehrfachauswahl für Ports sowie Access- und additive Trunk-Zuweisung
- Planvorschau, Sicherheitswarnungen, Snapshot und Anwenden in der VLAN-Ansicht
- Echte Dante-Zustandsprüfung per Cisco-CLI für IGMP, DSCP und EEE
- Deutsche Beschriftungen und Rückmeldungen im Dante-Bereich
- Tests für VLAN-Planung, Dante-Auswertung, API und Rollback

### Geändert

- Frische SSH-Verbindung pro Konfigurationsphase für ältere SG350-SSH-Server
- Wiederanmeldung bei abgelaufenen oder leeren Cisco-Websessions
- Fehler werden nicht mehr durch Frontend-Mockdaten verdeckt
- Rückweg erkennt normalisierte SG350-QoS-/EEE-Konfigurationen

### Geprüft

- VLAN-Änderung auf einem link-down Testport mit anschließendem Rollback
- Dante-Plan und Echtzustand auf einem SG350-28P
- alle sechs App-Bereiche und Netzwerkscan in der gerenderten Oberfläche

## 0.1.0 – 2026-08-22

- Erstes lauffähiges React-/Go-/Tauri-/SQLite-Skeleton
- Reale SG350-HTTPS-Telemetrie und SSH-Konfigurationspipeline
- Multi-Switch-Grenzen, Snapshots, Host-Key-Pinning und macOS-Bundles
