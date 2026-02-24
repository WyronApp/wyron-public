#!/usr/bin/env bash
set -euo pipefail

# Wyron Panel Installer (Ubuntu/Debian-friendly)
# - No hard distro gating
# - If MongoDB missing: installs MongoDB using official repo, choosing latest available major channel (8.0 -> 7.0 fallback)
# - If Mongo installed: asks for MongoDB URI and DOES NOT modify MongoDB config
# - If Mongo installed by this script: enables auth + localhost bind, creates admin user, and auto-fills mongo uri in config.yml
# - Prompts for key config: domain, tls, allowed-hosts/origins, listen, port, db name
# - Sets up /opt/wyron, copies binary, creates systemd service

WYRON_DIR="/opt/wyron"
WYRON_BIN="${WYRON_DIR}/wyron"
SYMLINK_BIN="/usr/local/bin/wyron"
SERVICE_FILE="/etc/systemd/system/wyron.service"
CONFIG_FILE="${WYRON_DIR}/config.yml"
MONGO_CONF="/etc/mongod.conf"

log()  { echo -e "[+] $*"; }
warn() { echo -e "[!] $*" >&2; }
die()  { echo -e "[x] $*" >&2; exit 1; }

need_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "Please run as root (use sudo)."
  fi
}

have_cmd() { command -v "$1" >/dev/null 2>&1; }

apt_install_deps() {
  log "Installing dependencies..."
  apt-get update -y
  apt-get install -y curl gnupg ca-certificates lsb-release openssl
}

detect_codename() {
  local codename=""
  codename="$(. /etc/os-release && echo "${VERSION_CODENAME:-}")"
  if [[ -z "$codename" ]]; then
    codename="$(lsb_release -cs 2>/dev/null || true)"
  fi
  [[ -n "$codename" ]] || die "Could not detect distro codename."
  echo "$codename"
}

add_mongo_repo_and_install_channel() {
  # Args: channel_major (e.g. 8.0 or 7.0), codename
  local channel="$1"
  local codename="$2"

  log "Trying MongoDB channel: mongodb-org/${channel} (codename: ${codename})"

  # Clean older lists for this channel (best-effort)
  rm -f /etc/apt/sources.list.d/mongodb-org-*.list || true

  local keyring="/usr/share/keyrings/mongodb-server-${channel}.gpg"
  curl -fsSL "https://pgp.mongodb.com/server-${channel}.asc" | gpg --dearmor -o "${keyring}"

  echo "deb [ arch=amd64,arm64 signed-by=${keyring} ] https://repo.mongodb.org/apt/ubuntu ${codename}/mongodb-org/${channel} multiverse" \
    > "/etc/apt/sources.list.d/mongodb-org-${channel}.list"

  apt-get update -y
  apt-get install -y mongodb-org

  systemctl enable mongod || true
  systemctl start mongod || true

  if ! systemctl is-active --quiet mongod; then
    die "mongod is not running. Check: journalctl -u mongod --no-pager"
  fi

  log "MongoDB installed (latest available in ${channel} channel) and mongod started."
}

install_mongodb_latest_available() {
  # Chooses "latest available major channel" by trying 8.0 then 7.0.
  # Note: MongoDB apt repositories are major-channel based; inside a channel you get latest patch/minor via apt.
  local codename
  codename="$(detect_codename)"

  # Try 8.0, then fallback to 7.0
  if add_mongo_repo_and_install_channel "8.0" "$codename"; then
    echo "8.0"
    return 0
  fi

  warn "MongoDB 8.0 install failed. Falling back to 7.0..."
  add_mongo_repo_and_install_channel "7.0" "$codename"
  echo "7.0"
}

set_yaml_key_simple() {
  # crude, safe-enough for mongod.conf style YAML: key: value replacements
  # Args: file, key, value
  local file="$1"
  local key="$2"
  local val="$3"

  if grep -qE "^\s*${key}\s*:" "$file"; then
    sed -i "s|^\(\s*${key}\s*:\s*\).*|\1${val}|" "$file"
  else
    echo "${key}: ${val}" >> "$file"
  fi
}

