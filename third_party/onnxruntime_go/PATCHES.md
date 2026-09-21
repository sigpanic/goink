# Local patches

## Upstream baseline

| | |
|---|---|
| Module | `github.com/yalue/onnxruntime_go` |
| Version | `v1.30.1` |
| Commit | `1f580ef4e09615b787275b3777b61ab97f24c086` |
| Released | 2026-05-11T12:33:59Z |
| Source | https://github.com/yalue/onnxruntime_go/tree/v1.30.1 |

This directory is that release verbatim, except for the patch below and the
files listed under "Vendoring changes".

## Patch: `RunOptions.AddRunConfigEntry`

Goink adds only `RunOptions.AddRunConfigEntry`, including the corresponding C
wrapper declaration and implementation. The method exposes ONNX Runtime's
existing `OrtApi.AddRunConfigEntry` API so a run can request CPU memory arena
shrinkage (`memory.enable_memory_arena_shrinkage=cpu:0`).

| File | Change |
|---|---|
| `onnxruntime_go.go` | +16 lines: `RunOptions.AddRunConfigEntry` |
| `onnxruntime_wrapper.c` | +5 lines: `AddRunConfigEntry` wrapper |
| `onnxruntime_wrapper.h` | +3 lines: wrapper declaration |

## Vendoring changes

Dropped from the upstream tree because Goink does not build them:
`.gitattributes`, `onnxruntime_test.go`, `test_data/`.

## Upgrading upstream

1. Copy the new release over this directory.
2. Re-apply the patch above (three small additions, see the table).
3. Drop `.gitattributes`, `onnxruntime_test.go`, `test_data/` again.
4. Update the baseline table at the top of this file.
5. Verify: `go build ./...` and `go test -tags=cgo,e2e ./internal/e2e/...`.

## Removal condition

Remove this local module and the root `go.mod` `replace` when upstream offers an
equivalent public method. Checked on 2026-09-21: the latest release `v1.31.0`
(commit `06ff58a2b12e391fae3382f52a3906c3d038eeea`) still does not provide it,
so the patch is still required.
