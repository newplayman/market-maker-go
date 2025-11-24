#!/usr/bin/env bash
#
# 启动交易 - 一步到位脚本
#

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Market Maker 启动脚本"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查环境变量
if [ -z "${BINANCE_API_KEY:-}" ] || [ -z "${BINANCE_API_SECRET:-}" ]; then
    echo -e "${RED}错误: 未设置API密钥${NC}"
    echo ""
    echo "请先设置环境变量："
    echo ""
    echo "  export BINANCE_API_KEY=\"your_api_key_here\""
    echo "  export BINANCE_API_SECRET=\"your_secret_here\""
    echo ""
    echo "或者编辑 ~/.bashrc 添加上述内容，然后运行:"
    echo "  source ~/.bashrc"
    echo ""
    exit 1
fi

echo -e "${GREEN}✓${NC} API密钥已设置"
echo ""

# 检查余额
echo "检查账户余额..."
if go run ./cmd/binance_balance -config configs/config.yaml; then
    echo ""
    echo -e "${GREEN}✓${NC} 余额检查通过"
else
    echo ""
    echo -e "${RED}✗${NC} 余额检查失败，请检查API密钥是否正确"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  准备启动交易"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "配置信息："
echo "  交易对: ETHUSDC"
echo "  订单大小: 0.001 ETH (约3 USDC)"
echo "  日亏损限制: 5 USDC"
echo "  最大持仓: 0.01 ETH"
echo ""
echo -e "${YELLOW}按 Ctrl+C 可随时停止程序${NC}"
echo ""
read -p "按回车键开始交易..."
echo ""

# 创建日志目录
mkdir -p /var/log/market-maker 2>/dev/null || sudo mkdir -p /var/log/market-maker

# 启动交易
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  开始交易"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

./build/trader -config configs/config.yaml
