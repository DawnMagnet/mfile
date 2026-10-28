# 基线（master，本机实测）

测的是本仓库当时的 master 内容（分支 `cursor/qa-corpus-bench-47bf`，语料和脚本一起提交）。数字来自一次完整的 `python3 scripts/bench/perf` 和 `python3 scripts/bench/equiv`，准确率来自 `python3 scripts/bench/compare`。

## 环境

| 项 | 值 |
|---|---|
| CPU | `Intel(R) Xeon(R) Processor`（虚拟机不暴露商品名）。CPUID family 6，model 207（0xCF），stepping 2。4 核，1 线程/核 |
| 指令集 | SSE4.2、AVX、AVX2、AVX-512（F/DQ/CD/BW/VL/VBMI/VBMI2/VNNI/BF16/FP16 等）、AMX（tile / bf16 / int8） |
| OS | Linux 6.12.94+，x86_64 |
| Go | go1.27.3 linux/amd64 |
| file | file-5.45 |
| magika CLI | 1.1.0，模型 `standard_v3_3`（Python 包 `magika==1.0.3` 自带的二进制） |
| magika Python | 1.0.3，`Magika.get_model_name() == standard_v3_3`。score 对比用这个 API。CLI `--json` 的 score 只保留约 3 位小数，不能用来卡 1e-4 |
| gcc | Ubuntu 13.3.0（只用于生成语料里的 ELF） |
| 页缓存 | 没有 drop。冷启动指每次新进程都重新编译计算图 |

Rusty 在 AVX-512 桌面上的参考值（不是本机）：单文件普通构建约 0.125 s，simd 约 0.03 s，峰值 RSS 约 39 MiB，体积 16.3 MiB 和 19.7 MiB。本机体积和单文件 RSS 对得上，冷启动中位数也接近。

## 构建

| | 怎么编 | 怎么跑 |
|---|---|---|
| (a) v1 | `CGO_ENABLED=0 GOAMD64=v1` | 默认 |
| (b) simd | `CGO_ENABLED=0 GOEXPERIMENT=simd GOAMD64=v1` | 默认 |
| (c) simd_off | 与 (b) 同一个二进制 | `GODEBUG=simd=0` |
| (d) nosimd | 与 (b) 同一个二进制 | `GODEBUG=simd=0,cpu.avx2=off,cpu.avx512f=off,cpu.fma=off`，再加上 `GOMLX_GO_SIMD_AVX2=0` 和 `GOMLX_GO_SIMD_AVX512=0` |
| sandy_like（补充） | 与 (b) 同一个二进制 | `GODEBUG=simd=0 GOMLX_GO_SIMD_AVX2=0 GOMLX_GO_SIMD_AVX512=0` |

(d) 是第 2 项启动保护在 Sandy Bridge 上重新 exec 之后实际会走的路径，这次在本机原生跑。(c) 只关了 Go `simd` 包的硬件向量，gomlx 的 AVX matmul 核仍按 CPUID 开着，所以分数和 (b) 一样、速度介于 (a) 和 (b) 之间。(d) 再把 `cpu.avx2` / `cpu.avx512f` / `cpu.fma` 和 gomlx 的两个 AVX 开关关掉。

启动保护（第 2 项）还没进 master。合入之前，simd 构建在只有 AVX、没有 AVX2 的 CPU 上会 SIGILL。这次没有跑 qemu。

## 体积和单文件

| | 体积 | 冷启动均值 | 冷启动中位数 | 单文件峰值 RSS |
|---|---:|---:|---:|---:|
| (a) v1 | 16.30 MiB（17088672 B） | 126.6 ms（标准差 11.5） | 121.9 ms | 38.8 MiB |
| (b) simd | 19.72 MiB（20676768 B） | 26.3 ms（标准差 2.6） | 26.0 ms | 39.8 MiB |
| (c) simd_off | 同 (b) | 36.3 ms（标准差 1.8） | 35.7 ms | 40.0 MiB |
| (d) nosimd | 同 (b) | 141.6 ms（标准差 1.4） | 141.6 ms | 39.9 MiB |
| sandy_like | 同 (b) | 144.2 ms（标准差 1.8） | 144.1 ms | 39.8 MiB |

冷启动是 hyperfine，warmup 1，10 次，样本是 `testdata/corpus/code/hello.go`。每次都是新进程。

