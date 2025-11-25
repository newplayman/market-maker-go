#!/bin/bash

# 启动Prometheus和Grafana监控系统

echo "正在启动Prometheus和Grafana监控系统..."

# 检查Docker是否运行
if ! docker info >/dev/null 2>&1; then
  echo "错误: Docker未运行，请先启动Docker服务"
  exit 1
fi

# 进入监控目录并启动服务
cd /root/market-maker-go/monitoring

# 启动服务
docker compose up -d

if [ $? -eq 0 ]; then
  echo "Prometheus和Grafana已成功启动！"
  echo ""
  echo "访问地址:"
  echo "  Prometheus: http://localhost:9090"
  echo "  Grafana: http://localhost:3001"
  echo ""
  echo "Grafana默认登录信息:"
  echo "  用户名: admin"
  echo "  密码: admin"
  echo ""
  echo "注意: 启动market-maker-go时请使用 -metricsAddr :8080 参数"
else
  echo "启动失败，请检查错误信息"
  exit 1
fi