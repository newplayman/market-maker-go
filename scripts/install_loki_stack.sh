#!/usr/bin/env bash
#
# 安装 Loki + Promtail + Grafana（仅本地登录）的简易脚本。
# 注意：
#   - 需 root 权限。
#   - 安装路径默认 /opt/market-monitoring。
#   - Promtail 会读取 /root/market-maker-go/logs/*.log，你可按需修改。
set -euo pipefail

LOKI_VER="2.9.6"
PROMTAIL_VER="2.9.6"
GRAFANA_VER="10.4.3"
INSTALL_ROOT="/opt/market-monitoring"
DATA_ROOT="/var/lib/market-monitoring"
USER="monitor"

mkdir -p "${INSTALL_ROOT}" "${DATA_ROOT}/loki" "${DATA_ROOT}/grafana"
mkdir -p "${DATA_ROOT}/loki"/{chunks,index,cache,wal,compactor}

if ! id -u "${USER}" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "${USER}"
fi

install_zip_binary() {
  local name=$1 ver=$2 url=$3 binary=$4
  local dir="${INSTALL_ROOT}/${name}-${ver}"
  if [[ -d "${dir}" ]]; then
    echo "[${name}] 已存在，跳过"
    ln -sf "${dir}" "${INSTALL_ROOT}/${name}"
    return
  fi
  tmp=$(mktemp -d)
  pushd "${tmp}" >/dev/null
  curl -sSL "${url}" -o "${name}.zip"
  if ! command -v unzip >/dev/null 2>&1; then
    apt-get update && apt-get install -y unzip
  fi
  unzip -q "${name}.zip"
  mkdir -p "${dir}"
  mv "${binary}" "${dir}/"
  chmod +x "${dir}/${binary}"
  popd >/dev/null
  rm -rf "${tmp}"
  ln -sf "${dir}" "${INSTALL_ROOT}/${name}"
}

install_zip_binary "loki" "${LOKI_VER}" \
  "https://github.com/grafana/loki/releases/download/v${LOKI_VER}/loki-linux-amd64.zip" \
  "loki-linux-amd64"
install_zip_binary "promtail" "${PROMTAIL_VER}" \
  "https://github.com/grafana/loki/releases/download/v${PROMTAIL_VER}/promtail-linux-amd64.zip" \
  "promtail-linux-amd64"

if [[ ! -d "${INSTALL_ROOT}/grafana-${GRAFANA_VER}" ]]; then
  curl -sSL "https://dl.grafana.com/oss/release/grafana-${GRAFANA_VER}.linux-amd64.tar.gz" \
    | tar -xz -C "${INSTALL_ROOT}"
fi
ln -sf "${INSTALL_ROOT}/grafana-${GRAFANA_VER}" "${INSTALL_ROOT}/grafana"

chown -R "${USER}:${USER}" "${INSTALL_ROOT}"/{loki,promtail,grafana} "${DATA_ROOT}"

cat >"${INSTALL_ROOT}/loki-config.yml" <<'EOF'
auth_enabled: false
server:
  http_listen_port: 3100
ingester:
  lifecycler:
    address: 127.0.0.1
    ring:
      kvstore:
        store: inmemory
      replication_factor: 1
  chunk_idle_period: 5m
  wal:
    dir: /var/lib/market-monitoring/loki/wal
schema_config:
  configs:
    - from: 2024-01-01
      store: boltdb-shipper
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h
storage_config:
  boltdb_shipper:
    active_index_directory: /var/lib/market-monitoring/loki/index
    cache_location: /var/lib/market-monitoring/loki/cache
  filesystem:
    directory: /var/lib/market-monitoring/loki/chunks
limits_config:
  retention_period: 168h
chunk_store_config:
  max_look_back_period: 168h
compactor:
  working_directory: /var/lib/market-monitoring/loki/compactor
  shared_store: filesystem
table_manager:
  retention_deletes_enabled: true
  retention_period: 168h
EOF

cat >"${INSTALL_ROOT}/promtail-config.yml" <<'EOF'
server:
  http_listen_port: 9080
  grpc_listen_port: 0
positions:
  filename: /var/lib/market-monitoring/promtail-positions.yaml
clients:
  - url: http://127.0.0.1:3100/loki/api/v1/push
scrape_configs:
  - job_name: runner_logs
    static_configs:
      - targets: [localhost]
        labels:
          job: runner
          __path__: /root/market-maker-go/logs/*.log
EOF

cat >/etc/systemd/system/loki.service <<EOF
[Unit]
Description=Loki Log Storage
After=network-online.target

[Service]
User=${USER}
Group=${USER}
Type=simple
ExecStart=${INSTALL_ROOT}/loki/loki-linux-amd64 --config.file=${INSTALL_ROOT}/loki-config.yml
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/promtail.service <<EOF
[Unit]
Description=Promtail Log Shipper
After=network-online.target loki.service

[Service]
User=${USER}
Group=${USER}
Type=simple
ExecStart=${INSTALL_ROOT}/promtail/promtail-linux-amd64 --config.file=${INSTALL_ROOT}/promtail-config.yml
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/grafana.service <<EOF
[Unit]
Description=Grafana Dashboard
After=network-online.target

[Service]
User=${USER}
Group=${USER}
Type=simple
WorkingDirectory=${INSTALL_ROOT}/grafana
ExecStart=${INSTALL_ROOT}/grafana/bin/grafana-server --homepath=${INSTALL_ROOT}/grafana --config=${INSTALL_ROOT}/grafana/conf/defaults.ini
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now loki.service promtail.service grafana.service

echo "Loki(3100)/Promtail/Grafana(3000) 安装完成。Grafana 默认账号 admin/admin，请登录后修改密码并配置 Loki 数据源。"
