# Changelog

Alle wesentlichen Änderungen an Event Network Manager werden hier dokumentiert.

## 0.3.0 – 2026-08-22

### Hinzugefügt

- Sieben zentrale Rollenprofile: Dante/Audio, Control, Lighting, Video,
  Internet, Trunk und Switch-Management
- Rollenbasierte Ein-Klick-Konfiguration mit verständlichen Portnamen
- Frei wählbare Switch-Anzeigenamen für die Topologie
- Persistenz von Rollen, Portnamen und Switch-Namen in SQLite
- Automatische VLAN-Erzeugung aus Rollenprofilen
- Endgeräteansicht aus LLDP/CDP sowie MAC-/ARP-Daten mit Rollenvorschlägen
- Switchübergreifende Auslastungsübersicht mit Filtern, Kapazitätsbalken und
  Sitzungsspitzen
- Event-Check und Alarme für Auslastung, Portfehler, Dante-Link-Speed,
  Temperatur, Erreichbarkeit und PoE-Budget

### Geändert

- Normalbetrieb zeigt keine VLAN-, Tagged-, PVID- oder Cisco-Details mehr
- Dante-Zustand ist vollständig deutsch und bezieht seine Ports automatisch
  aus der Dante/Audio-Rolle
- SG350-Portsyntax auf die am SG350-28P bestätigte Form `gi1` bis `gi28`
- SSH-Ausführung wartet auf den echten CLI-Prompt statt mit festen kurzen
  Pausen zu arbeiten
- Running-Config-Erfassung wartet auf die vollständige Ausgabe
- Rollenpläne werden direkt vor dem Anwenden serverseitig neu validiert
- CLI- oder Verifikationsfehler lösen einen automatischen Rollback aus
- Rollback stellt zusätzlich lokale Portnamen und Rollenzuweisungen wieder her

### Geprüft

- Portname und Control-Rolle auf Port 24 des SG350-28P real angewendet
- Automatische VLAN-Erzeugung, vollständiger Snapshot und Startup-Config geprüft
- Vollständiger Rollback auf Switch und in SQLite geprüft
- Rollen-, Alarm-, Speicher-, API- und SG350-Parser-Tests bestanden
- Ports & Rollen, Alarme und Auslastungsübersicht im gerenderten Browser geprüft

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
