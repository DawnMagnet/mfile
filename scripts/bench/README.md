# mfile 语料、对比和基准

这里是 QA 用的小语料、三方对比、模型等价性门禁和性能基准。仓库根目录的 README 不在这次改动里。`internal/detect` 在当前 master 上还不存在，等价性测试先放在 `scripts/bench/accuracy/`（build tag `accuracy`），等检测包落地后再挪。

样本都由 `gen_corpus.py` 生成或现场编译，没有提交来历不明的二进制。大语料只写到临时目录，不进 git。

## 依赖

```bash
python3 -m pip install --user -r scripts/bench/requirements.txt
```

固定 `magika==1.0.3`（模型 `standard_v3_3`）。这个 wheel 同时带上 CLI `magika 1.1.0 standard_v3_3`。CLI 的 `--json` 把 score 截成 3 位小数，不能用来做 1e-4 的等价性判断；脚本里的 score 对比走 Python API。

另外需要本机的 `file(1)`、`gcc`（语料里的 ELF）和 [hyperfine](https://github.com/sharkdp/hyperfine)（性能脚本）。`gcc` 版本会写进 ELF 的 `.comment`，换编译器再生成，字节会变，类型不会变。

## 一条命令

在仓库根目录执行。

```bash
# 小语料（写入 testdata/corpus，约几百 KB）
python3 scripts/bench/gen_corpus.py

# 三方准确率：(a) v1、(b) simd、(c) 同一个 simd 二进制加 GODEBUG=simd=0
# 以及 file、magika。三种构建 label 不一致或 score 差超过 1e-4 时退出码为 1。
python3 scripts/bench/compare

# 等价性门禁。见下面的「门禁」一节。
python3 scripts/bench/equiv

# 性能。会在 /tmp/mfile-bench-large 生成约 1.2 万个小文件和两个 32 MiB 文件。
python3 scripts/bench/perf

# 只跑当前这个测试二进制的进程内门禁（不含三种构建的交叉对比）
go test -tags accuracy -count=1 -timeout 30m ./scripts/bench/accuracy/
```

不加 `accuracy` tag 时，`go test ./...` 看不到这些测试。

构建约定（amd64 只发一个包，启动保护还没进 master）：

| 名称 | 怎么来的 |
|---|---|
| (a) v1 | `CGO_ENABLED=0 GOAMD64=v1` |
| (b) simd | `CGO_ENABLED=0 GOEXPERIMENT=simd GOAMD64=v1` |
| (c) simd_off | (b) 的二进制，运行时加 `GODEBUG=simd=0` |

`GODEBUG=simd=0` 让 Go 的 `simd` 包走模拟，不走硬件向量指令。这是启动保护合入之后，在「有 AVX、没有 AVX2」的 CPU 上重新 exec 自己时要用的那条路径。启动保护本身是第 2 项，现在还没有。

gomlx 自己的 AVX2/AVX-512 matmul 核看的是 CPUID 和 `GOMLX_GO_SIMD_AVX2` / `GOMLX_GO_SIMD_AVX512`，不看 `GODEBUG`。本机有 AVX-512 时，(c) 仍会用这些核。脚本里多了一组补充运行 `sandy_like`：在 (c) 的环境上再把这两个变量设为 0，用来近似 Sandy Bridge 上的核选择。它不是发布矩阵里的第四个包。

## 语料

`testdata/corpus/expected.tsv` 的列：

| 列 | 含义 |
|---|---|
| path | 相对语料根目录 |
| label | 期望的 Magika 输出 label |
| mime | 期望的 MIME。sqlite 和 sum 在 KB 里是 null，这里写的是应有的类型 |
| file_substr | `file -b` 输出里应出现的子串，用来给 file 算 label 准确率 |
| known_fail | `1` 表示已知失败，目前只有 `byname/go.mod` |
| notes | 说明 |

`go.mod` 的期望 label 是 `txt`（词表里没有 gomod）。两种构建都会把它判成 `verilog`（score 约 0.745）。这是模型的结果，计划在第 5 项用文件名层盖掉。样本留在语料里，不从分母里拿掉。

大语料：

```bash
python3 scripts/bench/gen_large.py --out /tmp/mfile-bench-large --small 12000 --big-mib 32
```

## 门禁

`scripts/bench/equiv` 会：

1. 用 (a) 和 (b) 各编译一份 `go test -tags accuracy -c` 的测试二进制。
2. 每个进程里先把整份语料以 workers=1、batch=1 扫 5 轮，再换 workers/batch（2/1、4/1、4/4、1/8）。
3. 对 (a)、(b)、(c) 各再跑一遍 `GOGC=1`。
4. 和 magika Python API 比 dl label（模型没跑时，mfile 的空 dl 和 magika 的 `undefined` 视为相同）和 score，阈值 `1e-4`。
5. 用 onnxruntime 在同一批特征上对比 `model.onnx` 和 `model.patched.onnx`，用来判断残差来自 `tools/patch_model.py` 还是 Go 的 float32。
6. 用 `-gcflags=all=-d=checkptr` 再编一份 CLI，在 `GODEBUG=checkptr=2` 下扫一遍语料，对照 (a)。`unsafePackRHS` 会碰到 checkptr，这里就是在看它会不会让结果悄悄变掉。

本机实测没有达到 1e-4：三种构建的 label 全部一致，score 最大差约 3.4e-4，来自 Go 后端的 float，不是 `tools/patch_model.py`（onnxruntime 上原始模型和改写模型的最大差是 9.5e-7）。因此当前 `scripts/bench/equiv` 退出码是 1。超标文件和多轮稳定性写在 `BASELINE.md`。

进程内 5 轮和 magika 的对比也可以单独跑：

```bash
GOGC=1 go test -tags accuracy -count=1 -timeout 30m -v ./scripts/bench/accuracy/
```

`GOGC` 和 `GODEBUG` 是进程级的，测试自己不会改它们。要覆盖 GC 压力和 simd 回退，用 `scripts/bench/equiv`。

## 性能脚本量什么

`scripts/bench/perf` 对 (a)(b)(c) 以及补充的 `sandy_like` 报告：

- 二进制体积
- 单文件冷启动的墙钟时间（hyperfine，warmup 1；每次进程仍会重新编译计算图）
- 启动拆分：`NewScanner`（加载模型、建执行器）、计算图编译、单次推理。编译时间用 `ScanAll(1) - infer` 估算，`infer = (ScanAll(32) - ScanAll(1)) / 31`
- 约 1.2 万个小文件的吞吐（文件数/秒）和峰值 RSS（`-stats` 的 VmHWM，以及 `wait4` 的 `ru_maxrss`）
- workers ∈ {1,4,8}、batch ∈ {1,4,16} 在 2000 个文件上的墙钟和峰值 RSS
- 两个 32 MiB 文件
- 同一批小文件上的 `file` 和 `magika --json`

没有 drop 页缓存。冷启动指的是进程冷启动（计算图每轮都重编），不是磁盘冷缓存。

qemu 不在这里跑。Devin 会在启动保护合入后，在 CI 里加 Sandy Bridge 和 Westmere 的冒烟。启动保护合入之前，simd 构建在只有 AVX、没有 AVX2 的 CPU 上会 SIGILL。

本机测到的数字在 `BASELINE.md`。
