"""Shared helpers for the mfile corpus, accuracy gate, and benchmarks.

The scripts shell out to the mfile CLI and to the pinned Python magika
package. They do not modify the scanner.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import time
from pathlib import Path

SCORE_TOL = 1e-4
MAGIKA_REQUIREMENT = "magika==1.0.3"
MODEL_NAME = "standard_v3_3"

# (c) is the simd binary with portable SIMD forced off. gomlx's hand-written
# AVX2/AVX-512 matmul kernels do not read GODEBUG; they follow CPUID unless
# these are set. On this host CPUID says AVX-512 is present, so (c) alone does
# not imitate a Sandy Bridge (no AVX2) kernel selection. (c*) adds the switches
# gomlx actually consults.
SIMD_OFF = {"GODEBUG": "simd=0"}
SANDY_LIKE = {
    "GODEBUG": "simd=0",
    "GOMLX_GO_SIMD_AVX2": "0",
    "GOMLX_GO_SIMD_AVX512": "0",
}


def repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def corpus_dir() -> Path:
    return repo_root() / "testdata" / "corpus"


def qa_root() -> Path:
    path = Path(os.environ.get("MFILE_QA_DIR", "/tmp/mfile-qa"))
    path.mkdir(parents=True, exist_ok=True)
    return path


def iter_samples(corpus: Path) -> list[Path]:
    files: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(corpus):
        dirnames.sort()
        for name in sorted(filenames):
            if name == "expected.tsv":
                continue
            files.append(Path(dirpath) / name)
    return files


def rel_of(corpus: Path, path: Path) -> str:
    return path.relative_to(corpus).as_posix()


def load_expected(path: Path) -> dict[str, dict[str, str]]:
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines:
        raise SystemExit(f"empty expected file: {path}")
    header = lines[0].split("\t")
    out: dict[str, dict[str, str]] = {}
    for line in lines[1:]:
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) < len(header):
            parts += [""] * (len(header) - len(parts))
        rec = dict(zip(header, parts))
        out[rec["path"]] = rec
    return out


def scores_close(a: float, b: float, tol: float = SCORE_TOL) -> bool:
    return abs(float(a) - float(b)) <= tol


def go_build_env(simd: bool) -> dict[str, str]:
    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    env["GOAMD64"] = "v1"
    if simd:
        env["GOEXPERIMENT"] = "simd"
    else:
        env.pop("GOEXPERIMENT", None)
    return env


def runtime_env(extra: dict[str, str] | None = None) -> dict[str, str]:
    """Process environment for a measurement, without leaked debug switches."""
    env = os.environ.copy()
    env.pop("GODEBUG", None)
    env.pop("GOGC", None)
    env.pop("GOMAXPROCS", None)
    env.pop("GOMEMLIMIT", None)
    for key in list(env):
        if key.startswith("GOMLX_"):
            env.pop(key)
    if extra:
        for key, value in extra.items():
            if value is None:
                env.pop(key, None)
            else:
                env[key] = value
    return env


def _newest_inputs() -> float:
    latest = 0.0
    root = repo_root()
    for rel in ("cmd", "internal", "assets"):
        base = root / rel
        if not base.exists():
            continue
        for dirpath, dirnames, filenames in os.walk(base):
            dirnames[:] = [d for d in dirnames if d != "testdata"]
            for name in filenames:
                if name.endswith((".go", ".onnx", ".json")):
                    latest = max(latest, (Path(dirpath) / name).stat().st_mtime)
    return latest


def build_mfile(dest: Path, simd: bool, rebuild: bool = False) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() and not rebuild and dest.stat().st_mtime >= _newest_inputs():
        return
    print(f"building {'simd' if simd else 'v1'} -> {dest}", flush=True)
    subprocess.check_call(
        ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(dest), "./cmd/mfile"],
        cwd=repo_root(),
        env=go_build_env(simd),
    )


def build_accuracy_test(dest: Path, simd: bool, rebuild: bool = False) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    if dest.exists() and not rebuild and dest.stat().st_mtime >= _newest_inputs():
        return
    print(f"building accuracy test ({'simd' if simd else 'v1'}) -> {dest}", flush=True)
    subprocess.check_call(
        [
            "go",
            "test",
            "-tags",
            "accuracy",
            "-c",
            "-o",
            str(dest),
            "./scripts/bench/accuracy/",
        ],
        cwd=repo_root(),
        env=go_build_env(simd),
    )


def ensure_clis(rebuild: bool = False) -> tuple[Path, Path]:
    bindir = qa_root() / "bin"
    v1 = bindir / "mfile-v1"
    simd = bindir / "mfile-simd"
    build_mfile(v1, simd=False, rebuild=rebuild)
    build_mfile(simd, simd=True, rebuild=rebuild)
    return v1, simd


def run_mfile(
    binary: Path,
    paths: list[Path],
    *,
    workers: int = 0,
    batch: int = 0,
    extra_env: dict[str, str] | None = None,
    stats: bool = False,
) -> tuple[dict[str, dict], str, int]:
    cmd = [str(binary), "-json"]
    if stats:
        cmd.insert(1, "-stats")
    if workers:
        cmd.extend(["-workers", str(workers)])
    if batch:
        cmd.extend(["-batch", str(batch)])
    cmd.extend(str(p) for p in paths)
    proc = subprocess.run(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=runtime_env(extra_env),
        check=False,
    )
    stderr = proc.stderr.decode("utf-8", "replace")
    results: dict[str, dict] = {}
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        obj = json.loads(line)
        results[obj["path"]] = {
            "label": obj.get("label") or "",
            "mime": obj.get("mime_type") or "",
            "dl": obj.get("dl_label") or "",
            "score": float(obj.get("score") or 0.0),
            "error": obj.get("error") or "",
        }
    return results, stderr, proc.returncode


_DUR_PART = re.compile(r"([0-9.]+)(ns|us|µs|μs|ms|s|m|h)")
_STATS = re.compile(
    r"stat\+open (\S+), NewScanner (\S+), ScanAll (\S+) \((\d+) files\)"
)
_RSS = re.compile(r"peak RSS ([0-9.]+) MiB")
_HEAP = re.compile(r"heap sys ([0-9.]+) MiB.*GOMAXPROCS (\d+)")


def parse_go_duration(text: str) -> float:
    """Parse Go's time.Duration.String, including compound forms like 5m1.2s."""
    scale = {
        "ns": 1e-9,
        "us": 1e-6,
        "µs": 1e-6,
        "μs": 1e-6,
        "ms": 1e-3,
        "s": 1.0,
        "m": 60.0,
        "h": 3600.0,
    }
    raw = text.strip()
    matches = list(_DUR_PART.finditer(raw))
    if not matches or "".join(match.group(0) for match in matches) != raw:
        raise ValueError(f"cannot parse Go duration {text!r}")
    return sum(float(match.group(1)) * scale[match.group(2)] for match in matches)


