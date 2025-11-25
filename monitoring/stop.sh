#!/bin/bash

# 停止Prometheus和Grafana监控系统

echo "正在停止Prometheus和Grafana监控系统..."

# 进入监控目录并停止服务
cd /root/market-maker-go/monitoring

# 停止服务
docker compose down

if [ $? -eq 0 ]; then
  echo "Prometheus和Grafana已成功停止！"
else
  echo "停止失败，请检查错误信息"
  exit 1
fi