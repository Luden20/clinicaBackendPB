# Comandos VPS

VPS: `ubuntu@148.113.149.240`

## Túnel SSH a los paneles de PocketBase

Los paneles (`/_/`) no están publicados por Caddy; solo se entra por túnel.
Deja la terminal del túnel abierta mientras usas el panel.

| App | Puerto en el VPS | Comando (desde tu PC) | Panel |
|---|---|---|---|
| Clínica | `127.0.0.1:8090` | `ssh -L 8090:127.0.0.1:8090 ubuntu@148.113.149.240` | http://localhost:8090/_/ |
| Landing (luden20) | `127.0.0.1:8091` | `ssh -L 8091:127.0.0.1:8091 ubuntu@148.113.149.240` | http://localhost:8091/_/ |

Los dos a la vez:

```powershell
ssh -L 8090:127.0.0.1:8090 -L 8091:127.0.0.1:8091 ubuntu@148.113.149.240
```

Si sale `channel N: open failed: connect failed: Connection refused`, el servicio no está corriendo:

```bash
systemctl status clinica-backend   # o landing-pb
sudo journalctl -u clinica-backend -n 50 --no-pager
```

## Dónde vive cada cosa

| App | Carpeta | Servicio | Dominio público |
|---|---|---|---|
| Clínica (API) | `/opt/clinica-backend` | `clinica-backend` | https://clinica.api.luden20.online |
| Landing (API) | `/opt/landing-pb` | `landing-pb` | https://api.luden20.online |
| Landing (front) | `/opt/landing-front` | — (Caddy sirve `build/`) | https://luden20.online |

Caddy: `/etc/caddy/Caddyfile` → `sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl reload caddy`

## Deploy

| App | Tu PC | VPS |
|---|---|---|
| Clínica | `.\build.ps1` | `cd /opt/clinica-backend && ./deploy.sh` |
| Landing (front) | `.\build.ps1` en `petesSA` | `cd /opt/landing-front && ./deploy.sh` |
| Landing (API) | — | `cd /opt/landing-pb && ./pocketbase update && sudo systemctl restart landing-pb` |
