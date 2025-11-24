#!/usr/bin/env bash
#
# 环境变量兼容性设置
# 自动将 BINANCE_* 转换为 MM_GATEWAY_*
#

# 如果设置了BINANCE_*且没有MM_GATEWAY_*，则自动转换
if [ -n "${BINANCE_API_KEY:-}" ] && [ -z "${MM_GATEWAY_API_KEY:-}" ]; then
    export MM_GATEWAY_API_KEY="$BINANCE_API_KEY"
fi

if [ -n "${BINANCE_API_SECRET:-}" ] && [ -z "${MM_GATEWAY_API_SECRET:-}" ]; then
    export MM_GATEWAY_API_SECRET="$BINANCE_API_SECRET"
fi
