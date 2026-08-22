# Changelog

Alle wesentlichen Änderungen an Event Network Manager werden hier dokumentiert.

## 0.4.0 – 2026-08-22

### Hinzugefügt

- Gemeinsame Rollenzuweisung für beliebig viele ausgewählte Ports
- Schnellauswahl für alle, freie oder keine Ports sowie optionales Namensschema
- Physische SG350-28(P)-Portanordnung mit zwei RJ45-Reihen und Uplinkblock
- Achtstufige Fortschrittsanzeige mit anschließendem Live-Neuladen der Ports
- Eigener Button zum Speichern und Prüfen der Startup Config
- Sechs feste Betriebsrollen mit den Netz-IDs Audio 1, Control 2, Lighting 3,
  Internet 4, Video 5 und Trunk/Management 4000
- Geführtes Event-Grundsetup für Rollen-Netzwerke, Multicast, Dante-QoS und EEE
- Direkte Rekonstruktion von Switchname, Portnamen und Rollen aus Running Config
- Dauerhafte Switch-Umbenennung über den echten Cisco-Hostname
- Verifikation jeder Änderung zusätzlich gegen die gespeicherte Startup Config
- Gewerkhinweise für Yamaha-Control/Dante, MA-Net2/3, Art-Net und sACN
- Funktionaler IGMP-Schalter für Lighting: MA-Net3/sACN oder grandMA2-Modus
  ohne IGMP Snooping auf VLAN 3
- Physische Portreihen und separater Uplinkblock auch in der Topologieansicht

### Behoben

- „Sichern & anwenden“ bleibt nicht mehr kommentarlos wegen eines unbestätigten
  SSH-Schlüssels deaktiviert, sondern führt durch die einmalige Bestätigung
- Sichtbarer Fortschritt während langsamer Snapshot- und SG350-SSH-Vorgänge
- Alle SSH-Zugriffe eines Switches sind serialisiert, damit Live-Scans und
  Schreibvorgänge den älteren SG350-SSH-Server nicht gegenseitig blockieren
- SG350-Verifikation erkennt aktiviertes Spanning Tree an der entfernten
  `spanning-tree disable`-Zeile
- VLAN 1 wird trotz der von Cisco ausgelassenen Default-Zeile korrekt verifiziert
- Dante-Rollen setzen selbstständig DSCP-Maps, IGMP-Querier und Flow Control off

### Geprüft

- Zwei Ports gemeinsam auf dem SG350-28P konfiguriert, aus Running Config wieder
  erkannt und vollständig zurückgerollt
- Event-Grundsetup real angewendet und mit 4/4 erfolgreichen Prüfungen gelesen
- Test-Switch tatsächlich neu gestartet; Event-Grundsetup blieb vollständig
  erhalten und Uptime wurde nach dem Boot frisch eingelesen
- Switchname `Mushroom-Stage-Switch` direkt und dauerhaft auf dem Gerät gesetzt
- Direkte Control-Zuweisung auf Port 1 gespeichert, zurückgelesen und vollständig
  auf VLAN 50 zurückgerollt
- Dante-Rolle mit 5/5 Live-Prüfwerten erfolgreich gesetzt und zurückgerollt
- Neuer Event-Standard dauerhaft angewendet; 4/4 Grundprüfungen und 100 %
  Alarm-Health direkt vom SG350 gelesen

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
