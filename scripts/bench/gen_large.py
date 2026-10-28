#!/usr/bin/env python3
"""Generate a throwaway performance corpus. Nothing here is committed.

Default output is /tmp/mfile-bench-large: thousands of small files plus a
couple of multi-megabyte files. Re-running with the same arguments replaces
the directory.
"""

from __future__ import annotations

import argparse
import shutil
from pathlib import Path

SMALL_TEMPLATES = [
    "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println({n})\n}\n",
    "def f(n):\n    return n * {n} + 1\n\nif __name__ == '__main__':\n    print(f({n}))\n",
    '{{"id": {n}, "ok": true, "name": "sample-{n}"}}\n',
    "#include <stdio.h>\nint main(void) {{ printf(\"%d\\n\", {n}); }}\n",
    "#!/bin/sh\necho sample-{n}\nexit 0\n",
    "<html><body><p>row {n}</p></body></html>\n",
    "key = {n}\nname = \"sample\"\nenabled = true\n",
    "SELECT id, name FROM items WHERE id = {n};\n",
]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=Path("/tmp/mfile-bench-large"))
    parser.add_argument("--small", type=int, default=12000, help="number of small files")
    parser.add_argument("--big-mib", type=int, default=32)
    parser.add_argument("--big-count", type=int, default=2)
    args = parser.parse_args()
    out: Path = args.out
    if out.exists():
        shutil.rmtree(out)
    small = out / "s"
    small.mkdir(parents=True)
    for i in range(args.small):
        body = SMALL_TEMPLATES[i % len(SMALL_TEMPLATES)].format(n=i)
        # A few longer files so the mix is not only tiny sources.
        if i % 50 == 0:
            body = body * 40
        (small / f"{i:05d}").write_text(body, encoding="utf-8")
    big = out / "big"
    big.mkdir()
    chunk = b"mfile-qa-large-file\n" * 64
    for i in range(args.big_count):
        path = big / f"blob{i}.bin"
        remaining = args.big_mib * 1024 * 1024
        with path.open("wb") as handle:
            if i == 0:
                handle.write(b"\0" * remaining)
                continue
            while remaining:
                take = chunk if remaining >= len(chunk) else chunk[:remaining]
                handle.write(take)
                remaining -= len(take)
    total = sum(p.stat().st_size for p in out.rglob("*") if p.is_file())
    print(f"wrote {args.small} small files and {args.big_count} x {args.big_mib} MiB under {out}")
    print(f"total bytes {total}")


if __name__ == "__main__":
    main()
