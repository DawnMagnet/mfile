#!/usr/bin/env python3
# Small original Python sample.

def answer(n: int) -> int:
    return n * 2


def main() -> None:
    values = [1, 2, 3]
    print(sum(answer(v) for v in values))


if __name__ == "__main__":
    main()