启动拆分用 `-stats`，5 次取中位数。`NewScanner` 是解析模型和建执行器。第一次 `ScanAll` 含计算图编译和一次推理。用 1 个文件和 32 个相同文件拆开：`infer = (ScanAll(32) - ScanAll(1)) / 31`，`compile = ScanAll(1) - infer`。

| | NewScanner | 计算图编译 | 一次推理 | ScanAll(1) | 编译占 ScanAll(1) |
|---|---:|---:|---:|---:|---:|
| (a) v1 | 5.7 ms | 83.4 ms | 32.8 ms | 116.2 ms | 72% |
| (b) simd | 5.2 ms | 15.3 ms | 2.0 ms | 17.3 ms | 88% |
| (c) simd_off | 5.0 ms | 23.1 ms | 5.1 ms | 28.2 ms | 82% |
| (d) nosimd | 5.4 ms | 95.1 ms | 38.3 ms | 133.4 ms | 71% |
| sandy_like | 5.6 ms | 105.8 ms | 28.8 ms | 134.6 ms | 79% |

单文件时间里，模型加载只有大约 5 ms。大头是第一次推理时编译计算图，不是把权重读进来。

32 MiB 文件的 `ScanAll` 和单文件同一量级（v1 约 112–118 ms，simd 约 25–27 ms，峰值 RSS 仍约 39–40 MiB）。大文件只读头尾，不拉高内存。

## 目录吞吐

12000 个小文件，一次进程，默认 workers（本机 GOMAXPROCS = 4）和默认 batch 1。files/s 用墙钟，含启动。

| | 墙钟 | files/s | ScanAll | 峰值 RSS（-stats / ru_maxrss） |
|---|---:|---:|---:|---:|
| (a) v1 | 301.05 s | 39.9 | 300.75 s（39.9 files/s） | 336.7 / 336.5 MiB |
| (b) simd | 23.49 s | 510.8 | 23.24 s（516.3 files/s） | 366.2 / 365.7 MiB |
| (c) simd_off | 61.41 s | 195.4 | 61.16 s（196.2 files/s） | 376.1 / 375.7 MiB |
| (d) nosimd | 374.83 s | 32.0 | 374.56 s（32.0 files/s） | 332.5 / 332.2 MiB |
| sandy_like | 376.72 s | 31.9 | 376.42 s（31.9 files/s） | 331.6 / 331.2 MiB |
| file -b | 2.74 s | 4374 | | ru_maxrss 25.7 MiB |
| magika --json | 14.45 s | 830.5 | | ru_maxrss 82.7 MiB |

同一列表上，单文件 `file` 5.6 ms，单文件 `magika --json` 77.9 ms。目录场景里 simd 构建比 magika 慢（511 对 830 files/s），但单文件冷启动更快（26 ms 对 78 ms）。v1 构建的目录吞吐大约是 simd 的 1/13。

### workers 和 batch

2000 个小文件。batch 16 几乎不提高吞吐，峰值 RSS 升到大约 0.5–1.0 GiB。4 个核上 workers 从 4 加到 8 没有收益。

| mode | workers | batch | 墙钟 s | files/s | 峰值 RSS MiB |
|---|---:|---:|---:|---:|---:|
| v1 | 1 | 1 | 198.65 | 10.1 | 121.6 |
| v1 | 1 | 4 | 199.08 | 10.0 | 184.1 |
| v1 | 1 | 16 | 202.67 | 9.9 | 464.8 |
| v1 | 4 | 1 | 50.51 | 39.6 | 157.1 |
| v1 | 4 | 4 | 51.09 | 39.1 | 266.1 |
| v1 | 4 | 16 | 56.57 | 35.4 | 700.5 |
| v1 | 8 | 1 | 50.66 | 39.5 | 196.1 |
| v1 | 8 | 4 | 51.42 | 38.9 | 333.8 |
| v1 | 8 | 16 | 60.42 | 33.1 | 1060.4 |
| simd | 1 | 1 | 12.91 | 154.9 | 144.6 |
| simd | 1 | 4 | 14.46 | 138.3 | 191.0 |
| simd | 1 | 16 | 16.18 | 123.6 | 452.4 |
| simd | 4 | 1 | 4.16 | 480.8 | 179.4 |
| simd | 4 | 4 | 4.55 | 439.3 | 247.5 |
| simd | 4 | 16 | 5.45 | 366.8 | 690.0 |
| simd | 8 | 1 | 3.65 | 548.2 | 206.2 |
| simd | 8 | 4 | 4.47 | 447.9 | 354.4 |
| simd | 8 | 16 | 5.38 | 372.1 | 1031.5 |
| simd_off | 1 | 1 | 37.73 | 53.0 | 144.9 |
| simd_off | 1 | 4 | 38.08 | 52.5 | 195.9 |
| simd_off | 1 | 16 | 40.09 | 49.9 | 478.8 |
| simd_off | 4 | 1 | 10.30 | 194.2 | 169.5 |
| simd_off | 4 | 4 | 10.32 | 193.8 | 239.2 |
| simd_off | 4 | 16 | 10.85 | 184.3 | 691.6 |
| simd_off | 8 | 1 | 10.17 | 196.8 | 202.9 |
| simd_off | 8 | 4 | 10.18 | 196.4 | 349.6 |
| simd_off | 8 | 16 | 11.00 | 181.8 | 1009.9 |

