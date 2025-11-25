#!/bin/bash

# 实盘交易启动脚本

echo "🚀 准备启动实盘交易系统"
echo "======================"

# 检查配置文件
if [ ! -f "./configs/config_real_trading.yaml" ]; then
    echo "❌ 错误: 找不到实盘配置文件 configs/config_real_trading.yaml"
    exit 1
fi

echo "✅ 配置文件检查通过"

# 检查API密钥
API_KEY=$(grep "apiKey:" ./configs/config_real_trading.yaml | cut -d '"' -f 2)
API_SECRET=$(grep "apiSecret:" ./configs/config_real_trading.yaml | cut -d '"' -f 2)

if [ -z "$API_KEY" ] || [ -z "$API_SECRET" ]; then
    echo "❌ 错误: API密钥未配置或格式不正确"
    exit 1
fi

echo "✅ API密钥已配置"

# 显示交易对和基本参数
SYMBOL=$(grep "ETHUSDC:" ./configs/config_real_trading.yaml -A 15 | grep -E "^[[:space:]]+[a-zA-Z]" | head -1 | awk '{print $1}' | sed 's/://')
BASE_SIZE=$(grep "BaseSize:" ./configs/config_real_trading.yaml | awk '{print $2}')
MAX_EXPOSURE=$(grep "maxNetExposure:" ./configs/config_real_trading.yaml | awk '{print $2}')

echo "📈 交易对: ETHUSDC"
echo "📊 基础订单大小: $BASE_SIZE"
echo "🔒 最大净敞口: $MAX_EXPOSURE"

# 确认启动
echo ""
echo "⚠️  ⚠️  ⚠️  重要提醒  ⚠️  ⚠️  ⚠️"
echo "这将是实盘交易，使用真实资金在真实市场进行交易！"
echo ""
read -p "请输入 'TRADE' 确认启动实盘交易: " confirmation

if [ "$confirmation" != "TRADE" ]; then
    echo "操作已取消"
    exit 0
fi

echo ""
echo "🏁 启动实盘交易系统..."

# 启动实盘交易
nohup ./runner -config configs/config_real_trading.yaml -dryRun=false -metricsAddr :9100 > /var/log/market-maker-real.log 2>&1 &
RUNNER_PID=$!

echo "✅ 实盘交易系统已在后台启动"
echo "进程PID: $RUNNER_PID"
echo "日志文件: /var/log/market-maker-real.log"
echo ""
echo "使用以下命令查看日志:"
echo "tail -f /var/log/market-maker-real.log"
echo ""
echo "使用以下命令停止交易:"
echo "pkill -f runner"