#!/usr/bin/env bash
#
# 快速实盘测试脚本：
# 1. 限定运行时长（默认60秒）；
# 2. 结束后自动执行紧急停止，确保撤单与平仓。
#

set -euo pipefail

DURATION="${DURATION:-60}"
CONFIG_PATH="${CONFIG_PATH:-configs/config.yaml}"
SYMBOL="${SYMBOL:-ETHUSDC}"
METRICS_ADDR="${METRICS_ADDR:-:9200}"
SYMBOL_LOWER="$(echo "$SYMBOL" | tr '[:upper:]' '[:lower:]')"

if [[ $# -gt 0 ]]; then
  DURATION="$1"
  shift
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

CMD=(./build/trader -config "$CONFIG_PATH" -symbol "$SYMBOL" -metricsAddr "$METRICS_ADDR")
if [[ $# -gt 0 ]]; then
  CMD+=("$@")
fi

START_TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

echo ">>> 启动实盘测试：时长=${DURATION}s config=${CONFIG_PATH} symbol=${SYMBOL}"
set +e
"${CMD[@]}" &
RUN_PID=$!
trap 'if kill -0 "$RUN_PID" >/dev/null 2>&1; then echo ">>> 捕获信号，强制停止 runner ($RUN_PID)"; kill -TERM "$RUN_PID" >/dev/null 2>&1; fi' INT TERM

DRAIN_BEFORE="${DRAIN_BEFORE_SEC:-8}"
DRAIN_GRACE="${DRAIN_GRACE_SEC:-12}"
if (( DRAIN_BEFORE >= DURATION )); then
  DRAIN_BEFORE=$(( DURATION / 2 ))
  if (( DRAIN_BEFORE < 3 )); then
    DRAIN_BEFORE=3
  fi
fi
RUN_WINDOW=$(( DURATION - DRAIN_BEFORE ))
if (( RUN_WINDOW > 0 )); then
  sleep "$RUN_WINDOW"
fi

if kill -0 "$RUN_PID" >/dev/null 2>&1; then
  echo ">>> 触发提前 Drain (向 PID $RUN_PID 发送 SIGINT)..."
  kill -INT "$RUN_PID" >/dev/null 2>&1 || true
fi

SECONDS_WAITED=0
while kill -0 "$RUN_PID" >/dev/null 2>&1; do
  if (( SECONDS_WAITED >= DRAIN_GRACE )); then
    echo ">>> Drain 等待超时，发送 SIGTERM..."
    kill -TERM "$RUN_PID" >/dev/null 2>&1 || true
    break
  fi
  sleep 1
  SECONDS_WAITED=$(( SECONDS_WAITED + 1 ))
done

wait "$RUN_PID"
STATUS=$?
trap - INT TERM

echo ">>> 测试进程退出，状态码=${STATUS}，执行紧急停止..."
if ./scripts/emergency_stop.sh; then
  echo ">>> 紧急停止完成。"
else
  echo ">>> 紧急停止失败，请手动检查！" >&2
fi

LOG_DIR="logs"
CSV_PATH="${LOG_DIR}/net_value_${SYMBOL_LOWER}.csv"
RUN_SUMMARY="${LOG_DIR}/net_value_runs_${SYMBOL_LOWER}.csv"
mkdir -p "$LOG_DIR"
END_TS="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

if [[ -f "$CSV_PATH" ]]; then
  python3 - "$CSV_PATH" "$START_TS" "$END_TS" "$RUN_SUMMARY" <<'PY'
import csv
import datetime as dt
import os
import sys
from decimal import Decimal

csv_path, start_ts, end_ts, summary_path = sys.argv[1:5]
start = dt.datetime.fromisoformat(start_ts.replace("Z", "+00:00"))
end = dt.datetime.fromisoformat(end_ts.replace("Z", "+00:00"))
rows = []
with open(csv_path) as f:
    reader = csv.reader(f)
    header = next(reader, None)
    for row in reader:
        if not row or len(row) < 6:
            continue
        ts = dt.datetime.fromisoformat(row[0].replace("Z", "+00:00"))
        if start <= ts <= end:
            try:
                wallet = Decimal(row[4])
                equity = Decimal(row[5])
            except Exception:
                continue
            fee = Decimal(row[6]) if len(row) > 6 and row[6] else Decimal(0)
            rows.append((ts, equity, wallet, fee))
if not rows:
    print(">>> 未在 %s ~ %s 范围内找到净值记录" % (start_ts, end_ts))
    sys.exit(0)
duration = (rows[-1][0] - rows[0][0]).total_seconds()
equities = [r[1] for r in rows]
delta = equities[-1] - equities[0]
min_eq = min(equities)
max_eq = max(equities)
wallet_delta = rows[-1][2] - rows[0][2]
fees_delta = rows[-1][3] - rows[0][3]
print(">>> 净值统计：条数=%d 时长=%.1fs Δ=%.6f (起=%.6f 终=%.6f) 极值[min=%.6f max=%.6f] 现金Δ=%.6f 手续费Δ=%.6f" %
      (len(rows), duration, float(delta), float(equities[0]), float(equities[-1]),
       float(min_eq), float(max_eq), float(wallet_delta), float(fees_delta)))

SUMMARY_HEADER = "start_ts,end_ts,duration_s,delta,eq_start,eq_end,eq_min,eq_max,wallet_delta,fees_delta"

def ensure_summary_header(path, header):
    if not os.path.exists(path):
        return True
    with open(path, "r") as existing:
        lines = existing.readlines()
    if not lines:
        with open(path, "w") as out:
            out.write(header + "\n")
        return False
    current_header = lines[0].strip()
    if current_header == header:
        return False
    have_old_header = current_header.lower().startswith("start_ts")
    data_lines = lines[1:] if have_old_header else lines
    with open(path, "w") as out:
        out.write(header + "\n")
        for line in data_lines:
            line = line.rstrip("\n")
            if not line:
                continue
            comma_count = line.count(",")
            if comma_count >= 9:
                out.write(f"{line}\n")
            else:
                out.write(f"{line},,\n")
    return False

write_header = ensure_summary_header(summary_path, SUMMARY_HEADER)
with open(summary_path, "a") as out:
    if write_header:
        out.write(SUMMARY_HEADER + "\n")
    out.write("%s,%s,%.1f,%.10f,%.10f,%.10f,%.10f,%.10f,%.10f,%.10f\n" %
              (rows[0][0].isoformat().replace("+00:00", "Z"),
               rows[-1][0].isoformat().replace("+00:00", "Z"),
               duration,
               float(delta),
               float(equities[0]),
               float(equities[-1]),
               float(min_eq),
               float(max_eq),
               float(wallet_delta),
               float(fees_delta)))
PY
else
  echo ">>> 未找到净值文件: $CSV_PATH"
fi

exit $STATUS