enable_mongodb_auth_localhost_only() {
  [[ -f "$MONGO_CONF" ]] || die "mongod.conf not found at ${MONGO_CONF}"

  log "Configuring MongoDB: bindIp=127.0.0.1 and authorization=enabled"

  # Ensure bindIp is localhost-only (best-effort)
  if grep -qE '^\s*bindIp\s*:' "$MONGO_CONF"; then
    sed -i 's/^\(\s*bindIp\s*:\s*\).*/\1127.0.0.1/' "$MONGO_CONF"
  else
    if grep -qE '^\s*net\s*:\s*$' "$MONGO_CONF"; then
      sed -i '/^\s*net\s*:\s*$/a\  bindIp: 127.0.0.1' "$MONGO_CONF"
    else
      cat >> "$MONGO_CONF" <<'EOF'

net:
  port: 27017
  bindIp: 127.0.0.1
EOF
    fi
  fi

  # Enable authorization
  if grep -qE '^\s*authorization\s*:\s*enabled\s*$' "$MONGO_CONF"; then
    :
  else
    if grep -qE '^\s*security\s*:\s*$' "$MONGO_CONF"; then
      sed -i '/^\s*security\s*:\s*$/a\  authorization: enabled' "$MONGO_CONF"
    else
      cat >> "$MONGO_CONF" <<'EOF'

security:
  authorization: enabled
EOF
    fi
  fi

  systemctl restart mongod
  if ! systemctl is-active --quiet mongod; then
    die "mongod restart failed. Check: journalctl -u mongod --no-pager"
  fi
}

mongo_create_admin_user() {
  # Args: username password
  local user="$1"
  local pass="$2"

  log "Creating MongoDB admin user (if not exists)..."

  local js
  js=$(cat <<'EOF'
try {
  const u = process.env.MONGO_USER;
  const p = process.env.MONGO_PASS;

  // Switch DB correctly inside --eval
  db = db.getSiblingDB("admin");

  const existing = db.getUser(u);
  if (existing) {
    print("EXISTS");
  } else {
    db.createUser({user: u, pwd: p, roles: [{role:"root", db:"admin"}]});
    print("CREATED");
  }
} catch (e) {
  print("ERROR:" + e);
}
EOF
)
  MONGO_USER="$user" MONGO_PASS="$pass" mongosh --quiet --eval "$js" | tail -n 1
}

setup_wyron_dir_and_binary() {
  local src_bin="$1"

  log "Creating install directory: ${WYRON_DIR}"
  mkdir -p "$WYRON_DIR"

  [[ -f "$src_bin" ]] || die "Binary not found: $src_bin"

  log "Copying binary to ${WYRON_BIN}"
  cp -f "$src_bin" "$WYRON_BIN"
  chmod +x "$WYRON_BIN"

  if [[ -e "$SYMLINK_BIN" ]]; then
    warn "Path exists: ${SYMLINK_BIN} (skipping symlink)"
  else
    ln -s "$WYRON_BIN" "$SYMLINK_BIN"
  fi
}

csv_to_yaml_list() {
  local csv="$1"
  local out=""
  IFS=',' read -ra ITEMS <<< "$csv"
  for it in "${ITEMS[@]}"; do
    it="$(echo "$it" | xargs)"
    [[ -n "$it" ]] && out+="  - \"${it}\"\n"
  done
  printf "%b" "$out"
}

write_config() {
  local mongo_uri="$1"
  local db_name="$2"
  local panel_port="$3"
  local listen_on="$4"
  local main_address="$5"
  local allowed_hosts_csv="$6"
  local allowed_origins_csv="$7"
  local tls_enabled="$8"
  local log_level="$9"

  if [[ -f "$CONFIG_FILE" ]]; then
    warn "config.yml already exists. Not overwriting: ${CONFIG_FILE}"
    return 0
  fi

  log "Writing ${CONFIG_FILE} ..."
  umask 077

  local hosts_yaml origins_yaml
  hosts_yaml="$(csv_to_yaml_list "$allowed_hosts_csv")"
  origins_yaml="$(csv_to_yaml_list "$allowed_origins_csv")"

  cat > "$CONFIG_FILE" <<EOF
log: ${log_level}

mongodb:
  uri: "${mongo_uri}"

database_name: "${db_name}"

port: ${panel_port}
listen-on: "${listen_on}"

allowed-hosts:
$(printf "%b" "${hosts_yaml}")

allowed-origins:
$(printf "%b" "${origins_yaml}")

main-address: "${main_address}"

tls:
  enabled: ${tls_enabled}

# NOTE:
# jwt-secret and internal-secret (if used by Wyron) are typically auto-generated on first run.
# Avoid changing them unless you know exactly what you're doing.
EOF

  chmod 600 "$CONFIG_FILE"
}

ensure_timeout() {
  if ! have_cmd timeout; then
    apt-get install -y coreutils
  fi
}

