# Changelog

Alle wesentlichen Änderungen an Event Network Manager werden hier dokumentiert.

## 0.5.3 – 2026-08-23

### Behoben

- Portrollen werden aus der tatsächlich vom Switch gelesenen
  VLAN-Mitgliedschaft erkannt, auch wenn das separate SG350-PVID-Feld veraltet
  oder widersprüchlich ist
- Erkennungsreihenfolge: Trunk/Management über Trunk-Modus und VLAN 4000,
  Access-Rollen über untagged VLAN, anschließend PVID und eindeutige
  VLAN-Mitgliedschaft
- Die Rollenauswahl im Porteditor zeigt bei einem einzelnen bereits
  zugeordneten Port direkt dessen aktuelle Rolle; bei mehreren Ports gilt dies,
  wenn alle dieselbe Rolle haben
- Cisco-Session-Header mit angehängten Cookie-Attributen werden normalisiert,
  sodass keine fortlaufenden Cookie-Warnungen mehr entstehen

### Live geprüft

- VLAN 1 → Dante/Audio, VLAN 2 → Control, VLAN 3 → Lighting,
  VLAN 4 → Internet und VLAN 4000 → Trunk/Management auf beiden verbundenen
  SG350-Geräten
- Topologie, physische Portansicht, Inspector und Porteditor zeigen dieselben
  erkannten Rollen

## 0.5.2 – 2026-08-23

### Behoben

- Der Cisco-Weblogin übernimmt die vom SG350 im HTTP-Header gelieferte
  `sessionID` jetzt wie die originale Weboberfläche in die Session-Cookies
- Gültige Cisco-Anmeldestatus für Initial-, Komplexitäts- und
  Ablaufwarnungen werden nicht mehr pauschal als falsches Passwort behandelt
- Ein Switch wird erst als verbunden veröffentlicht, nachdem der erste echte
  Topologie- und Portabruf erfolgreich war; fehlgeschlagene Sitzungen blockieren
  keine erneute Eingabe korrigierter Zugangsdaten mehr

### Live geprüft

- `192.168.250.51` meldet sich erfolgreich an und erscheint als realer
  SG350-28 mit 28 Ports, VLANs, aktiven Links und Live-Traffic
- Der Switch erscheint in der Topologie sowie in der Auswahl unter
  „Ports & Rollen“ mit physischer Portanordnung

## 0.5.1 – 2026-08-23

### Behoben

- „Netzwerk scannen“ lädt nicht mehr nur bereits bekannte Geräte neu, sondern
  durchsucht den real angeschlossenen privaten Event-Netzbereich
- Kein 15-Sekunden-Startversuch mehr auf dem früher fest eingebauten Ziel
  `192.168.250.55`; der Scanner ist auch ohne bekannten Switch sofort verfügbar
- Ein leerer Gerätebestand wird als echte leere Topologie statt als Demo-Netz
  ausgeliefert

### Hinzugefügt

- Automatische Ermittlung des lokalen Bereichs, hier `192.168.250.1–254`
- Optionales Eingabefeld für einen anderen privaten Scanbereich
- Maskierte Benutzer-/Passwortabfrage für jeden gefundenen Switch
- Wiederholbare Anmeldung bei falschem Passwort mit Hinweis auf relevante
  Groß-/Kleinschreibung
- Optionales Speichern eines erfolgreich geprüften Passworts im macOS-
  Schlüsselbund; keine Ablage in Git, SQLite oder Logs
- Laufzeitfähiger Multi-Switch-Manager, der neu angemeldete Geräte ohne
  Backend-Neustart in Topologie, Telemetrie und Konfiguration übernimmt

### Geprüft

- Live-Scan fand die Cisco-Geräte `192.168.250.51` und `192.168.250.56`
- Beide Geräte erscheinen mit eigener Zugangsdatenabfrage
- Passwortfeld ist maskiert und „Verbinden“ ohne Passwort deaktiviert
- Öffentliche Bereiche wie `192.160.250.x` sowie Scans über 1024 Adressen werden
  serverseitig abgelehnt

## 0.5.0 – 2026-08-22

### Hinzugefügt

- Eigener Button „ME-Standard wiederherstellen“ im Event-Grundsetup
- Sanitierter Reset anhand der bereitgestellten SG350-28-Referenz mit dem
  festen Portschema für VLAN 1–4 und Management-Trunks auf Port 25–28
- Exakte DSCP-Zuordnung der Referenz mit Dante-Prioritäten 8/46/56
- Fortschrittsanzeige, automatischer Snapshot sowie Running-/Startup-Prüfung

### Sicherheit

- Reset-Plan wird ausschließlich serverseitig aus der unmittelbar gelesenen
  Running Config erzeugt; Browserbefehle werden ignoriert
- Vorgang wird verweigert, wenn die statische Management-IP nicht sicher
  erkannt werden kann
- Management-IP, Switchname, Benutzer, Passwörter, SNMP-Zugänge, SSH-Schlüssel
  und Zertifikate werden weder verändert noch aus der Referenz übernommen
- IGMP Snooping, alle Referenz-Querier und Multicast-Filterung werden explizit
  ausgeschaltet; zusätzliche VLAN-Definitionen bleiben als Schutz möglicher
  weiterer Management-Schnittstellen erhalten

### Geprüft

- Vollständiger Reset-Plan bleibt unter dem SG350-Limit von 512 Kommandos
- Unit-Tests für Portschema, Management-IP-Schutz, CLI-Allowlist, Verifikation
  und inversen Rollback
- API-Integrationstest stellt sicher, dass mitgesendete Client-Kommandos nicht
  ausgeführt werden

## 0.4.1 – 2026-08-22

### Hinzugefügt

- Sichtbare Switch-Auswahl in „Ports & Rollen“ mit Name und IP-Adresse
- Ein Klick auf einen Switch-Knoten in der Topologie öffnet direkt dessen
  Port- und Rollenkonfiguration
- Deutlicher „Ports konfigurieren“-Hinweis auf jedem Topologie-Switch

### Geprüft

- Topologie-Klick auf den realen SG350 öffnet dessen Portansicht
- Wechsel zwischen drei Switches per Topologie und Dropdown lädt jeweils den
  richtigen Switch und dessen Ports

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
