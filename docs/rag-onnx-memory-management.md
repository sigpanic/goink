# RAG ONNX 内存管理设计

## 背景

全量向量重建会连续执行大批量 ONNX 推理。旧实现单批最多 64 条，Linux
实测峰值约 4.3 GB；推理结束后，ONNX Runtime 的 CPU memory arena 默认保留
已申请的内存供后续推理复用，因此 RSS 不会自动回落。

关闭 CPU arena 不能稳定解决问题：真实模型测试中，释放后的 native 内存会被
glibc 的多线程 allocator 缓存，RSS 随批次数增加。Go GC 只能回收 Go 堆，不能
整理 ONNX Runtime 或 libc 管理的 native 堆。

## 方案

1. 单次 ONNX batch 上限保持为 8，推理线程限制为 2～4。
2. 开启 CPU memory arena，利用其稳定的高水位复用行为，避免 native allocator
   随连续推理逐步增长。
3. 普通查询、章节增量刷新不触发 arena shrink。
4. `RebuildAll` 完成全部小说后仅 shrink 一次；手动执行完整 `RebuildNovel` 时也
   在任务结束后 shrink 一次。
5. shrink 通过一次很小的 ONNX Run 触发，并使用 RunOptions 配置
   `memory.enable_memory_arena_shrinkage=cpu:0`。这次推理的结果直接丢弃。
6. shrink 是内存优化，不影响索引正确性；失败只记录警告，不覆盖重建结果。

`RebuildAll` 调用内部的 `rebuildNovel`，避免每部小说重建后重复 shrink：

```text
RebuildNovel              RebuildAll
├── rebuildNovel          ├── rebuildNovel × N
└── compact once          └── compact once
```

## Go binding

当前 `github.com/yalue/onnxruntime_go v1.30.1` 已提供 `RunOptions` 和
`RunWithOptions`，但没有封装底层 C API `AddRunConfigEntry`。项目保留一个最小的
本地补丁模块，只新增 `RunOptions.AddRunConfigEntry`，其余源码保持上游版本不变。
`go.mod` 使用本地 `replace`，避免依赖 `unsafe` 读取第三方包私有字段。

后续上游若提供等价 API，应删除本地补丁并恢复官方 module 依赖。

## 实测基线

Linux、ONNX Runtime 1.26.0、bge-small-zh-v1.5 int8、4 个 intra-op 线程，
batch=8、接近 512 token：

| 策略 | 20 批后 RSS | 20 批耗时 | 收尾结果 |
| --- | ---: | ---: | ---: |
| arena 开启，不 shrink | 363 MB | 4.14 s | 保持约 363 MB |
| 每次 Run 都 shrink | 413 MB | 5.13 s | native allocator 仍保留内存 |
| 最后 tiny Run shrink | 363 MB | 约 4 s | 降至 93 MB；Go GC 后约 68 MB |

数值只作为当前 Linux 测试环境的回归基线。Windows 使用相同 ORT arena 语义，
但 working set 和 private bytes 的统计及底层 allocator 行为不同，需要分别实测。

## 风险与验证

- shrink 会增加一次约几十毫秒的小推理，仅发生在完整重建结束时。
- shrink 和普通推理共用 embedder mutex，避免同时执行 session Run。
- 重建中途失败且已经执行过推理时，仍尝试 shrink。
- 验证 `go test ./internal/rag`、`go test ./...`，并使用真实模型确认 shrink 后仍可
  正常生成 embedding，同时记录 shrink 前后的 RSS。