first_run_wyron_generate_config() {
  log "Starting Wyron service for first-run..."

  # Record current time
  start_time="$(date '+%Y-%m-%d %H:%M:%S')"

  systemctl start wyron >/dev/null 2>&1 || true

  sleep 2

  logs="$(journalctl -u wyron --since "$start_time" --no-pager)"

  fingerprint="$(echo "$logs" | grep -i 'your device fingerprint is' | awk '{print $NF}' | head -n1)"

  license_failed="$(echo "$logs" | grep -i 'license validation failed' || true)"
  license_verified="$(echo "$logs" | grep -i 'License verified successfully' || true)"

  systemctl stop wyron >/dev/null 2>&1 || true
  systemctl reset-failed wyron >/dev/null 2>&1 || true

  echo
  echo "----------------------------------------"
  echo "Device Fingerprint:"
  echo "$fingerprint"
  echo "----------------------------------------"
  echo

  if [[ -n "$license_failed" ]]; then
    echo "License not found or invalid."
    echo "Place license.key inside ${WYRON_DIR} and run installer again."
    exit 1
  fi

  if [[ -z "$license_verified" ]]; then
    echo "License status could not be verified."
    echo "Check manually: journalctl -u wyron"
    exit 1
  fi

  log "License verified successfully. Continuing installation..."
}


patch_wyron_config_after_first_run() {
  local mongo_uri="$1"
  local db_name="$2"
  local panel_port="$3"
  local listen_on="$4"
  local main_address="$5"
  local allowed_hosts_csv="$6"
  local allowed_origins_csv="$7"
  local tls_enabled="$8"
  local log_level="$9"

  [[ -f "$CONFIG_FILE" ]] || die "config.yml not found"

  log "Patching config.yml..."

  # -----------------------
  # Helper: set top-level key
  # -----------------------
  set_top() {
    local key="$1"
    local value="$2"

    if grep -qE "^${key}:" "$CONFIG_FILE"; then
      sed -i "s|^${key}:.*|${key}: ${value}|" "$CONFIG_FILE"
    else
      echo "${key}: ${value}" >> "$CONFIG_FILE"
    fi
  }

  # -----------------------
  # Helper: set nested key (2 level)
  # parent:
  #   child: value
  # -----------------------
  set_nested_2() {
    local parent="$1"
    local child="$2"
    local value="$3"

    awk -v p="$parent" -v c="$child" -v v="$value" '
    BEGIN{inblk=0; done=0}
    $0 ~ "^"p":" {
        print
        inblk=1
        next
    }
    inblk && $0 ~ "^[^ ]" {
        if(!done){
            print "    "c": "v
            done=1
        }
        inblk=0
    }
    inblk && $0 ~ "^[ ]{4}"c":" {
        print "    "c": "v
        done=1
        next
    }
    {print}
    END{
        if(inblk && !done){
            print "    "c": "v
        }
    }
    ' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
  }

  set_nested_3() {
    local parent="$1"
    local mid="$2"
    local child="$3"
    local value="$4"

    awk -v p="$parent" -v m="$mid" -v c="$child" -v v="$value" '
    BEGIN{inparent=0; inmid=0; done=0}

    $0 ~ "^"p":" {
        print
        inparent=1
        next
    }

    inparent && $0 ~ "^[^ ]" {
        inparent=0
    }

    inparent && $0 ~ "^[ ]{4}"m":" {
        print
        inmid=1
        next
    }

    inmid && $0 ~ "^[ ]{8}"c":" {
        print "        "c": "v
        done=1
        next
    }

    inmid && $0 ~ "^[ ]{4}[^ ]" {
        if(!done){
            print "        "c": "v
            done=1
        }
        inmid=0
    }

    {print}

    END{
        if(inmid && !done){
            print "        "c": "v
        }
    }
    ' "$CONFIG_FILE" > "$CONFIG_FILE.tmp" && mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"
  }
  # -----------------------
  # Helper: replace list inside panel
  # -----------------------
  replace_panel_list() {
    local key="$1"
    local csv="$2"

    IFS=',' read -ra ITEMS <<< "$csv"

    awk -v k="$key" '
    BEGIN{inpanel=0; inlist=0}

    /^panel:/ {print; inpanel=1; next}

    inpanel && /^[^ ]/ {inpanel=0}

    inpanel && $0 ~ "^[ ]{4}"k":" {
        print "    "k":"
        inlist=1
        next
    }

    inlist && /^[ ]{8}-/ {next}

    {print}
    ' "$CONFIG_FILE" > "$CONFIG_FILE.tmp"

    mv "$CONFIG_FILE.tmp" "$CONFIG_FILE"

    for it in "${ITEMS[@]}"; do
        it="$(echo "$it" | xargs)"
        [[ -n "$it" ]] || continue
        sed -i "/^[ ]\{4\}${key}:/a\\
          - ${it}
  " "$CONFIG_FILE"
    done
  }
  # -----------------------
  # Apply changes
  # -----------------------

  set_top "log" "$log_level"

  set_nested_2 "mongodb" "uri" "$mongo_uri"
  set_nested_2 "mongodb" "database_name" "$db_name"

  set_nested_2 "panel" "port" "$panel_port"
  set_nested_2 "panel" "listen-on" "$listen_on"
  set_nested_2 "panel" "main-address" "$main_address"

  replace_panel_list "allowed-hosts" "$allowed_hosts_csv"
  replace_panel_list "allowed-origins" "$allowed_origins_csv"

  set_nested_3 "panel" "tls" "enabled" "$tls_enabled"

  chmod 600 "$CONFIG_FILE"

  log "Config patch complete."
}