## 准确率

小语料 80 个文件，对照 `testdata/corpus/expected.tsv`。三种 mfile 的输出 label 完全一致。(a)(b)(c) 对期望的准确率相同。magika 的输出 label 和 mfile 也完全一致（80/80），所以下面这些误判是模型（或 KB）的，不是移植引进的。

| 工具 | label | MIME |
|---|---:|---:|
| mfile (a)(b)(c) | 95.0%（76/80） | 92.5%（74/80） |
| magika 1.0.3 / standard_v3_3 | 95.0%（76/80） | 92.5%（74/80） |
| file 5.45 | 70.0%（56/80） | 60.0%（48/80） |

file 的 label 用 `file -b` 是否包含 `file_substr`。它把不少源码和配置说成 `ASCII text` / `text/plain`，把这份 JAR 说成 Zip，把 Go 说成 C source。MIME 词表也和 Magika KB 不一致。

`known_fail` 只有 `byname/go.mod`，仍算在 76/80 的分母里。

### mfile 误判

判断只是建议。三种构建的 label 相同，分数差在 1e-3 以内。magika 的 label 和 dl 与 mfile 相同。

| 文件 | 期望 | mfile label | score | dl | 判断 |
|---|---|---|---:|---|---|
| `byname/go.mod` | txt | verilog | 0.745396 | verilog | 归第 5 项文件名层。模型 dl 是 verilog（0.745，高于 0.5），词表里没有 gomod。已知失败，`known_fail=1`，样本保留 |
| `byname/Cargo.lock` | toml | txt | 0.999585 | txt | 归第 5 项文件名层。内容是 TOML，dl 以 0.999 判成 txt |
| `byname/PKGBUILD` | shell | txt | 0.498922 | r | 归第 5 项文件名层。dl 是 r（0.499），按 r 的阈值 0.9 回退成 txt |
| `edge/utf16be.txt` | txt | powershell | 0.913652 | powershell | 归第 5 项 textenc：在模型之前先认 BOM / UTF-16。现在 BE 被模型高置信度判成 powershell；LE 的 dl 是 randomtxt，被 `overwrite_map` 收成 txt |

另外两处 label 是对的，MIME 是空的，因为 KB 里 `mime_type` 为 null：

| 文件 | label | mfile MIME | 期望 MIME | magika MIME |
|---|---|---|---|---|
| `binary/sample.sqlite` | sqlite | （空） | application/vnd.sqlite3 | application/octet-stream |
| `byname/go.sum` | sum | （空） | text/plain | text/plain |

这是 KB 缺口，不是模型判错。TSV 的 label 两边都对；mfile 用仓库 KB 的 `text/tab-separated-values`，magika 1.0.3 的 KB 是 `text/tsv`。

输出 label 正确、但 dl 不是这个 label 的样本（不算误判，记在这里免得以后误读）：

| 文件 | 输出 | dl | score | 原因 |
|---|---|---|---:|---|
| `byname/go.work` | txt | asm | 0.388 | 低于 0.5，文本类型回退成 txt |
| `edge/long_line.txt` | txt | autoit | 0.450 | 同上。这也是和 magika 分差最大的文件 |
| `edge/utf16le.txt` | txt | randomtxt | 0.457 | `overwrite_map` 把 randomtxt 收成 txt |
| `edge/random.bin` | unknown | randombytes | 0.984 | `overwrite_map` 把 randombytes 收成 unknown |

