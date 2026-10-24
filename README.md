# Personalizacion
Todo lo relacionado debe incluirse en internal

# Despliegue en el VPS
Mismo esquema que formi: se compila en Windows y el VPS sigue la rama `release`.

**Local** (solo compila y publica, no toca el server):
```powershell
.\build.ps1           # compila dist/pocketbase, push de main y de release
.\build.ps1 -NoPush   # solo compila
```

**VPS** (cuando tú decidas):
```bash
./deploy.sh                  # muestra versión y migraciones nuevas, pide confirmación y aplica
./deploy.sh --rollback       # vuelve al binario anterior (dist.prev/)
./deploy.sh --rollback-data  # además restaura pb_data del último backup
```
`deploy.sh` para el servicio, hace backup de `pb_data` en `backups/`, `migrate up`, arranca y
comprueba `/api/health`. Si falla, restaura binario y datos solo. Las migraciones (`migrations/*.go`)
van compiladas en el binario.

## Primera vez en el VPS
```bash
sudo git clone -b release https://github.com/Luden20/clinicaBackendPB.git /opt/clinica-backend
sudo chown -R $USER /opt/clinica-backend && cd /opt/clinica-backend
cp deploy/.env.example .env && nano .env
# si vienes de Railway: copia el volumen /data a /opt/clinica-backend/pb_data
./deploy.sh
```
Corre como servicio systemd `clinica-backend` en `HTTP_ADDR` (por defecto `127.0.0.1:8090`).
Logs: `journalctl -u clinica-backend -f`
