#!/usr/bin/env bash
# Corre en el VPS. No compila: build.ps1 deja el binario en $APP_DIR/incoming/.
# Uso (normalmente lo llama build.ps1 por ssh):
#   deploy.sh                  → instala incoming/: backup, migrate up, restart
#   deploy.sh --rollback       → vuelve al binario anterior (pocketbase.prev)
#   deploy.sh --rollback-data  → además restaura pb_data del último backup
set -euo pipefail

APP_DIR=${APP_DIR:-/opt/clinica-backend}
SERVICE=clinica-backend
KEEP_BACKUPS=10

# Todo dentro de main(): este archivo se reemplaza mientras corre.
main() {
  cd "$APP_DIR"
  [[ -f .env ]] || { echo "Falta $APP_DIR/.env (copia .env.example y llénalo)"; exit 1; }
  HTTP_ADDR=$(grep -E '^HTTP_ADDR=' .env | tail -1 | cut -d= -f2- || true)
  HTTP_ADDR=${HTTP_ADDR:-127.0.0.1:8090}

  case ${1:-} in
    --rollback)      rollback no ;;
    --rollback-data) rollback yes ;;
    '')              deploy ;;
    *)               echo "Opción desconocida: $1"; exit 1 ;;
  esac
}

deploy() {
  [[ -f incoming/pocketbase ]] || { echo "No hay binario en incoming/"; exit 1; }
  echo "==> Release: $(cat incoming/build-info.txt 2>/dev/null || echo '?')"

  install_service
  mv -f incoming/deploy.sh deploy.sh

  # Con el servicio parado la copia de SQLite (WAL) es consistente.
  sudo systemctl stop "$SERVICE" || true
  local backup=""
  if [[ -d pb_data ]]; then
    backup="backups/pb_data-$(date +%Y%m%d-%H%M%S)"
    echo "==> Backup de pb_data en $backup"
    mkdir -p backups
    cp -a pb_data "$backup"
    echo "$backup" > .last-backup
    ls -1dt backups/pb_data-* | tail -n +$((KEEP_BACKUPS + 1)) | xargs -r rm -rf
  fi

  [[ -f pocketbase ]] && cp -a pocketbase pocketbase.prev
  install -m 755 incoming/pocketbase pocketbase
  mv -f incoming/build-info.txt build-info.txt 2>/dev/null || true
  rm -rf incoming

  echo "==> migrate up"
  if ! ./pocketbase migrate up --dir "$APP_DIR/pb_data"; then
    echo "!! Falló la migración: restaurando binario y datos"
    restore "$backup"
    exit 1
  fi

  start_and_check || { echo "!! El servicio no responde: restaurando binario y datos"; restore "$backup"; exit 1; }
  echo "==> OK: $(cat build-info.txt 2>/dev/null || echo '?')"
}

rollback() {
  local with_data=$1
  [[ -f pocketbase.prev ]] || { echo "No hay pocketbase.prev"; exit 1; }
  local backup=""
  if [[ $with_data == yes ]]; then
    backup=$(cat .last-backup 2>/dev/null || true)
    [[ -n $backup && -d $backup ]] || { echo "No hay backup registrado en .last-backup"; exit 1; }
    echo "==> Rollback de binario y datos ($backup)"
  else
    echo "==> Rollback de binario (pb_data no se toca)"
  fi
  restore "$backup"
}

# Vuelve a pocketbase.prev y, si se pasa un backup, a esos datos; luego arranca.
restore() {
  local backup=${1:-}
  sudo systemctl stop "$SERVICE" || true
  if [[ -f pocketbase.prev ]]; then
    mv -f pocketbase pocketbase.failed 2>/dev/null || true
    cp -a pocketbase.prev pocketbase
  fi
  if [[ -n $backup && -d $backup ]]; then
    rm -rf pb_data.failed
    [[ -d pb_data ]] && mv pb_data pb_data.failed
    cp -a "$backup" pb_data
  fi
  start_and_check || { echo "!! Tampoco arranca la versión anterior: revisa journalctl -u $SERVICE"; exit 1; }
  echo "==> Restaurado"
}

start_and_check() {
  sudo systemctl start "$SERVICE"
  echo "==> Healthcheck http://$HTTP_ADDR/api/health"
  for _ in $(seq 1 15); do
    curl -fsS "http://$HTTP_ADDR/api/health" >/dev/null 2>&1 && return 0
    sleep 1
  done
  sudo journalctl -u "$SERVICE" -n 30 --no-pager || true
  return 1
}

# Instala o actualiza la unidad systemd solo si cambió.
install_service() {
  local unit=/etc/systemd/system/$SERVICE.service tmp
  tmp=$(mktemp)
  sed -e "s|__APP_DIR__|$APP_DIR|g" -e "s|__USER__|$(id -un)|g" incoming/$SERVICE.service > "$tmp"
  if ! sudo cmp -s "$tmp" "$unit"; then
    echo "==> Instalando $unit"
    sudo install -m 644 "$tmp" "$unit"
    sudo systemctl daemon-reload
    sudo systemctl enable "$SERVICE"
  fi
  rm -f "$tmp" incoming/$SERVICE.service
}

main "$@"
