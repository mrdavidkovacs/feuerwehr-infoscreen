# Feuerwehr Infoscreen

Produktionsnahe, schlanke Go-Applikation für einen lokalen Feuerwehr-Infoscreen mit zwei Kioskseiten.

- **Display 1**: zeigt `MAIN_URL` im iframe und blendet bei Ausfall eine lokale Fallback-Seite ein.
- **Display 2**: zeigt eine Vollbild-Slideshow aus einem lokalen Bilderverzeichnis.
- **Keine Datenbank, kein Auth, nur Go Standard Library.**
- Robuster 24/7-Betrieb: kleine Binary, einfache Health-/Status-Endpunkte, stdout-Logging, moderate Cache-Header.

## Projektstruktur

```text
.
├── main.go
├── main_test.go
├── public/
│   ├── display1.html
│   ├── display2.html
│   ├── css/
│   │   └── style.css
│   └── js/
│       ├── display1.js
│       └── display2.js
├── images/
│   └── .gitkeep
├── Dockerfile
├── docker-compose.yml
├── .dockerignore
├── go.mod
└── README.md
```

## Konfiguration

| Variable | Pflicht | Default | Beschreibung |
|---|---:|---|---|
| `MAIN_URL` | ja | - | URL des eigentlichen Informationsservers für Display 1 |
| `IMAGE_DIR` | nein | `/app/images` | Verzeichnis mit Bildern für Display 2 |
| `PORT` | nein | `8080` | HTTP-Port im Container/Prozess |
| `STATUS_TIMEOUT_SECONDS` | nein | `5` | Timeout für serverseitige Checks gegen `MAIN_URL` |
| `SLIDESHOW_INTERVAL_SECONDS` | nein | `15` | Bildwechselintervall für Display 2 |
| `IMAGE_REFRESH_SECONDS` | nein | `60` | Intervall zum Neuladen der Bilderliste |

Unterstützte Bildformate: `jpg`, `jpeg`, `png`, `webp`, `gif`. Die Sortierung erfolgt alphabetisch, case-insensitive.

## HTTP-Endpunkte

| Pfad | Beschreibung |
|---|---|
| `/display1` | Kioskseite für Display 1 mit iframe auf `MAIN_URL` |
| `/display2` | Kioskseite für Display 2 mit Vollbild-Slideshow |
| `/api/status` | Prüft serverseitig die Erreichbarkeit von `MAIN_URL` |
| `/api/images` | Listet Bilder aus `IMAGE_DIR` |
| `/images/*` | Liefert Bilder aus `IMAGE_DIR` aus, mit Path-Traversal-Schutz |

### `/api/status`

Antwort:

```json
{
  "online": true,
  "lastSuccess": "2026-06-04T12:00:00Z",
  "checkedAt": "2026-06-04T12:00:00Z"
}
```

Der Check nutzt zuerst `HEAD`. Wenn `HEAD` nicht erfolgreich ist, wird einmal mit `GET` fallbacked. `lastSuccess` wird nur im Speicher gehalten und geht bei Neustart verloren — absichtlich, weil keine Datenbank verwendet wird.

## Lokal entwickeln

Voraussetzung: Go 1.25+.

```bash
go test ./...
MAIN_URL="https://example.org" IMAGE_DIR="$(pwd)/images" go run .
```

Dann öffnen:

- <http://localhost:8080/display1>
- <http://localhost:8080/display2>
- <http://localhost:8080/api/status>
- <http://localhost:8080/api/images>

## Docker Build

```bash
docker build -t feuerwehr-infoscreen:local .
```

Start:

```bash
docker run --rm \
  -p 8080:8080 \
  -e MAIN_URL="https://infoserver.example.local" \
  -v "$(pwd)/images:/app/images:ro" \
  feuerwehr-infoscreen:local
```

## Docker Compose

`docker-compose.yml` anpassen, vor allem `MAIN_URL`:

```yaml
environment:
  MAIN_URL: "https://infoserver.example.local"
```

Start:

```bash
docker compose up -d --build
```

Logs:

```bash
docker compose logs -f
```

Update nach Git-Pull:

```bash
docker compose up -d --build
```

## Beispiel: SMB-Mount am Linux-Host

Beispiel: Bilder liegen auf einem SMB-Share und werden am Host nach `/mnt/feuerwehr-images` gemountet.

1. Paket installieren:

```bash
sudo apt-get update
sudo apt-get install -y cifs-utils
```

2. Credentials-Datei anlegen:

```bash
sudo install -m 600 /dev/null /etc/smbcredentials/feuerwehr-images
sudo nano /etc/smbcredentials/feuerwehr-images
```

Inhalt:

```text
username=SMB_BENUTZER
password=SMB_PASSWORT
domain=WORKGROUP
```

3. Mountpoint erstellen:

```bash
sudo mkdir -p /mnt/feuerwehr-images
```

4. `/etc/fstab` Beispiel:

```fstab
//fileserver.local/feuerwehr-images /mnt/feuerwehr-images cifs credentials=/etc/smbcredentials/feuerwehr-images,iocharset=utf8,vers=3.0,ro,nofail,x-systemd.automount,x-systemd.idle-timeout=60,uid=1000,gid=1000,file_mode=0444,dir_mode=0555 0 0
```

5. Test:

```bash
sudo systemctl daemon-reload
sudo mount -a
ls -la /mnt/feuerwehr-images
```

6. Compose-Volume anpassen:

```yaml
volumes:
  - /mnt/feuerwehr-images:/app/images:ro
```

## Beispiel: Chromium-Kiosk mit zwei Displays

Die genaue Display-Geometrie hängt von GPU, Window Manager und Bildschirm-Anordnung ab. Beispiel für X11 mit zwei Full-HD-Displays nebeneinander:

Display 1 links (`/display1`):

```bash
chromium-browser \
  --kiosk \
  --new-window \
  --window-position=0,0 \
  --window-size=1920,1080 \
  --noerrdialogs \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --check-for-update-interval=31536000 \
  http://localhost:8080/display1
```

Display 2 rechts (`/display2`):

```bash
chromium-browser \
  --kiosk \
  --new-window \
  --window-position=1920,0 \
  --window-size=1920,1080 \
  --noerrdialogs \
  --disable-infobars \
  --disable-session-crashed-bubble \
  --check-for-update-interval=31536000 \
  http://localhost:8080/display2
```

Für Autostart kann man diese Befehle z. B. in eine systemd-User-Unit oder in die Autostart-Konfiguration des Window Managers legen. Für 24/7-Kiosks zusätzlich sinnvoll:

```bash
xset s off
xset -dpms
xset s noblank
```

## Betriebsnotizen

- HTML/CSS/JS und API-Antworten werden mit `no-cache` ausgeliefert.
- Bilder werden mit `Cache-Control: public, max-age=300` ausgeliefert.
- `/display1` prüft alle 30 Sekunden `/api/status`.
- `/display1` lädt sich alle 60 Minuten neu.
- `/display2` wechselt Bilder gemäß `SLIDESHOW_INTERVAL_SECONDS`.
- `/display2` lädt die Bilderliste gemäß `IMAGE_REFRESH_SECONDS` neu.
- Das Image läuft als statische Binary in `scratch`, ohne Shell und ohne Paketmanager. Sehr langweilig. Also gut.

## GitHub Push

Dieses Verzeichnis ist bereits als Git-Repo geeignet. Beispiel:

```bash
git remote add origin git@github.com:DEIN_ORG/feuerwehr-infoscreen.git
git push -u origin main
```
