# Architektur

```text
React UI ──HTTP/JSON── Local Agent API (Go)
                         │
              ┌──────────┼───────────┐
          Discovery   Telemetry   Configuration
          SNMP/CDP    SNMP/LLDP       SSH
              └──────────┼───────────┘
                       SQLite
```

Der Go-Prozess ist der **Agent**. In der lokalen App läuft er auf Loopback. Ein
späterer Proxmox-Modus implementiert dieselbe versionierte API und ergänzt
Authentifizierung/TLS. Die React-Oberfläche kennt keine Geräteprotokolle.

## Module

- `domain`: protokollunabhängige Switch-, Port-, Link- und Snapshot-Modelle
- `platform`: kleine Interfaces sowie Cisco-HTTPS-/SSH- und Mock-Adapter
- `api`: lokale, später versionierbare HTTP-Grenze
- `storage`: SQLite-Schema; Credentials bleiben im Schlüsselbund
- `multi`: Aggregation mehrerer Geräte hinter denselben Schnittstellen

## Sichere Konfigurationspipeline

1. Gerät und Modell/Firmware verifizieren.
2. Running Config über SSH lesen.
3. Snapshot atomar in SQLite speichern.
4. Befehle aus einem validierten Plan anwenden.
5. Konfiguration und Erreichbarkeit prüfen.
6. Persistenten inversen Plan für einen kontrollierten Rollback bereithalten.

## Dante-Preset

Das Preset erzeugt einen prüfbaren SG350-Plan für QoS (DSCP), IGMP Snooping und
das Deaktivieren von EEE auf ausgewählten Dante-Ports. Immediate Leave und der
IGMP-Querier werden nicht automatisch verändert, weil diese Entscheidungen von
der tatsächlichen Multicast-Topologie abhängen.
