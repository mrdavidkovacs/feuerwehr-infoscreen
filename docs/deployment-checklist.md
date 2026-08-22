# Deployment checklist: Feuerwehr-Infoscreen

Reuse the existing Windows mini PC: install Ubuntu Desktop on it at home, then return the prepared device to the Feuerwehr. This deliberately replaces Windows; it is not a rollback plan.

## Prepare the existing mini PC at home

1. At the Feuerwehr, photograph the cabling and display order before taking the existing mini PC home.
2. Install current Ubuntu Desktop LTS from the official USB installer, replacing Windows. A one-off machine does not need an autoinstall image.
3. Create the local admin account and apply updates:
   ```bash
   sudo apt update && sudo apt full-upgrade -y
   sudo apt install -y docker.io docker-compose-v2 chromium-browser cifs-utils tailscale unattended-upgrades
   sudo usermod -aG docker "$USER"
   ```
   Log out and back in once so the Docker group takes effect.
4. Enable automatic security updates through Ubuntu's `Software & Updates` → `Updates` → `Automatically check for updates` and `Install security updates without confirmation`. Verify later with:
   ```bash
   systemctl status apt-daily-upgrade.timer
   ```
5. Bring Tailscale up over the available home network and verify remote access before transport:
   ```bash
   sudo tailscale up
   tailscale status
   ```
6. Store the Feuerwehr Wi-Fi profile if its SSID and credentials are known; connectivity can only be verified on site.
7. Clone this repository and start the screen locally after setting the real `MAIN_URL` in `docker-compose.yml`:
   ```bash
   git clone https://github.com/mrdavidkovacs/feuerwehr-infoscreen.git
   cd feuerwehr-infoscreen
   docker compose up -d --build
   curl -fsS http://127.0.0.1:8080/api/status
   ```

## Slideshow images: SMB bind mount

`/display2` only serves files from `IMAGE_DIR`. In the committed Compose file this is `./images`, so it is currently a normal Docker bind mount. For the real photos, mount the SMB share read-only on Ubuntu and bind that mount into the container.

1. Create a restricted credentials file; do **not** commit it:
   ```bash
   sudo install -d -m 700 /etc/samba/credentials
   sudoedit /etc/samba/credentials/feuerwehr-images
   sudo chmod 600 /etc/samba/credentials/feuerwehr-images
   ```
   Content:
   ```text
   username=SMB_USER
   password=SMB_PASSWORD
   domain=WORKGROUP
   ```
2. Create the mount point:
   ```bash
   sudo mkdir -p /mnt/feuerwehr-images
   ```
3. Add this to `/etc/fstab`, replacing server and share names:
   ```fstab
   //SMB_SERVER/SHARE /mnt/feuerwehr-images cifs credentials=/etc/samba/credentials/feuerwehr-images,vers=3.0,ro,nofail,x-systemd.automount,x-systemd.idle-timeout=60,uid=1000,gid=1000,file_mode=0444,dir_mode=0555 0 0
   ```
4. Verify the mount and image list:
   ```bash
   sudo systemctl daemon-reload
   sudo mount -a
   ls -la /mnt/feuerwehr-images
   ```
5. Change the Compose volume from `./images:/app/images:ro` to:
   ```yaml
   - /mnt/feuerwehr-images:/app/images:ro
   ```
   Then restart and verify `http://127.0.0.1:8080/api/images` lists the photos.

The application reloads the image list every 60 seconds and rotates pictures every 15 seconds by default. Supported formats are JPG/JPEG, PNG, WebP, and GIF.

## Two-display Chromium kiosk

The existing kiosk commands use X11 coordinates. At login choose **Ubuntu on Xorg** (not the default Wayland session), arrange the two displays in Ubuntu Settings, then confirm their geometry with `xrandr --query`.

Create `~/bin/infoscreen-kiosk.sh`:

```sh
#!/bin/sh
sleep 8
xset s off
xset -dpms
xset s noblank

chromium-browser --kiosk --new-window --user-data-dir="$HOME/.cache/infoscreen-chromium-1" \
  --window-position=0,0 --window-size=1920,1080 \
  --no-first-run --noerrdialogs --disable-infobars \
  http://127.0.0.1:8080/display1 &

chromium-browser --kiosk --new-window --user-data-dir="$HOME/.cache/infoscreen-chromium-2" \
  --window-position=1920,0 --window-size=1920,1080 \
  --no-first-run --noerrdialogs --disable-infobars \
  http://127.0.0.1:8080/display2 &
```

Make it executable and add it to GNOME's Startup Applications:

```bash
chmod +x ~/bin/infoscreen-kiosk.sh
mkdir -p ~/.config/autostart
cat > ~/.config/autostart/infoscreen-kiosk.desktop <<EOF
[Desktop Entry]
Type=Application
Name=Feuerwehr Infoscreen
Exec=$HOME/bin/infoscreen-kiosk.sh
X-GNOME-Autostart-enabled=true
EOF
```

Adjust the second window position and both sizes if the displays are not two 1920×1080 panels arranged left-to-right.

## On-site reconnection and acceptance

1. Reconnect the prepared former Windows mini PC using the cabling photo taken before removal.
2. Connect both displays, Ethernet/Wi-Fi, and power; boot the device.
3. Confirm the Feuerwehr Wi-Fi receives an address and has working DNS/internet.
4. Confirm each display opens the intended local endpoint.
5. Confirm display 1's `MAIN_URL` renders and its fallback is understandable if the remote source is down.
6. Confirm display 2 contains current SMB photos and advances after one interval.
7. From another network, confirm the device is reachable through Tailscale.
8. Reboot once. Both Docker service and Chromium kiosk must recover without local intervention.

## Values to decide before deployment

- Actual `MAIN_URL` for display 1.
- Feuerwehr Wi-Fi credentials and whether a captive portal exists.
- SMB server/share, dedicated read-only account, and whether the share is reachable from the Feuerwehr network.
- Physical display order, resolution, and connectors.
- Device hostname and Tailscale ownership/tags.
