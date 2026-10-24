# Personalizacion
Todo lo relacionado debe incluirse en internal

# Despliegue en el VPS
Se compila en Windows y se despliega nativo (systemd, sin Docker):

```powershell
.\build.ps1                 # compila, sube y despliega
.\build.ps1 -NoDeploy       # solo compila a dist/
.\build.ps1 -Rollback       # vuelve al binario anterior
.\build.ps1 -RollbackData   # binario + pb_data del último backup
```

Las migraciones (`migrations/*.go`) van compiladas en el binario. `deploy/deploy.sh` (corre en el VPS)
hace backup de `pb_data`, `migrate up`, reinicia el servicio, comprueba `/api/health` y, si algo
falla, restaura solo.

## Primera vez
1. Local: copia `.env.build.example` a `.env.build` y llena `VPS_HOST`, `VPS_USER` (acceso por llave SSH).
2. VPS (el usuario necesita `sudo` sin contraseña para `systemctl`/`install`, y `curl`):
   ```bash
   sudo mkdir -p /opt/clinica-backend && sudo chown $USER /opt/clinica-backend
   nano /opt/clinica-backend/.env      # ver deploy/.env.example
   ```
3. Si vienes de Railway, copia el volumen `/data` a `/opt/clinica-backend/pb_data`.
4. `.\build.ps1`

Logs: `journalctl -u clinica-backend -f`