## 等价性

门槛常量在 `scripts/bench/gate.json` 的 `score_tol`，现在是 **5e-4**。`scripts/bench/equiv` 在下面三条都成立时退出码为 0，否则为 1：

1. 输出 label 和 dl label 完全一致（模型没跑时，mfile 的空 dl 和 magika 的 `undefined` 算相同）。
2. score 绝对误差 ≤ 5e-4。
3. score 与判定阈值的距离 < 5e-4 的样本，阈值回退之后的最终 label 和 MIME 必须与 magika 完全一致。判定阈值按 magika 默认的 `high_confidence`：该 dl label 在 config 的 `thresholds` 里就用它，否则用 `medium_confidence_threshold`（0.5）。配置里没有单独的默认 high 阈值，per-label 表就是那一组。

**这次重跑通过。** 全部门禁对比的最大绝对误差是 **3.991e-4**（(d) 对 magika，`edge/utf16le.txt`）。接近阈值的样本 **0 个**。离阈值最近的是 `byname/flake.lock`：dl `json`，score 0.5144，阈值 0.5，距离 1.44e-2。

| 对比 | 最大 \|score\| | 超过 5e-4 |
|---|---:|---|
| (a) 对 magika | 3.428e-4 | 无 |
| (b) 对 magika | 2.965e-4 | 无 |
| (c) 对 magika | 2.965e-4 | 无 |
| (d) 对 magika | 3.991e-4 | 无 |
| (b) 对 (a) | 2.936e-4 | 无 |
| (c) 对 (a) | 2.936e-4 | 无 |
| (c) 对 (b) | 0 | 无 |
| (d) 对 (a) | 3.936e-4 | 无 |
| (d) 对 (b) | 3.987e-4 | 无 |
| onnxruntime(原始模型) 对 onnxruntime(patched)，74 个跑了模型的样本 | 9.54e-7，argmax 全相同 | 无 |

(d) 的分数和 (b) 不同：80 个里有 61 个 score 不一样，最大 3.987e-4（`edge/utf16le.txt`）。吞吐是 32.0 files/s，接近 (a) 的 39.9，而不是 (b) 的 510.8；冷启动 141.6 ms，也接近 (a) 的 121.9 ms，不是 (b) 的 26.0 ms。这组开关是生效的，(d) 没有留在 AVX2/AVX-512 核上。

label 上的差仍然不是 `patch_model.py`：改写在 onnxruntime 里小于 1e-6。剩下的是 Go 后端 float。

### 多轮和 GC

每个进程对整份语料做 5 轮 workers=1、batch=1，再加 workers/batch 为 (2,1)、(4,1)、(4,4)、(1,8)。(a)(b)(c) 各有一轮 `GOGC=1`。

- 所有轮次、所有构建、`GOGC=1` 与默认 GOGC，输出 label 和 dl label 都没有变化。
- 同一配置重复 5 轮，以及 `GOGC=1` 对默认 GOGC 的第一轮，score 的 float32 比特相同。
- batch 1 配 workers 1、2、4 也是比特相同。
- batch 4 和 batch 8 相对 batch 1 有 ulp 级差别。v1 上最大 2.3e-6（`config/app.ini`），simd 上最大 3.0e-7（`byname/PKGBUILD`）。都远小于 5e-4，而且同一配置重复是稳定的，不是一轮一个数。

这次压力测试没有看到 `unsafePackRHS` 把结果悄悄改掉。用 `-gcflags=all=-d=checkptr` 编出来的 v1 二进制，在 `GODEBUG=checkptr=2` 下扫一个 Go 源文件会直接死掉，不是返回另一套 label：

```text
fatal error: checkptr: pointer arithmetic result points to invalid allocation
github.com/gomlx/compute/internal/gobackend/dot/matmul.unsafePackRHS
    packing.go:82
```

默认构建没有这条检测，所以「checkptr 能抓住的指针运算」在普通二进制里仍可能存在。本语料上它没有表现为跨轮或跨 GC 的结果漂移。

`scripts/bench/equiv` 按上面三条、`score_tol = 5e-4` 重跑，退出码为 0。checkptr 构建仍然会在 `packing.go:82` fatal，那一项不计入退出码。
