#!/usr/bin/env python3
"""Dump full-precision magika results as JSON. Stdin: {"paths": [...]}."""

from __future__ import annotations

import json
import sys

from magika import Magika


def main() -> int:
    payload = json.load(sys.stdin)
    magika = Magika()
    model = magika.get_model_name()
    if model != "standard_v3_3":
        print(f"refusing model {model!r}; want standard_v3_3", file=sys.stderr)
        return 2
    rows = []
    for path in payload["paths"]:
        result = magika.identify_path(path)
        if not result.ok:
            rows.append({"path": path, "error": str(result.status)})
            continue
        rows.append(
            {
                "path": path,
                "label": str(result.output.label),
                "mime": result.output.mime_type or "",
                "dl": str(result.dl.label),
                "score": float(result.score),
                "group": result.output.group or "",
            }
        )
    json.dump(
        {
            "model": model,
            "module": magika.get_module_version(),
            "results": rows,
        },
        sys.stdout,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
