#!/usr/bin/env bash
#
# 在 Ubuntu/Debian 上安装 Prometheus + node_exporter + Alertmanager 的一键脚本。
# 说明：
#   1. 默认安装到 /opt/market-monitoring。
#   2. 需要 root 权限（创建用户、systemd 服务）。
#   3. Prometheus 会抓取本机 9100 端口（runner metrics），并监听 9090。
#   4. Alertmanager 默认监听 9093，示例里会把告警写到本地日志；你可以替换为邮件/钉钉。
#
set -euo pipefail

PROM_VER="2.45.0"
NODE_VER="1.7.0"
ALERT_VER="0.27.0"
INSTALL_ROOT="/opt/market-monitoring"
DATA_ROOT="/var/lib/market-monitoring"
USER="monitor"

mkdir -p "${INSTALL_ROOT}" "${DATA_ROOT}/prometheus" "${DATA_ROOT}/alertmanager"

if ! id -u "${USER}" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "${USER}"
fi

download_and_extract() {
  local name=$1 ver=$2 url=$3 dest=$4
  local dir="${INSTALL_ROOT}/${name}-${ver}"
  if [[ -d "${dir}" ]]; then
    echo "[${name}] 已存在，跳过下载"
    ln -sf "${dir}" "${dest}"
    return
  fi
  tmp=$(mktemp -d)
  curl -sSL "${url}" -o "${tmp}/${name}.tar.gz"
  mkdir -p "${dir}"
  tar -xzf "${tmp}/${name}.tar.gz" -C "${dir}" --strip-components=1
  rm -rf "${tmp}"
  ln -sf "${dir}" "${dest}"
}

download_and_extract "prometheus" "${PROM_VER}" \
  "https://github.com/prometheus/prometheus/releases/download/v${PROM_VER}/prometheus-${PROM_VER}.linux-amd64.tar.gz" \
  "${INSTALL_ROOT}/prometheus"
download_and_extract "node_exporter" "${NODE_VER}" \
  "https://github.com/prometheus/node_exporter/releases/download/v${NODE_VER}/node_exporter-${NODE_VER}.linux-amd64.tar.gz" \
  "${INSTALL_ROOT}/node_exporter"
download_and_extract "alertmanager" "${ALERT_VER}" \
  "https://github.com/prometheus/alertmanager/releases/download/v${ALERT_VER}/alertmanager-${ALERT_VER}.linux-amd64.tar.gz" \
  "${INSTALL_ROOT}/alertmanager"

chown -R "${USER}:${USER}" "${INSTALL_ROOT}"/{prometheus,prometheus-${PROM_VER}} \
  "${INSTALL_ROOT}"/{alertmanager,alertmanager-${ALERT_VER}} \
  "${DATA_ROOT}"

cat >"${INSTALL_ROOT}/prometheus.yml" <<'EOF'
global:
  scrape_interval: 5s

scrape_configs:
  - job_name: "runner"
    static_configs:
      - targets: ["127.0.0.1:9100"]
  - job_name: "node"
    static_configs:
      - targets: ["127.0.0.1:9101"]

rule_files:
  - /opt/market-monitoring/rules.yml
EOF

cat >"${INSTALL_ROOT}/rules.yml" <<'EOF'
groups:
  - name: runner_alerts
    rules:
      - alert: RunnerWSDisconnected
        expr: increase(runner_ws_failures_total[1m]) > 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Runner WS 断开"
          description: "过去 1 分钟内 WS 断线次数 >0，请检查网络/交易所状态。"
      - alert: RunnerRestErrors
        expr: increase(runner_rest_requests_total{action="place"}[5m]) > 0
          and
          increase(runner_rest_latency_seconds_count{action="place"}[5m]) == 0
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Runner REST 异常"
          description: "5 分钟内 REST 请求无成功记录，请排查 API Key/网络。"
EOF

cat >"${INSTALL_ROOT}/alertmanager.yml" <<'EOF'
route:
  receiver: "default"
  group_wait: 10s
  group_interval: 1m

receivers:
  - name: "default"
    webhook_configs:
      - url: "http://127.0.0.1:19093/"
        send_resolved: true
EOF

cat >/etc/systemd/system/prometheus.service <<EOF
[Unit]
Description=Prometheus Monitoring
After=network-online.target

[Service]
User=${USER}
Group=${USER}
Type=simple
ExecStart=${INSTALL_ROOT}/prometheus/prometheus \\
  --config.file=${INSTALL_ROOT}/prometheus.yml \\
  --storage.tsdb.path=${DATA_ROOT}/prometheus \\
  --web.listen-address=:9090
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/node_exporter.service <<EOF
[Unit]
Description=Node Exporter
After=network-online.target

[Service]
User=${USER}
Group=${USER}
Type=simple
ExecStart=${INSTALL_ROOT}/node_exporter/node_exporter --web.listen-address=:9101
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/alertmanager.service <<EOF
[Unit]
Description=Alertmanager
After=network-online.target

[Service]
User=${USER}
Group=${USER}
Type=simple
ExecStart=${INSTALL_ROOT}/alertmanager/alertmanager \\
  --config.file=${INSTALL_ROOT}/alertmanager.yml \\
  --storage.path=${DATA_ROOT}/alertmanager \\
  --web.listen-address=:9093
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now prometheus.service node_exporter.service alertmanager.service

echo "Prometheus/NodeExporter/Alertmanager 已安装；Prometheus: http://<host>:9090"
