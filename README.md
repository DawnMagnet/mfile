# mfile

mfile is a modern, fast replacement for `file`. It is written in pure Go, with no cgo, and is distributed as a single static binary with an embedded deep-learning content-type model. It identifies a file's content type and prints basic metadata: size, mode, modification time, and, for a symlink, the target.

## Install

Download a release from [GitHub Releases](https://github.com/DawnMagnet/mfile/releases).

Archive names:

- `mfile_<ver>_<os>_<arch>.tar.gz` for Linux and macOS
- `mfile_<ver>_windows_amd64.zip` for Windows

`<ver>` is the git tag with the leading `v` removed, so tag `v0.1.0` publishes `mfile_0.1.0_linux_amd64.tar.gz`. The release workflow builds `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`. Each archive contains the `mfile` binary (`mfile.exe` on Windows), `LICENSE`, `NOTICE`, and `README.md`.

Every release also uploads `SHA256SUMS` (ordinary `sha256sum` output). From a directory that contains the archives you downloaded:

```bash
sha256sum -c SHA256SUMS
tar -xzf mfile_<ver>_linux_amd64.tar.gz
```

On Windows, check the same `SHA256SUMS` file and unzip `mfile_<ver>_windows_amd64.zip`.

Arch Linux: `mfile-bin` on the AUR (coming).

## Build from source

```bash
go install github.com/DawnMagnet/mfile/cmd/mfile@latest
```

A static binary from a checkout of this repository, the same shape as the release archives:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o mfile ./cmd/mfile
```

On amd64, `GOEXPERIMENT=simd` turns on the backend's AVX2 and AVX-512 matmul kernels. The binary is still cgo-free and static, and inference is much faster (see Performance):

```bash
CGO_ENABLED=0 GOEXPERIMENT=simd go build -trimpath -ldflags='-s -w' -o mfile ./cmd/mfile
```

A release build sets the version with `-X main.version=<ver>`. `mfile --version` then prints `mfile <ver>` on the first line.

### Model patch

The embedded model is `assets/standard_v3_3/model.patched.onnx`, an equivalent rewrite of the original `model.onnx`:

- `GlobalMaxPool` becomes `ReduceMax(axes=[2], keepdims=1)`; onnx-gomlx v0.5.15 does not implement `GlobalMaxPool`.
- The one-hot encoding followed by `MatMul` becomes a `Gather` of the embedding rows.
- The 1D `Conv` (kernel 5, 256 -> 512 channels) becomes im2col (`Slice` + `Concat`) followed by `MatMul`. The Go backend's generic conv took ~90% of the inference time; its matmul is much faster.

Regenerate and verify it against the original with onnxruntime:

```bash
uv run --no-project --with onnx --with onnxruntime --with numpy python -I \
  tools/patch_model.py --check \
  assets/standard_v3_3/model.onnx assets/standard_v3_3/model.patched.onnx
```

## Usage

`mfile --help` prints:

```text
usage: mfile [flags] FILE...
      --batch int          max files per inference call, a power of two (0: 1)
      --cpuprofile file    write a CPU profile to file
  -L, --dereference        follow symlinks
  -E, --exit-on-fs-error   On filesystem errors (file not found etc), instead of handling the error as regular output as POSIX mandates and keep going, issue an error message and exit
      --format string      output format: plain, json, or pretty
      --json               print one JSON object per line
      --memprofile file    write a heap profile to file at exit
      --stats              print timings and memory use to stderr
      --version            print version, Go version, GOAMD64 level, and SIMD path
      --workers int        concurrent inference calls (0: GOMAXPROCS)

MFILE_CPU_GUARD is an internal variable. Setting it manually is for debugging only and will crash with SIGILL on affected CPUs (AVX without AVX2, or AVX2 without FMA).
```

The long options also accept a single dash (`-json`, `-format pretty`, `-workers 4`, `-batch 1`, `-stats`, `-version`). `-L` and `-E` are the short names shown above. With no paths, mfile prints that usage to stderr and exits 2.

An untagged build's `--version` looks like this (`dev` when the version was not stamped; a release prints the tag without the leading `v`):

```text
$ mfile --version
mfile dev
go1.27.3
GOAMD64=v1
simd=software
cpu_guard=false
```

Plain output is one line per path. These lines are from one run; `mode` and `mtime` are the files on disk. `score` is the model confidence, or `1.000` when a rule decided the label (an empty file, below).

```text
$ mfile hello.go app.yaml pixel.png empty plain.txt
hello.go: Golang source [go, text/x-golang, code] score=1.000 size=223B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
app.yaml: YAML source [yaml, application/x-yaml, code] score=1.000 size=108B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
pixel.png: PNG image [png, image/png, image] score=1.000 size=69B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
empty: Empty file [empty, inode/x-empty, inode] score=1.000 size=0B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
plain.txt: Generic text document [txt, text/plain, text] score=0.978 size=170B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
```

`--json` (or `--format json`) prints one JSON object per line:

```text
$ mfile --json hello.go
{"path":"hello.go","kind":"file","size":223,"mode":"-rw-r--r--","mtime":"2026-10-29T10:18:09.672335567Z","label":"go","mime_type":"text/x-golang","group":"code","description":"Golang source","extensions":["go"],"is_text":true,"score":0.9997906,"dl_label":"go"}
```

`--format pretty` aligns a description, group, score, size, decision source, label, and MIME. On a pipe the full layout is kept and color is off:

```text
$ mfile --format pretty hello.go app.yaml
hello.go                      Golang source                 code          score=1.000   size=223B       model      go                text/x-golang
app.yaml                      YAML source                   code          score=1.000   size=108B       model      yaml              application/x-yaml
```

A directory is reported as a directory. mfile does not walk it:

```text
$ mfile code
code: directory mode=drwxr-xr-x mtime=2026-10-29 10:18:09
```

Without `-L`, a symlink is reported as a symlink. `-L` / `--dereference` stats and classifies the target:

```text
$ mfile link-hello.go
link-hello.go: symbolic link to hello.go
$ mfile -L link-hello.go
link-hello.go: Golang source [go, text/x-golang, code] score=1.000 size=223B mode=-rw-r--r-- mtime=2026-10-29 10:18:09
```

A path that cannot be opened is printed on stdout in the same form as `file`, and the process exits 0. Other paths are still scanned:

```text
$ mfile missing-file
missing-file: cannot open `missing-file' (No such file or directory)
```

`-E` / `--exit-on-fs-error` prints that line on stderr and exits 1.

`--stats` writes phase timings and process memory to stderr. One local run:

```text
stat+open 10.857µs, NewScanner 5.018485ms, ScanAll 112.46458ms (1 files)
heap sys 31.6 MiB, total alloc 23.5 MiB, 6 GCs, GOMAXPROCS 4
peak RSS 39.3 MiB
```

`--workers` is the number of concurrent inference calls (`0` means `GOMAXPROCS`). `--batch` is the maximum files per inference call and must be a power of two (`0` means 1). `--cpuprofile` and `--memprofile` write pprof profiles.

`MFILE_CPU_GUARD` is an internal variable. Setting it manually is for debugging only and will crash with SIGILL on affected CPUs (AVX without AVX2, or AVX2 without FMA).

## Performance

The figures below are earlier measurements, kept as recorded. They compare mfile with `file(1)` and with the upstream reference implementation (model `standard_v3_3`).

### mfile vs file(1) vs the upstream reference

Linux VM, 4 cores, Go 1.27.3, `file` 5.45. Directory throughput is 12,000 small files in one process; the wall clock includes process startup. Cold start is a new process on `testdata/corpus/code/hello.go`. mfile here is the `GOEXPERIMENT=simd` build. The reference row is that implementation's CLI in `--json` mode.

| | cold start, one file | 12,000 files | files/s | peak RSS |
|---|---:|---:|---:|---:|
| mfile (`GOEXPERIMENT=simd`) | 26.0 ms median | 23.49 s | 510.8 | 366.2 MiB |
| file(1) (`file -b`) | 5.6 ms | 2.74 s | 4374 | 25.7 MiB (`ru_maxrss`) |
| `magika --json` 1.1.0 CLI | 77.9 ms | 14.45 s | 830.5 | 82.7 MiB (`ru_maxrss`) |

On that directory, simd mfile is slower than the reference CLI (511 vs 830 files/s) and faster to cold-start (26 ms vs 78 ms). `file(1)` is faster and smaller on this corpus because it does not run a neural net. The default (non-simd) mfile build did the same directory in 301.05 s (39.9 files/s).

Accuracy on the 80-file corpus in `testdata/corpus`, scored against `testdata/corpus/expected.tsv`. mfile's labels matched the upstream reference on all 80 files, so the misses are the model's (or the knowledge base's), not a difference in this program. `file(1)` label accuracy is whether `file -b` contains the corpus `file_substr`.

| | label | MIME |
|---|---:|---:|
| mfile | 95.0% (76/80) | 92.5% (74/80) |
| `magika` 1.0.3 / standard_v3_3 | 95.0% (76/80) | 92.5% (74/80) |
| file 5.45 | 70.0% (56/80) | 60.0% (48/80) |

`file(1)` calls a lot of source and config `ASCII text` or `text/plain`.

### Windows timings

i5-12600KF (16 logical CPUs), Windows 11, Go 1.27.3. The corpus is 3200 files in 16 invocations of 200. Peak RSS is the median over those invocations. `file(1)` was not part of this run. The last row is the upstream reference CLI.

| | 1 file | corpus time | corpus peak RSS |
|---|---|---|---|
| GlobalMaxPool patch only, fixed batch 16 | | 181 s | |
| simd, sequential batches of 16 | 0.12 s | 25 s | |
| simd, batches of 4, 8 workers, 16 Ps | 0.11 s | 12.2 s | ~440 MiB |
| default build, batch 1, 16 workers, 16 Ps | 0.16 s | 42.8 s | 194 MiB |
| simd, batch 1, 8 workers, 8 Ps (current) | 0.14 s | 11.2 s | 133 MiB |
| `magika` 1.1.0 CLI | 0.29 s | 12.8–13.4 s | 129 MiB |

A single file peaks at about 54 MiB.

Each inference call scans one file, and calls run concurrently. Within one call the Go backend parallelizes the big matmuls; the slice, transpose, and elementwise ops between them left most cores idle when calls ran one after another. Peak memory is dominated by the backend's buffer pool and scales with the batch size and with `GOMAXPROCS`, not with the worker count. Batch 1 was both the fastest and the smallest setting. In the `GOEXPERIMENT=simd` build, mfile caps `GOMAXPROCS` at 8 unless the `GOMAXPROCS` environment variable is set.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

## Credits

The embedded model, config, and content-types knowledge base come from Google's [Magika](https://github.com/google/magika) project (Apache-2.0).
