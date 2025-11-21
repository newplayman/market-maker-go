# 配置映射与校验指南

目标：将策略/风控/网关的关键参数映射到 config，并提供校验与示例。

## 已有字段（config/AppConfig）
- env: dev/prod
- risk: maxOrderValueUSDT, maxNetExposure
- gateway: apiKey, apiSecret, baseURL
- inventory: targetPosition, maxDrift

## 建议新增/映射字段
- strategy:
  - minSpread (bps)
  - baseSize
  - targetPosition
  - maxDrift
  - gridLevels / volFactor 等（待细化）
- risk 扩展:
  - singleMax / dailyMax / netMax（已在风控模块使用）
  - maxSpreadRatio (VWAP guard)
  - minPnL / maxPnL (PnL guard)
  - minIntervalMs (频率限制)
- gateway:
  - wsEndpoint (可选覆盖默认)

## 校验建议
- 所有 >0 的数值字段：MinSpread/BaseSize/MaxDrift/风险阈值必须 >0
- APIKey/Secret 必填（或通过 env 覆盖）
- 当组合字段存在（如 maxSpreadRatio、minIntervalMs）时，确保挂载风控守卫

## 示例 config 结构（yaml 概念稿）
```yaml
env: dev
strategy:
  minSpread: 0.001
  baseSize: 0.5
  targetPosition: 0
  maxDrift: 1
risk:
  singleMax: 5
  dailyMax: 50
  netMax: 10
  maxSpreadRatio: 0.05
  minIntervalMs: 200
  minPnL: -50
  maxPnL: 200
gateway:
  apiKey: your_api_key
  apiSecret: your_api_secret
  baseURL: https://fapi.binance.com
  wsEndpoint: wss://fstream.binance.com
inventory:
  targetPosition: 0
  maxDrift: 1
```

以上为离线文档，可在接入配置前参照。***
