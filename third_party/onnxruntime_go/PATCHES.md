# 本地补丁

## 上游基线

| | |
|---|---|
| 模块 | `github.com/yalue/onnxruntime_go` |
| 版本 | `v1.30.1` |
| commit | `1f580ef4e09615b787275b3777b61ab97f24c086` |
| 发布 | 2026-05-11T12:33:59Z |
| 源码 | https://github.com/yalue/onnxruntime_go/tree/v1.30.1 |

本目录是该版本的完整源码，除下方补丁与「裁剪内容」列出的文件外与上游一致。

## 补丁：`RunOptions.AddRunConfigEntry`

Goink 只新增 `RunOptions.AddRunConfigEntry` 及其对应的 C wrapper 声明与实现，
用于暴露 ONNX Runtime 既有的 `OrtApi.AddRunConfigEntry`，使一次 Run 可以请求
CPU memory arena 收缩（`memory.enable_memory_arena_shrinkage=cpu:0`）。

| 文件 | 改动 |
|---|---|
| `onnxruntime_go.go` | +16 行：`RunOptions.AddRunConfigEntry` |
| `onnxruntime_wrapper.c` | +5 行：`AddRunConfigEntry` wrapper |
| `onnxruntime_wrapper.h` | +3 行：wrapper 声明 |

## 上游已有等价 API，但本项目暂不升级

上游自 **v1.34.0**（2026-08-18，PR #145）起已提供 `RunOptions.AddRunConfigEntry`，
v1.36.0 又补充了 `GetRunConfigEntry`。本项目**有意停留在 v1.30.1 + 本地补丁**，
不升级到上游版本，原因：

1. **绑定与 ORT runtime 版本强耦合**。绑定的 `ORT_API_VERSION` 在编译期固定，
   `GetApi` 在 runtime 低于该版本时返回 nullptr，初始化直接失败。v1.36.0 的
   API 版本为 29，要求 ORT runtime ≥ 1.29.0；而本项目捆绑的是 **1.26.0**
   （见 `scripts/download-onnx.sh` 的 `ONNX_VERSION`）。两者必须原子升级。
2. **ORT 1.29 引入 POSIX telemetry**。官方构建在 Linux/macOS 上默认开启 1DS
   上报（1.26/1.27/1.28 无此行为），对本桌面应用是隐私回归，需额外用
   `ORT_DISABLE_TELEMETRY=1` 关闭并验证。
3. **glibc 下限抬升**。ORT 1.26.0 需 GLIBC_2.27，1.29.1 需 GLIBC_2.28
   （新增符号 `fcntl64`）。AppImage 不捆绑 glibc，宿主 glibc 直接生效，因此
   升级会放弃 glibc 2.27 的系统（Ubuntu 18.04 LTS、Linux Mint 19.x）。
4. 安装包体积每平台约 +5.5 MB。

**重新评估的触发条件**（满足任一即可考虑升级）：

- 决定放弃 glibc 2.27 的系统，且确认 `ORT_DISABLE_TELEMETRY=1` 能可靠阻断上报；
- 需要上游 v1.34.0 之后的某项功能或修复；
- 上游在后续 ORT 版本移除或默认关闭 POSIX telemetry。

升级时需同步处理：`scripts/download-onnx.sh` 的 `ONNX_VERSION`、`go.mod` 的
`require` 与 `replace`、本目录，以及 Windows VC++ Runtime 与 macOS 部署目标的
CI 验证。

## 裁剪内容

以下上游文件未保留，因为 Goink 不编译它们：`.gitattributes`、
`onnxruntime_test.go`、`test_data/`。

## 升级上游版本时的步骤

1. 用新版本覆盖本目录。
2. 重新应用上方补丁（三处小改动，见表格）。
3. 再次删除 `.gitattributes`、`onnxruntime_test.go`、`test_data/`。
4. 更新本文顶部的基线表。
5. 验证：`go build ./...` 与 `go test -tags=cgo,e2e ./internal/e2e/...`。

## 维护风险

本目录由人工维护，目前缺少自动化守卫：

- **静默漂移**：任何人对本目录的改动都不会被察觉。可用 `git hash-object`
  比对每个文件与上游 tag 的 blob SHA，只允许上方表格中的 3 个文件不同。
- **版本落后**：`go.mod` 改用本地 `replace` 后，依赖更新工具不再跟踪该模块，
  上游发布新版本不会有任何提示。
