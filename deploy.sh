#!/usr/bin/env bash
# Corre en el VPS, que es un clon de este repo en la rama release. No compila:
# trae dist/pocketbase con git. Uso:
#   ./deploy.sh                  → muestra qué cambia, pide confirmación y aplica
#   ./deploy.sh -y               → igual, sin preguntar
#   ./deploy.sh --rollback       → vuelve al binario anterior (dist.prev/)
#   ./deploy.sh --rollback-data  → además restaura pb_data del último backup
set -euo pipefail

SERVICE=clinica-backend
KEEP_BACKUPS=10

# Todo dentro de main(): git reset reemplaza este archivo mientras corre.
main() {
  cd "$(dirname "$0")"
  APP_DIR=$(pwd)
  [[ -f .env ]] || { echo "Falta $APP_DIR/.env (copia deploy/.env.example y llénalo)"; exit 1; }
  HTTP_ADDR=$(grep -E '^HTTP_ADDR=' .env | tail -1 | cut -d= -f2- || true)
  HTTP_ADDR=${HTTP_ADDR:-127.0.0.1:8090}

  case ${1:-} in
    ''|-y)           deploy "${1:-}" ;;
    --rollback)      rollback no ;;
    --rollback-data) rollback yes ;;
    *)               echo "Opción desconocida: $1"; exit 1 ;;
  esac
}

deploy() {
  local yes=$1
  echo "==> Trayendo la última release"
  git fetch -q origin release
  echo "   actual: $(cat dist/build-info.txt 2>/dev/null || echo 'nada')"
  echo "   nueva:  $(git show origin/release:dist/build-info.txt)"
  if cmp -s <(git show origin/release:dist/build-info.txt) dist/build-info.txt 2>/dev/null; then
    echo "Ya está desplegada."; exit 0
  fi
  local migrations
  migrations=$(git diff --name-only --diff-filter=AM HEAD origin/release -- migrations/ || true)
  echo "   migraciones nuevas o cambiadas:"
  if [[ -n $migrations ]]; then sed 's/^/      /' <<< "$migrations"; else echo "      ninguna"; fi
  if [[ $yes != -y ]]; then
    read -rp "¿Aplicar? Se para el servicio unos segundos. [s/N] " ans
    [[ $ans == [sS] ]] || { echo "Cancelado."; exit 0; }
  fi

  if [[ -d dist ]]; then rm -rf dist.prev && cp -a dist dist.prev; fi
  git reset -q --hard origin/release
  install_service

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

  chmod +x dist/pocketbase
  echo "==> migrate up"
  if ! dist/pocketbase migrate up --dir "$APP_DIR/pb_data"; then
    echo "!! Falló la migración: restaurando binario y datos"
    restore "$backup"
    exit 1
  fi

  start_and_check || { echo "!! El servicio no responde: restaurando binario y datos"; restore "$backup"; exit 1; }
  echo "==> OK: $(cat dist/build-info.txt)"
}

rollback() {
  local with_data=$1 backup=""
  [[ -d dist.prev ]] || { echo "No hay versión anterior en dist.prev/"; exit 1; }
  if [[ $with_data == yes ]]; then
    backup=$(cat .last-backup 2>/dev/null || true)
    [[ -n $backup && -d $backup ]] || { echo "No hay backup registrado en .last-backup"; exit 1; }
    echo "==> Rollback a $(cat dist.prev/build-info.txt) con datos de $backup"
    echo "   Se pierde lo escrito en la base desde ese backup."
  else
    echo "==> Rollback a $(cat dist.prev/build-info.txt) (pb_data no se toca)"
  fi
  read -rp "¿Seguro? [s/N] " ans
  [[ $ans == [sS] ]] || { echo "Cancelado."; exit 0; }
  restore "$backup"
}

# Vuelve a dist.prev/ y, si se pasa un backup, a esos datos; luego arranca.
restore() {
  local backup=${1:-}
  sudo systemctl stop "$SERVICE" || true
  if [[ -d dist.prev ]]; then
    rm -rf dist.failed && mv dist dist.failed && mv dist.prev dist && mv dist.failed dist.prev
  fi
  if [[ -n $backup && -d $backup ]]; then
    rm -rf pb_data.failed
    [[ -d pb_data ]] && mv pb_data pb_data.failed
    cp -a "$backup" pb_data
  fi
  start_and_check || { echo "!! Tampoco arranca la versión anterior: revisa journalctl -u $SERVICE"; exit 1; }
  echo "==> Restaurado a $(cat dist/build-info.txt)"
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
  sed -e "s|__APP_DIR__|$APP_DIR|g" -e "s|__USER__|$(id -un)|g" deploy/$SERVICE.service > "$tmp"
  if ! sudo cmp -s "$tmp" "$unit"; then
    echo "==> Instalando $unit"
    sudo install -m 644 "$tmp" "$unit"
    sudo systemctl daemon-reload
    sudo systemctl enable "$SERVICE"
  fi
  rm -f "$tmp"
}

main "$@"
