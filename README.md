# magika-go-poc

A static, cgo-free `file` replacement that identifies content types with
Google's [Magika](https://github.com/google/magika) `standard_v3_3` model and
prints basic metadata (size, mode, mtime, symlink target).

Inference runs on the pure Go gomlx backend (`github.com/gomlx/compute/gobackend`)
through `github.com/gomlx/onnx-gomlx`. No ONNX Runtime, no cgo; the model,
config and content types KB are embedded with `go:embed`.

## Build

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/mfile ./cmd/mfile
```

For about 3x faster inference on amd64, enable the backend's AVX2/AVX-512
matmul kernels (still cgo-free and static):

```bash
CGO_ENABLED=0 GOEXPERIMENT=simd go build -trimpath -ldflags='-s -w' -o bin/mfile ./cmd/mfile
```

## Usage

```bash
mfile [-json] [-L] [-E] [-workers N] [-batch N] [-stats] FILE...
```

`-stats` prints phase timings, Go heap counters and the process peak RSS
(PeakWorkingSetSize on Windows, VmHWM on Linux) to stderr. `-cpuprofile` and
`-memprofile` write pprof profiles.

A path that cannot be opened or stat'd is printed on stdout in the same
form as `file` and the process exits 0, after the other paths are still
scanned:

```text
path: cannot open `path' (No such file or directory)
```

`-E` prints that line on stderr and exits 1. Usage and argument errors
exit 2.

`MFILE_CPU_GUARD` is an internal variable. Setting it manually is for
debugging only and will crash with SIGILL on affected CPUs (AVX without
AVX2, or AVX2 without FMA).

## Model patch

The embedded model is `assets/standard_v3_3/model.patched.onnx`, an
equivalent rewrite of the original `model.onnx`:

- `GlobalMaxPool` becomes `ReduceMax(axes=[2], keepdims=1)`; onnx-gomlx
  v0.5.15 does not implement `GlobalMaxPool`.
- The one-hot encoding followed by `MatMul` becomes a `Gather` of the
  embedding rows.
- The 1D `Conv` (kernel 5, 256 -> 512 channels) becomes im2col
  (`Slice` + `Concat`) followed by `MatMul`. The Go backend's generic conv
  took ~90% of the inference time; its matmul is much faster.

Regenerate and verify it against the original with onnxruntime:

```bash
uv run --no-project --with onnx --with onnxruntime --with numpy python -I \
  tools/patch_model.py --check \
  assets/standard_v3_3/model.onnx assets/standard_v3_3/model.patched.onnx
```

## Performance

Each inference call scans one file, and calls run concurrently on GOMAXPROCS
goroutines. Within one call the Go backend parallelizes only the big matmuls;
the slice, transpose and elementwise ops between them left most cores idle
(~3 of 16 busy) when calls ran one after another.

Peak memory is dominated by the backend's buffer pool (a `sync.Pool` per
buffer size, cached per P), mainly matmul packing and im2col buffers. It
scales with the batch size and with GOMAXPROCS, not with the worker count.
Batch 1 was both the fastest and the smallest setting. In the
`GOEXPERIMENT=simd` build, mfile caps GOMAXPROCS at 8 (unless the
`GOMAXPROCS` environment variable is set): throughput is flat beyond that,
and memory is not. The default build is compute bound and keeps all cores.
Lowering GOGC or setting GOMEMLIMIT made memory worse, because every GC
empties the pool and the next calls allocate fresh buffers.

Measured on an i5-12600KF (16 logical CPUs), Windows 11, Go 1.27.3. The
corpus is 3200 files in 16 invocations of 200; peak RSS is the median over
the invocations:

| | 1 file | corpus time | corpus peak RSS |
|---|---|---|---|
| GlobalMaxPool patch only, fixed batch 16 | | 181 s | |
| simd, sequential batches of 16 | 0.12 s | 25 s | |
| simd, batches of 4, 8 workers, 16 Ps | 0.11 s | 12.2 s | ~440 MiB |
| default build, batch 1, 16 workers, 16 Ps | 0.16 s | 42.8 s | 194 MiB |
| simd, batch 1, 8 workers, 8 Ps (current) | 0.14 s | 11.2 s | 133 MiB |
| official `magika` 1.1.0 CLI | 0.29 s | 12.8–13.4 s | 129 MiB |

A single file peaks at about 54 MiB.

## Labeling rules

Same as the reference implementation: empty files are `empty`; files with
fewer than `min_file_size_for_dl` bytes after stripping whitespace are `txt`
(valid UTF-8) or `unknown`; otherwise the model label is mapped through
`overwrite_map`, and predictions below the per-label threshold fall back to
`txt` (text types) or `unknown`.