def parse_stats(stderr: str) -> dict[str, float | int]:
    out: dict[str, float | int] = {}
    stats = _STATS.search(stderr)
    if stats:
        out["stat_open_s"] = parse_go_duration(stats.group(1))
        out["new_scanner_s"] = parse_go_duration(stats.group(2))
        out["scan_all_s"] = parse_go_duration(stats.group(3))
        out["nfiles"] = int(stats.group(4))
    rss = _RSS.search(stderr)
    if rss:
        out["peak_rss_mib"] = float(rss.group(1))
    heap = _HEAP.search(stderr)
    if heap:
        out["heap_sys_mib"] = float(heap.group(1))
        out["gomaxprocs"] = int(heap.group(2))
    return out


def run_file(paths: list[Path]) -> dict[str, dict[str, str]]:
    args = [str(p) for p in paths]
    descr = subprocess.check_output(["file", "-b", *args], text=True).splitlines()
    mime = subprocess.check_output(
        ["file", "--mime-type", "-b", *args], text=True
    ).splitlines()
    if len(descr) != len(paths) or len(mime) != len(paths):
        raise SystemExit(
            f"file(1) returned {len(descr)} descriptions and {len(mime)} mimes for {len(paths)} paths"
        )
    return {
        str(path): {"descr": descr[i], "mime": mime[i]}
        for i, path in enumerate(paths)
    }


