#!/usr/bin/env bash
set -euo pipefail

# 简单的 Binance WS demo：订阅一个交易对的深度和用户数据流（如果提供 listenKey）。
# 依赖环境变量：BINANCE_WS_ENDPOINT, LISTEN_KEY (可选)

SYM=${1:-BTCUSDT}

BINANCE_WS_ENDPOINT=${BINANCE_WS_ENDPOINT:-"wss://fstream.binance.com"}

echo "WS demo for ${SYM} via ${BINANCE_WS_ENDPOINT}"
echo "Press Ctrl+C to exit."

python3 - <<'PY'
import os, json, websocket

sym = os.getenv("SYM", "BTCUSDT").lower()
base = os.getenv("BINANCE_WS_ENDPOINT", "wss://fstream.binance.com")
listen = os.getenv("LISTEN_KEY")

streams = [f"{sym}@depth20@100ms"]
if listen:
    streams.append(listen)

url = f"{base}/stream?streams={'/'.join(streams)}"
print("connecting:", url)

def on_message(ws, msg):
    data = json.loads(msg)
    print("recv stream:", data.get("stream"), "len", len(msg))

def on_error(ws, err):
    print("error:", err)

def on_close(ws, *args):
    print("closed")

ws = websocket.WebSocketApp(url, on_message=on_message, on_error=on_error, on_close=on_close)
ws.run_forever()
PY