main() {
  need_root

  echo "Wyron Panel Installer"
  echo "---------------------"

  read -r -p "Path to wyron binary (e.g. /root/wyron or ./wyron): " SRC_BIN
  [[ -n "${SRC_BIN}" ]] || die "Binary path is required."

  apt_install_deps

  # 1) Install/copy Wyron binary ASAP
  setup_wyron_dir_and_binary "$SRC_BIN"

  # 2) First run ASAP (generate config + show fingerprint/license output)
  first_run_wyron_generate_config

  # Require config to exist after first run
  if [[ ! -f "$CONFIG_FILE" ]]; then
    warn "config.yml was not generated after first run."
    warn "Check: ${WYRON_DIR}/first-run.log"
    warn "If Wyron needs MongoDB to generate config, install MongoDB and rerun."
    exit 1
  fi

  # 3) Mongo detection AFTER first run
  local mongo_installed="false"
  if have_cmd mongod || systemctl list-unit-files 2>/dev/null | grep -qE '^mongod\.service'; then
    mongo_installed="true"
  fi

  local mongo_installed_by_script="false"
  local mongo_major_channel=""

  local MONGO_URI=""
  local MONGO_USER=""
  local MONGO_PASS=""

  if [[ "$mongo_installed" == "false" ]]; then
    echo
    read -r -p "MongoDB not detected. Install MongoDB automatically? [Y/n]: " INSTALL_MONGO
    INSTALL_MONGO="${INSTALL_MONGO:-Y}"
    if [[ "$INSTALL_MONGO" =~ ^[Yy]$ ]]; then
      mongo_major_channel="$(install_mongodb_latest_available)"
      mongo_installed_by_script="true"

      read -r -p "MongoDB admin username [root]: " MONGO_USER
      MONGO_USER="${MONGO_USER:-root}"

      read -r -p "Generate a strong random MongoDB password? [Y/n]: " GEN_PASS
      GEN_PASS="${GEN_PASS:-Y}"
      if [[ "$GEN_PASS" =~ ^[Yy]$ ]]; then
        MONGO_PASS="$(openssl rand -hex 24)"
        echo "Generated MongoDB password: ${MONGO_PASS}"
      else
        read -r -s -p "MongoDB admin password: " MONGO_PASS
        echo
        [[ -n "$MONGO_PASS" ]] || die "MongoDB password is required."
      fi

      mongo_create_admin_user "$MONGO_USER" "$MONGO_PASS" || true
      enable_mongodb_auth_localhost_only

      MONGO_URI="mongodb://${MONGO_USER}:${MONGO_PASS}@localhost:27017/admin"
    else
      warn "Skipping MongoDB installation. You must provide a valid MongoDB URI."
      read -r -p "MongoDB URI (e.g. mongodb://user:pass@host:27017/admin): " MONGO_URI
      [[ -n "${MONGO_URI}" ]] || die "MongoDB URI is required."
    fi
  else
    warn "MongoDB detected. Will NOT modify MongoDB configuration."
    echo
    read -r -p "MongoDB URI (e.g. mongodb://user:pass@localhost:27017/admin): " MONGO_URI
    [[ -n "${MONGO_URI}" ]] || die "MongoDB URI is required."
  fi

  # 4) Ask Wyron config values AFTER config exists
  read -r -p "Database name for Wyron (database_name) [wyron]: " DB_NAME
  DB_NAME="${DB_NAME:-wyron}"

  read -r -p "Panel port [8000]: " PANEL_PORT
  PANEL_PORT="${PANEL_PORT:-8000}"

  read -r -p "Listen address (recommended 127.0.0.1) [127.0.0.1]: " LISTEN_ON
  LISTEN_ON="${LISTEN_ON:-127.0.0.1}"

  read -r -p "Log level [info]: " LOG_LEVEL
  LOG_LEVEL="${LOG_LEVEL:-info}"

  echo
  read -r -p "Main domain (e.g. panel.example.com) (leave empty to use localhost): " MAIN_DOMAIN

  local MAIN_ADDRESS=""
  local ALLOWED_HOSTS=""
  local ALLOWED_ORIGINS=""
  local TLS_ENABLED="false"

  if [[ -n "$MAIN_DOMAIN" ]]; then
    read -r -p "Use TLS in Wyron config? (true/false) [false]: " TLS_ENABLED
    TLS_ENABLED="${TLS_ENABLED:-false}"

    if [[ "$TLS_ENABLED" == "true" ]]; then
      MAIN_ADDRESS="https://${MAIN_DOMAIN}"
      ALLOWED_ORIGINS="https://${MAIN_DOMAIN},http://${MAIN_DOMAIN}"
    else
      MAIN_ADDRESS="http://${MAIN_DOMAIN}"
      ALLOWED_ORIGINS="http://${MAIN_DOMAIN},https://${MAIN_DOMAIN}"
    fi

    read -r -p "Allowed hosts CSV [${MAIN_DOMAIN}]: " ALLOWED_HOSTS
    ALLOWED_HOSTS="${ALLOWED_HOSTS:-$MAIN_DOMAIN}"

    read -r -p "Allowed origins CSV [${ALLOWED_ORIGINS}]: " AO
    ALLOWED_ORIGINS="${AO:-$ALLOWED_ORIGINS}"
  else
    MAIN_ADDRESS="http://localhost:${PANEL_PORT}"
    ALLOWED_HOSTS="localhost"
    ALLOWED_ORIGINS="http://localhost:${PANEL_PORT}"
  fi

  # 5) Patch generated config.yml
  patch_wyron_config_after_first_run \
    "$MONGO_URI" "$DB_NAME" "$PANEL_PORT" "$LISTEN_ON" \
    "$MAIN_ADDRESS" "$ALLOWED_HOSTS" "$ALLOWED_ORIGINS" \
    "$TLS_ENABLED" "$LOG_LEVEL"

  # 6) Detect Wyron systemd unit (created by Wyron itself)
  if systemctl list-unit-files | grep -qi 'wyron'; then
    log "Wyron-related systemd unit detected:"
    systemctl list-unit-files | grep -i wyron || true
  else
    warn "No Wyron-related systemd unit detected. Wyron may not have installed a service yet."
  fi
  # 2) Patch generated config.yml with user answers
  patch_wyron_config_after_first_run \
    "$MONGO_URI" "$DB_NAME" "$PANEL_PORT" "$LISTEN_ON" \
    "$MAIN_ADDRESS" "$ALLOWED_HOSTS" "$ALLOWED_ORIGINS" \
    "$TLS_ENABLED" "$LOG_LEVEL"

  systemctl daemon-reload
  systemctl enable wyron
  
  echo
  echo "========================================"
  echo "Install complete."
  echo "Install dir: ${WYRON_DIR}"
  echo "Config:      ${CONFIG_FILE}"
  echo "Service:     wyron (systemd)"
  echo
  if [[ "$mongo_installed_by_script" == "true" ]]; then
    echo "MongoDB: installed by script (channel ${mongo_major_channel})."
    echo "MongoDB URI used in config: ${MONGO_URI}"
  fi
  if [[ ! -f "${WYRON_DIR}/license.key" ]]; then
    echo "NOTE: license.key not found in ${WYRON_DIR}."
    echo "Run:  ${WYRON_BIN} start"
    echo "Get fingerprint, request license.key, then place it into ${WYRON_DIR}."
  fi
  echo "Next steps:"
  echo "  If license.key is present in ${WYRON_DIR}:"
  echo "    ${WYRON_BIN} start"
  echo "  Otherwise:"
  echo "    Check ${WYRON_DIR}/first-run.log for Fingerprint and request license.key."
  echo "    Then run: ${WYRON_BIN} start"
}

main "$@"