def magika_python(paths: list[Path]) -> dict[str, dict]:
    """Full-precision magika results from the pinned Python module."""
    script = repo_root() / "scripts" / "bench" / "magika_dump.py"
    proc = subprocess.run(
        ["python3", str(script)],
        input=json.dumps({"paths": [str(p) for p in paths]}).encode(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if proc.returncode != 0:
        raise SystemExit(
            "magika dump failed:\n" + proc.stderr.decode("utf-8", "replace")
        )
    payload = json.loads(proc.stdout)
    if payload.get("model") != MODEL_NAME:
        raise SystemExit(f"magika model is {payload.get('model')}, want {MODEL_NAME}")
    return {row["path"]: row for row in payload["results"]}, payload


def magika_cli_labels(paths: list[Path]) -> dict[str, dict]:
    proc = subprocess.run(
        ["magika", "--json", *[str(p) for p in paths]],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        env=runtime_env(),
    )
    if proc.returncode != 0:
        raise SystemExit(
            "magika CLI failed:\n" + proc.stderr.decode("utf-8", "replace")
        )
    data = json.loads(proc.stdout)
    out: dict[str, dict] = {}
    for item in data:
        value = item["result"]["value"]
        out[item["path"]] = {
            "label": value["output"]["label"],
            "mime": value["output"].get("mime_type") or "",
            "dl": value["dl"]["label"],
            "score": float(value["score"]),
        }
    return out


def norm_dl(label: str) -> str:
    """mfile omits dl_label when the model did not run; magika says undefined."""
    if label in ("", "undefined"):
        return ""
    return label


def timed_run(
    args: list[str],
    *,
    extra_env: dict[str, str] | None = None,
    stdout=subprocess.DEVNULL,
) -> dict:
    env = runtime_env(extra_env)
    start = time.perf_counter()
    proc = subprocess.Popen(
        args,
        stdout=stdout,
        stderr=subprocess.PIPE,
        env=env,
    )
    _, status, usage = os.wait4(proc.pid, 0)
    elapsed = time.perf_counter() - start
    stderr = proc.stderr.read().decode("utf-8", "replace") if proc.stderr else ""
    return {
        "seconds": elapsed,
        "returncode": os.waitstatus_to_exitcode(status),
        "rss_mib": usage.ru_maxrss / 1024.0,
        "stderr": stderr,
    }


def environment() -> dict[str, str]:
    def capture(args: list[str]) -> str:
        try:
            return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()
        except (OSError, subprocess.CalledProcessError) as exc:
            return f"(unavailable: {exc})"

    cpu = ""
    flags = ""
    if Path("/proc/cpuinfo").exists():
        for line in Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name") and not cpu:
                cpu = line.split(":", 1)[1].strip()
            elif line.startswith("flags") and not flags:
                flags = line.split(":", 1)[1].strip()
    interesting = [
        flag
        for flag in flags.split()
        if flag.startswith(("avx", "sse", "amx", "fma")) or flag in {"aes", "popcnt"}
    ]
    lscpu = capture(["lscpu"])
    family = model = stepping = ""
    for line in lscpu.splitlines():
        if line.startswith("CPU family:"):
            family = line.split(":", 1)[1].strip()
        elif line.startswith("Model:"):
            model = line.split(":", 1)[1].strip()
        elif line.startswith("Stepping:"):
            stepping = line.split(":", 1)[1].strip()
        elif line.startswith("CPU(s):") and "On-line" not in line and not family:
            pass
    cores = str(os.cpu_count() or "")
    return {
        "cpu": cpu,
        "cpu_family": family,
        "cpu_model": model,
        "cpu_stepping": stepping,
        "cores": cores,
        "simd_flags": " ".join(interesting),
        "os": capture(["uname", "-srvm"]),
        "go": capture(["go", "version"]),
        "file": capture(["file", "--version"]).splitlines()[0],
        "magika_cli": capture(["magika", "--version"]),
        "python": capture(["python3", "--version"]),
    }


def file_version_line() -> str:
    text = subprocess.check_output(["file", "--version"], text=True, stderr=subprocess.STDOUT)
    return text.splitlines()[0]


def pct(n: int, d: int) -> str:
    if d == 0:
        return "n/a"
    return f"{100.0 * n / d:.1f}% ({n}/{d})"
