#!/usr/bin/env python3
"""Generate testdata/corpus and expected.tsv.

Every sample is written by this script (or compiled from a source string in
this script). Nothing is copied from a third-party binary. Re-running replaces
the corpus directory except it is rewritten in place file by file; delete the
directory first if you remove a sample from this script.

Requires: Python 3, gcc, and py7zr (see requirements.txt). The ELF sample is
produced by gcc, so its bytes include that compiler's .comment string and are
stable for a given gcc.
"""

from __future__ import annotations

import argparse
import gzip
import io
import json
import sqlite3
import struct
import subprocess
import tarfile
import zipfile
import zlib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
KB_PATH = ROOT / "assets" / "content_types_kb.min.json"

# KB entries with a null mime. The expected MIME is the type a correct
# identifier should emit; mfile currently prints an empty string for these.
MIME_OVERRIDE = {
    "sqlite": "application/vnd.sqlite3",
    "sum": "text/plain",
}

ELF_SRC = r"""
void _start(void) {
    __asm__ volatile("mov $60, %%eax; xor %%edi, %%edi; syscall" ::: "rax", "rdi");
}
"""

GO_MOD = """module github.com/DawnMagnet/mfile

go 1.27.3

require (
	github.com/gomlx/compute v0.1.15
	github.com/gomlx/gomlx v0.28.18
	github.com/gomlx/onnx-gomlx v0.5.15
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/gomlx/compute-onnx v0.1.14 // indirect
	github.com/gomlx/exceptions v0.0.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/exp v0.0.0-20260908205506-85c1c2202aba // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
)
"""


def chunk(tag: bytes, data: bytes) -> bytes:
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)


def png_1x1() -> bytes:
    sig = b"\x89PNG\r\n\x1a\n"
    ihdr = struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)
    raw = zlib.compress(b"\x00\xff\x00\x00")
    return sig + chunk(b"IHDR", ihdr) + chunk(b"IDAT", raw) + chunk(b"IEND", b"")


def gif_1x1() -> bytes:
    out = bytearray()
    out += b"GIF89a"
    out += struct.pack("<HH", 1, 1)
    out += bytes([0x80, 0, 0])
    out += b"\x00\x00\x00\xff\xff\xff"
    out += b"\x2c" + struct.pack("<HHHH", 0, 0, 1, 1) + b"\x00"
    out += b"\x02"
    bits: list[int] = []

    def emit(code: int, n: int) -> None:
        for i in range(n):
            bits.append((code >> i) & 1)

    emit(4, 3)
    emit(0, 3)
    emit(5, 3)
    while len(bits) % 8:
        bits.append(0)
    data = bytearray()
    for i in range(0, len(bits), 8):
        value = 0
        for j in range(8):
            value |= bits[i + j] << j
        data.append(value)
    out += bytes([len(data)]) + data + b"\x00\x3b"
    return bytes(out)


def jpeg_8x8() -> bytes:
    def seg(marker: bytes, data: bytes = b"") -> bytes:
        if marker in (b"\xff\xd8", b"\xff\xd9"):
            return marker
        return marker + struct.pack(">H", len(data) + 2) + data

    out = b"\xff\xd8"
    out += seg(b"\xff\xe0", b"JFIF\x00" + bytes([1, 1, 0, 0, 1, 0, 1, 0, 0]))
    out += seg(b"\xff\xdb", bytes([0]) + bytes([8] * 64))
    out += seg(b"\xff\xc0", bytes([8, 0, 8, 0, 8, 1, 1, 0x11, 0]))
    dc_bits = bytes([0, 1, 5, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0])
    dc_vals = bytes([0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11])
    out += seg(b"\xff\xc4", bytes([0x00]) + dc_bits + dc_vals)
    ac_bits = bytes([0, 2, 1, 3, 3, 2, 4, 3, 5, 5, 4, 4, 0, 0, 1, 0x7D])
    ac_vals = bytes(
        [
            0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07,
            0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08, 0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0,
            0x24, 0x33, 0x62, 0x72, 0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
            0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49,
            0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69,
            0x6A, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
            0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
            0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5,
            0xC6, 0xC7, 0xC8, 0xC9, 0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
            0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8,
            0xF9, 0xFA,
        ]
    )
    out += seg(b"\xff\xc4", bytes([0x10]) + ac_bits + ac_vals)
    out += seg(b"\xff\xda", bytes([1, 1, 0x00, 0, 63, 0]))
    # DC category 0 (bits 00) then AC EOB (bits 1010), padded with ones.
    out += bytes([0x2B, 0xFF, 0xD9])
    return out


def pe_header() -> bytes:
    e_lfanew = 0x80
    blob = bytearray(0x200)
    blob[0:2] = b"MZ"
    struct.pack_into("<I", blob, 0x3C, e_lfanew)
    blob[e_lfanew : e_lfanew + 4] = b"PE\x00\x00"
    coff = e_lfanew + 4
    struct.pack_into("<HHIIIHH", blob, coff, 0x8664, 1, 0, 0, 0, 0xF0, 0x22)
    opt = coff + 20
    struct.pack_into("<H", blob, opt, 0x20B)
    blob[opt + 2] = 14
    struct.pack_into("<I", blob, opt + 16, 0x1000)
    struct.pack_into("<Q", blob, opt + 24, 0x140000000)
    struct.pack_into("<II", blob, opt + 32, 0x1000, 0x200)
    struct.pack_into("<HH", blob, opt + 40, 6, 0)
    struct.pack_into("<I", blob, opt + 56, 0x2000)
    struct.pack_into("<I", blob, opt + 60, 0x200)
    struct.pack_into("<H", blob, opt + 70, 3)
    struct.pack_into("<I", blob, opt + 108, 16)
    sec = opt + 0xF0
    blob[sec : sec + 8] = b".text\x00\x00\x00"
    struct.pack_into("<IIIIIIHHI", blob, sec + 8, 0x10, 0x1000, 0x200, 0x200, 0, 0, 0, 0, 0x60000020)
    return bytes(blob)


def macho_header() -> bytes:
    blob = bytearray(2048)
    struct.pack_into("<III", blob, 0, 0xFEEDFACF, 0x01000007, 3)
    struct.pack_into("<IIII", blob, 12, 2, 1, 72 + 80, 1)
    off = 32
    struct.pack_into("<II", blob, off, 0x19, 72 + 80)
    seg = b"__TEXT" + b"\x00" * 10
    blob[off + 8 : off + 24] = seg
    struct.pack_into("<QQQQII", blob, off + 24, 0x100000000, 0x1000, 0, 0x1000, 5, 1)
    so = off + 72
    blob[so : so + 16] = b"__text" + b"\x00" * 10
    blob[so + 16 : so + 32] = seg
    struct.pack_into("<QQII", blob, so + 32, 0x100000000, 16, 0x200, 4)
    blob[0x200:0x210] = bytes([0xB8, 60, 0, 0, 0, 0x31, 0xFF, 0x0F, 0x05])
    return bytes(blob)


def wasm_module() -> bytes:
    return b"\x00asm\x01\x00\x00\x00"


def pdf_document() -> bytes:
    return b"""%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Count 1 /Kids [3 0 R] >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT /F1 12 Tf 72 72 Td (mfile qa) Tj ET
endstream
endobj
xref
0 5
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000214 00000 n 
trailer
<< /Size 5 /Root 1 0 R >>
startxref
308
%%EOF
"""


def ole_word() -> bytes:
    """Minimal Compound File with a WordDocument stream and a Word FIB."""
    sector = 512
    fatsect = 0xFFFFFFFD
    endofchain = 0xFFFFFFFE
    freesect = 0xFFFFFFFF
    header = bytearray(sector)
    header[0:8] = bytes.fromhex("D0CF11E0A1B11AE1")
    struct.pack_into("<H", header, 24, 0x003E)
    struct.pack_into("<H", header, 26, 0x0003)
    struct.pack_into("<H", header, 28, 0xFFFE)
    struct.pack_into("<H", header, 30, 9)
    struct.pack_into("<H", header, 32, 6)
    struct.pack_into("<I", header, 44, 1)
    struct.pack_into("<I", header, 48, 0)
    struct.pack_into("<I", header, 56, 4096)
    struct.pack_into("<I", header, 60, endofchain)
    struct.pack_into("<I", header, 68, endofchain)
    struct.pack_into("<I", header, 76, 1)
    fat = bytearray(sector)
    for i in range(sector // 4):
        struct.pack_into("<I", fat, i * 4, freesect)
    struct.pack_into("<I", fat, 0, endofchain)
    struct.pack_into("<I", fat, 4, fatsect)
    struct.pack_into("<I", fat, 8, endofchain)
    directory = bytearray(sector)

    def put_entry(index: int, name: str, kind: int, child: int = 0xFFFFFFFF, start: int = 0, size: int = 0) -> None:
        entry = memoryview(directory)[index * 128 : (index + 1) * 128]
        raw = name.encode("utf-16le")
        entry[0 : len(raw)] = raw
        struct.pack_into("<H", entry, 64, len(raw) + 2)
        entry[66] = kind
        entry[67] = 1
        struct.pack_into("<I", entry, 68, 0xFFFFFFFF)
        struct.pack_into("<I", entry, 72, 0xFFFFFFFF)
        struct.pack_into("<I", entry, 76, child)
        struct.pack_into("<I", entry, 116, start)
        struct.pack_into("<Q", entry, 120, size)

    stream = bytearray(struct.pack("<HH", 0xA5EC, 0x00C1) + b"\x00" * 16 + b"\x00" * 64)[:256]
    put_entry(0, "Root Entry", 5, child=1)
    put_entry(1, "WordDocument", 2, start=2, size=len(stream))
    return bytes(header + directory + fat + stream)


def zip_bytes(entries: list[tuple[str, bytes]]) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for name, data in entries:
            info = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            zf.writestr(info, data)
    return buf.getvalue()


def docx_bytes() -> bytes:
    return zip_bytes(
        [
            (
                "[Content_Types].xml",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>""",
            ),
            (
                "_rels/.rels",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>""",
            ),
            (
                "word/document.xml",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>mfile qa sample document</w:t></w:r></w:p></w:body></w:document>""",
            ),
        ]
    )


def xlsx_bytes() -> bytes:
    return zip_bytes(
        [
            (
                "[Content_Types].xml",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>""",
            ),
            (
                "xl/workbook.xml",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>""",
            ),
            (
                "xl/worksheets/sheet1.xml",
                b"""<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>qa</t></is></c></row></sheetData></worksheet>""",
            ),
        ]
    )


def jar_bytes() -> bytes:
    # Original empty class (CAFEBABE) plus a manifest, both written here.
    klass = bytes.fromhex(
        "cafebabe00000034000a0a000200030700040c000500060100063c696e69743e"
        "010003282956010004436f646507000801001068656c6c6f0021000700020000"
        "0000000100010005000100060000001100010001000000052ab70001b1000000"
        "0000010008000000020009"
    )
    return zip_bytes(
        [
            ("META-INF/MANIFEST.MF", b"Manifest-Version: 1.0\nCreated-By: mfile-qa\n\n"),
            ("Hello.class", klass),
        ]
    )


def seven_zip(payload: bytes) -> bytes:
    import py7zr
    import py7zr.helpers as helpers

    fixed = helpers.ArchiveTimestamp.from_datetime(1577836800.0)

    def from_now(cls):
        return fixed

    helpers.ArchiveTimestamp.from_now = classmethod(from_now)
    buf = io.BytesIO()
    with py7zr.SevenZipFile(buf, "w") as archive:
        archive.writestr(payload, "readme.txt")
    return buf.getvalue()


def gnu_tar(payload: bytes) -> bytes:
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w", format=tarfile.GNU_FORMAT) as tf:
        info = tarfile.TarInfo("readme.txt")
        info.size = len(payload)
        info.mtime = 0
        info.uid = 0
        info.gid = 0
        info.uname = ""
        info.gname = ""
        tf.addfile(info, io.BytesIO(payload))
    return buf.getvalue()


def build_elf(dest: Path) -> None:
    src = Path("/tmp/mfile-corpus-elf.c")
    src.write_text(ELF_SRC, encoding="utf-8")
    subprocess.check_call(
        [
            "gcc",
            "-static",
            "-nostdlib",
            "-s",
            "-Os",
            "-Wl,--build-id=none",
            "-o",
            str(dest),
            str(src),
        ],
        stdout=subprocess.DEVNULL,
    )


def write_sqlite(dest: Path) -> None:
    if dest.exists():
        dest.unlink()
    conn = sqlite3.connect(dest)
    try:
        conn.execute("create table qa(id integer primary key, name text)")
        conn.execute("insert into qa(id, name) values (1, 'mfile qa')")
        conn.commit()
    finally:
        conn.close()


TEXT: list[tuple[str, str, str, str, str]] = [
    (
        "code/hello.c",
        "c",
        "C source",
        "C source with stdio and a struct, no C++ tokens",
        """\
#include <stdio.h>
#include <string.h>

struct sample {
    int id;
    char name[32];
};

static int answer(const struct sample *s) {
    return s->id + (int)strlen(s->name);
}

int main(void) {
    struct sample s;
    s.id = 7;
    strcpy(s.name, "qa");
    printf("%d\\n", answer(&s));
    return 0;
}
""",
    ),
    (
        "code/hello.cpp",
        "cpp",
        "C++",
        "C++ source using iostream and a class",
        """\
#include <iostream>
#include <string>
#include <vector>

class Sample {
public:
    explicit Sample(std::string name) : name_(std::move(name)) {}
    void greet() const { std::cout << "hello " << name_ << std::endl; }
private:
    std::string name_;
};

int main() {
    std::vector<Sample> items;
    items.emplace_back("qa");
    for (const auto& item : items) {
        item.greet();
    }
    return 0;
}
""",
    ),
    (
        "code/hello.go",
        "go",
        "Go",
        "Go source; distinct from the go.mod fixture",
        """\
package main

import "fmt"

type sample struct {
    ID   int
    Name string
}

func (s sample) String() string {
    return fmt.Sprintf("%d:%s", s.ID, s.Name)
}

func main() {
    fmt.Println(sample{ID: 1, Name: "qa"})
}
""",
    ),
    (
        "code/hello.rs",
        "rust",
        "Rust",
        "",
        """\
fn answer(n: i32) -> i32 {
    n * 2
}

fn main() {
    let values = vec![1, 2, 3];
    let total: i32 = values.iter().map(|v| answer(*v)).sum();
    println!("total {total}");
}
""",
    ),
    (
        "code/hello.py",
        "python",
        "Python",
        "Python with a shebang",
        """\
#!/usr/bin/env python3
# Small original Python sample.

def answer(n: int) -> int:
    return n * 2


def main() -> None:
    values = [1, 2, 3]
    print(sum(answer(v) for v in values))


if __name__ == "__main__":
    main()
""",
    ),
    (
        "code/hello_noshebang.py",
        "python",
        "Python",
        "Python without a shebang",
        """\
class Sample:
    def __init__(self, name):
        self.name = name

    def greet(self):
        return "hello " + self.name


def main():
    print(Sample("qa").greet())


if __name__ == "__main__":
    main()
""",
    ),
    (
        "code/Hello.java",
        "java",
        "Java",
        "",
        """\
public class Hello {
    private final String name;

    public Hello(String name) {
        this.name = name;
    }

    public String greet() {
        return "hello " + name;
    }

    public static void main(String[] args) {
        System.out.println(new Hello("qa").greet());
    }
}
""",
    ),
    (
        "code/hello.js",
        "javascript",
        "JavaScript",
        "",
        """\
function answer(n) {
  return n * 2;
}

const values = [1, 2, 3];
const total = values.map(answer).reduce((a, b) => a + b, 0);
console.log(`total ${total}`);
""",
    ),
    (
        "code/hello.ts",
        "typescript",
        "TypeScript",
        "",
        """\
interface Sample {
  id: number;
  name: string;
}

function greet(s: Sample): string {
  return `${s.id}:${s.name}`;
}

const item: Sample = { id: 1, name: "qa" };
console.log(greet(item));
""",
    ),
    (
        "code/hello.rb",
        "ruby",
        "Ruby",
        "",
        """\
class Sample
  def initialize(name)
    @name = name
  end

  def greet
    "hello #{@name}"
  end
end

puts Sample.new("qa").greet
""",
    ),
    (
        "code/hello.php",
        "php",
        "PHP",
        "",
        """\
<?php
class Sample {
    public function __construct(private string $name) {}
    public function greet(): string {
        return "hello " . $this->name;
    }
}
echo (new Sample("qa"))->greet(), PHP_EOL;
""",
    ),
    (
        "code/hello.pl",
        "perl",
        "Perl",
        "",
        """\
use strict;
use warnings;

sub answer {
    my ($n) = @_;
    return $n * 2;
}

my $total = answer(21);
print "total $total\\n";
""",
    ),
    (
        "code/hello.lua",
        "lua",
        "Lua",
        "",
        """\
local function answer(n)
  return n * 2
end

local sample = { id = 1, name = "qa" }
print(sample.name, answer(sample.id))
""",
    ),
    (
        "code/hello.kt",
        "kotlin",
        "Kotlin",
        "",
        """\
data class Sample(val id: Int, val name: String)

fun greet(s: Sample): String = "${s.id}:${s.name}"

fun main() {
    println(greet(Sample(1, "qa")))
}
""",
    ),
    (
        "code/hello.swift",
        "swift",
        "Swift",
        "",
        """\
struct Sample {
    let id: Int
    let name: String
    func greet() -> String { "\\(id):\\(name)" }
}

let item = Sample(id: 1, name: "qa")
print(item.greet())
""",
    ),
    (
        "code/hello.scala",
        "scala",
        "Scala",
        "",
        """\
object Hello {
  case class Sample(id: Int, name: String)
  def greet(s: Sample): String = s"${s.id}:${s.name}"
  def main(args: Array[String]): Unit = {
    println(greet(Sample(1, "qa")))
  }
}
""",
    ),
    (
        "code/Hello.hs",
        "haskell",
        "Haskell",
        "",
        """\
module Main where

data Sample = Sample Int String

greet :: Sample -> String
greet (Sample i n) = show i ++ ":" ++ n

main :: IO ()
main = putStrLn (greet (Sample 1 "qa"))
""",
    ),
    (
        "code/hello.zig",
        "zig",
        "Zig",
        "",
        """\
const std = @import("std");

fn answer(n: i32) i32 {
    return n * 2;
}

pub fn main() void {
    std.debug.print("answer {d}\\n", .{answer(21)});
}
""",
    ),
    (
        "code/hello.cs",
        "cs",
        "C#",
        "",
        """\
using System;

sealed class Sample {
    public int Id { get; }
    public string Name { get; }
    public Sample(int id, string name) { Id = id; Name = name; }
    public string Greet() => $"{Id}:{Name}";
}

static class Program {
    static void Main() {
        Console.WriteLine(new Sample(1, "qa").Greet());
    }
}
""",
    ),
    (
        "code/query.sql",
        "sql",
        "SQL",
        "",
        """\
CREATE TABLE samples (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO samples (id, name) VALUES (1, 'qa');

SELECT s.id, s.name
FROM samples AS s
WHERE s.id = 1
ORDER BY s.name;
""",
    ),
    (
        "code/style.css",
        "css",
        "CSS",
        "",
        """\
:root {
  --ink: #222;
  --paper: #fff;
}

body {
  color: var(--ink);
  background: var(--paper);
  font-family: sans-serif;
}

.sample > p {
  margin: 0 0 1rem;
}
""",
    ),
    (
        "code/page.html",
        "html",
        "HTML",
        "",
        """\
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>QA sample</title>
</head>
<body>
  <h1>QA sample</h1>
  <p>Original HTML fixture for the corpus.</p>
</body>
</html>
""",
    ),
    (
        "code/with_shebang.sh",
        "shell",
        "shell",
        "shell script with a shebang",
        """\
#!/bin/sh
set -eu

answer() {
    echo $(( $1 * 2 ))
}

answer 21
""",
    ),
    (
        "code/no_shebang.sh",
        "shell",
        "shell",
        "shell script without a shebang",
        """\
set -eu

answer() {
    echo $(( $1 * 2 ))
}

for n in 1 2 3; do
    answer "$n"
done
""",
    ),
    (
        "config/app.json",
        "json",
        "JSON",
        "",
        """\
{
  "name": "qa-sample",
  "enabled": true,
  "retries": 3,
  "tags": ["corpus", "mfile"]
}
""",
    ),
    (
        "config/app.yaml",
        "yaml",
        "YAML",
        "",
        """\
name: qa-sample
enabled: true
retries: 3
tags:
  - corpus
  - mfile
service:
  host: localhost
  port: 8080
""",
    ),
    (
        "config/app.toml",
        "toml",
        "TOML",
        "",
        """\
name = "qa-sample"
enabled = true
retries = 3

[service]
host = "localhost"
port = 8080

[[tags]]
name = "corpus"
""",
    ),
    (
        "config/app.ini",
        "ini",
        "INI",
        "",
        """\
[service]
host = localhost
port = 8080
enabled = true

[logging]
level = info
""",
    ),
    (
        "config/app.xml",
        "xml",
        "XML",
        "",
        """\
<?xml version="1.0" encoding="UTF-8"?>
<config>
  <name>qa-sample</name>
  <enabled>true</enabled>
  <port>8080</port>
</config>
""",
    ),
    (
        "config/data.csv",
        "csv",
        "CSV",
        "",
        """\
id,name,ok
1,qa,true
2,corpus,false
3,mfile,true
""",
    ),
    (
        "config/data.tsv",
        "tsv",
        "TSV",
        "repo KB mime is text/tab-separated-values; magika 1.0.3 KB says text/tsv",
        "id\tname\tok\n1\tqa\ttrue\n2\tcorpus\tfalse\n3\tmfile\ttrue\n",
    ),
    (
        "text/readme.md",
        "markdown",
        "Markdown",
        "",
        """\
# QA sample

Original markdown fixture.

- one
- two

See [the repo](https://example.com) and `code/hello.go`.

```
echo hello
```
""",
    ),
    (
        "text/plain.txt",
        "txt",
        "ASCII",
        "plain ASCII prose",
        "This is a plain ASCII paragraph written for the mfile QA corpus.\n"
        "It is not source code, not markdown, and not configuration.\n"
        "The quick brown fox jumps over the lazy dog.\n",
    ),
]

BYNAME_NOTES = {
    "byname/go.mod": "known-fail: model labels this verilog at about 0.745; filename layer is item 5",
}


def text_samples() -> list[tuple[str, str, str, str, str, str]]:
    rows = [(path, label, sub, "0", note, body) for path, label, sub, note, body in TEXT]
    extra = [
        (
            "byname/go.mod",
            "txt",
            "ASCII",
            "1",
            BYNAME_NOTES["byname/go.mod"],
            GO_MOD,
        ),
        (
            "byname/go.sum",
            "sum",
            "ASCII",
            "0",
            "checksum file; KB mime_type is null so mfile emits an empty MIME",
            "example.com/dep v1.2.3 h1:abcdefghijklmnopqrstuvwxyz0123456789ABCDEF=\n"
            "example.com/dep v1.2.3/go.mod h1:abcdefghijklmnopqrstuvwxyz0123456789ABCDEF=\n"
            "rsc.io/quote v1.5.2 h1:zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz=\n"
            "rsc.io/quote v1.5.2/go.mod h1:yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy=\n",
        ),
        (
            "byname/go.work",
            "txt",
            "ASCII",
            "0",
            "no go.work label in the Magika vocabulary",
            "go 1.22\n\nuse (\n\t./pkg\n\t./cmd\n)\n",
        ),
        (
            "byname/Cargo.lock",
            "toml",
            "ASCII",
            "0",
            "Cargo.lock is TOML; identification does not use the file name",
            "# This file is automatically @generated by Cargo.\n"
            "version = 3\n\n"
            "[[package]]\n"
            'name = "qa-sample"\n'
            'version = "0.1.0"\n'
            'dependencies = ["libc"]\n\n'
            "[[package]]\n"
            'name = "libc"\n'
            'version = "0.2.155"\n'
            'source = "registry+https://github.com/rust-lang/crates.io-index"\n'
            'checksum = "1111111111111111111111111111111111111111111111111111111111111111"\n',
        ),
        (
            "byname/Makefile",
            "makefile",
            "makefile",
            "0",
            "",
            ".PHONY: all test clean\n\n"
            "all: hello\n\n"
            "hello: hello.c\n"
            "\tcc -Wall -O2 -o hello hello.c\n\n"
            "test: hello\n"
            "\t./hello\n\n"
            "clean:\n"
            "\trm -f hello\n",
        ),
        (
            "byname/Dockerfile",
            "dockerfile",
            "Dockerfile",
            "0",
            "",
            "FROM alpine:3.20\n"
            "RUN apk add --no-cache ca-certificates\n"
            "COPY hello /usr/local/bin/hello\n"
            "USER nobody\n"
            'CMD ["hello"]\n',
        ),
        (
            "byname/CMakeLists.txt",
            "cmake",
            "CMake",
            "0",
            "",
            "cmake_minimum_required(VERSION 3.16)\n"
            "project(QaSample C)\n"
            "add_executable(hello hello.c)\n",
        ),
        (
            "byname/.gitignore",
            "ignorefile",
            "ASCII",
            "0",
            "",
            "*.o\n*.a\n/bin/\n/build/\n.DS_Store\n",
        ),
        (
            "byname/PKGBUILD",
            "shell",
            "shell",
            "0",
            "PKGBUILD is shell; no shebang, name is the usual signal",
            "pkgname=qa-sample\n"
            "pkgver=1.0.0\n"
            "pkgrel=1\n"
            'pkgdesc="synthetic QA sample"\n'
            "arch=('x86_64')\n"
            "license=('MIT')\n"
            "source=()\n"
            "sha256sums=()\n\n"
            "package() {\n"
            '  install -Dm644 README "$pkgdir/usr/share/doc/qa-sample/README"\n'
            "}\n",
        ),
        (
            "byname/flake.lock",
            "json",
            "JSON",
            "0",
            "flake.lock is JSON",
            json.dumps(
                {
                    "nodes": {
                        "nixpkgs": {
                            "locked": {
                                "lastModified": 1,
                                "narHash": "sha256-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=",
                                "owner": "NixOS",
                                "repo": "nixpkgs",
                                "rev": "0123456789abcdef0123456789abcdef01234567",
                                "type": "github",
                            }
                        }
                    },
                    "root": "nixpkgs",
                    "version": 7,
                },
                indent=2,
            )
            + "\n",
        ),
    ]
    rows.extend(extra)
    return rows


def edge_and_mismatch(png: bytes) -> list[tuple[str, str, str, str, str, bytes]]:
    utf8 = "The café serves crème brûlée and the naïve résumé.\n".encode("utf-8")
    rows: list[tuple[str, str, str, str, str, bytes]] = [
        ("edge/empty", "empty", "empty", "0", "zero-length file", b""),
        ("edge/one_byte.txt", "txt", "very short file", "0", "one ASCII byte; below min_file_size_for_dl", b"A"),
        (
            "edge/one_nul.bin",
            "txt",
            "very short file",
            "0",
            "NUL is valid UTF-8, so the short-file rule yields txt",
            b"\x00",
        ),
        (
            "edge/one_ff.bin",
            "unknown",
            "very short file",
            "0",
            "single non-UTF-8 byte; short-file rule yields unknown",
            b"\xff",
        ),
        (
            "edge/ascii.txt",
            "txt",
            "ASCII",
            "0",
            "plain ASCII",
            b"This is a plain ASCII paragraph for the edge corpus.\nIt has several lines and no markup.\n",
        ),
        ("edge/utf8_bom.txt", "txt", "UTF-8", "0", "UTF-8 with BOM", b"\xef\xbb\xbf" + utf8),
        ("edge/utf8.txt", "txt", "UTF-8", "0", "UTF-8 without BOM", utf8 * 4),
        (
            "edge/utf16le.txt",
            "txt",
            "UTF-16",
            "0",
            "UTF-16 LE with BOM",
            "Hello from the QA corpus. This is UTF-16 text.\n".encode("utf-16-le")
            and b"\xff\xfe" + "Hello from the QA corpus. This is UTF-16 text.\n".encode("utf-16-le") * 8,
        ),
        (
            "edge/utf16be.txt",
            "txt",
            "UTF-16",
            "0",
            "UTF-16 BE with BOM",
            b"\xfe\xff" + "Hello from the QA corpus. This is UTF-16 text.\n".encode("utf-16-be") * 8,
        ),
        (
            "edge/latin1.txt",
            "txt",
            "ISO-8859",
            "0",
            "ISO-8859-1 text, not valid UTF-8",
            "Cafe\u00e9 na\u00efve fa\u00e7ade - deja vu.\n".encode("latin-1") * 20,
        ),
        (
            "edge/random.bin",
            "unknown",
            "data",
            "0",
            "deterministic pseudo-random bytes",
            bytes(int.from_bytes(__import__("random").Random(20260328).randbytes(1), "little") for _ in range(4096))
            if False
            else __import__("random").Random(20260328).randbytes(4096),
        ),
        (
            "edge/long_line.txt",
            "txt",
            "ASCII",
            "0",
            "one line longer than 100KB",
            b"long-line " + b"a" * 100_000,
        ),
        (
            "edge/whitespace.txt",
            "txt",
            "ASCII",
            "0",
            "only whitespace; the model is not run",
            (b" \t\n\r\f\v" * 800),
        ),
        ("edge/bom_only.txt", "txt", "UTF-8", "0", "BOM alone is shorter than min_file_size_for_dl", b"\xef\xbb\xbf"),
        (
            "mismatch/photo.png",
            "json",
            "JSON",
            "0",
            "extension says png; body is JSON",
            (b'{"type":"mismatch","reason":"extension is png but the body is json"}\n' * 20),
        ),
        (
            "mismatch/notes.txt",
            "png",
            "PNG",
            "0",
            "extension says txt; body is a PNG",
            png,
        ),
        (
            "mismatch/README",
            "python",
            "Python",
            "0",
            "no extension; body is Python",
            b"def greet(name):\n    return 'hello ' + name\n\nif __name__ == '__main__':\n    print(greet('qa'))\n",
        ),
        (
            "mismatch/blob",
            "gzip",
            "gzip",
            "0",
            "no extension; body is gzip",
            gzip.compress(b"mfile qa payload\n", mtime=0),
        ),
    ]
    return rows


def mime_for(kb: dict, label: str) -> str:
    if label in MIME_OVERRIDE:
        return MIME_OVERRIDE[label]
    entry = kb.get(label)
    if entry is None:
        raise SystemExit(f"label {label!r} is not in the content-types KB")
    return entry.get("mime_type") or ""


def check_field(value: str, what: str) -> str:
    if "\t" in value or "\n" in value:
        raise SystemExit(f"{what} contains a tab or newline: {value!r}")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", type=Path, default=ROOT / "testdata" / "corpus")
    args = parser.parse_args()
    out: Path = args.out
    kb = json.loads(KB_PATH.read_text(encoding="utf-8"))
    if out.exists():
        for child in sorted(out.rglob("*"), reverse=True):
            if child.is_file():
                child.unlink()
            elif child.is_dir():
                child.rmdir()
    out.mkdir(parents=True, exist_ok=True)

    records: list[tuple[str, str, str, str, str, str]] = []

    def add(rel: str, label: str, file_substr: str, known_fail: str, notes: str, data: bytes) -> None:
        dest = out / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(data)
        records.append(
            (
                check_field(rel, "path"),
                check_field(label, "label"),
                check_field(mime_for(kb, label), "mime"),
                check_field(file_substr, "file_substr"),
                check_field(known_fail, "known_fail"),
                check_field(notes, "notes"),
            )
        )

    for rel, label, file_substr, known_fail, notes, body in text_samples():
        add(rel, label, file_substr, known_fail, notes, body.encode("utf-8"))

    png = png_1x1()
    for rel, label, file_substr, known_fail, notes, data in edge_and_mismatch(png):
        add(rel, label, file_substr, known_fail, notes, data)

    payload = b"mfile qa payload\n"
    binaries: list[tuple[str, str, str, str, bytes]] = [
        ("binary/pixel.png", "png", "PNG", "", png),
        ("binary/pixel.jpg", "jpeg", "JPEG", "", jpeg_8x8()),
        ("binary/pixel.gif", "gif", "GIF", "", gif_1x1()),
        ("binary/sample.pdf", "pdf", "PDF", "", pdf_document()),
        ("binary/sample.gz", "gzip", "gzip", "", gzip.compress(payload, mtime=0)),
        ("binary/sample.bz2", "bzip", "bzip2", "", __import__("bz2").compress(payload)),
        ("binary/sample.xz", "xz", "XZ", "", __import__("lzma").compress(payload)),
        ("binary/sample.zip", "zip", "Zip", "", zip_bytes([("readme.txt", payload)])),
        ("binary/sample.tar", "tar", "tar", "", gnu_tar(payload)),
        ("binary/sample.7z", "sevenzip", "7-zip", "", seven_zip(payload)),
        ("binary/sample.wasm", "wasm", "WebAssembly", "", wasm_module()),
        ("binary/hello.exe", "pebin", "PE32", "", pe_header()),
        ("binary/hello.macho", "macho", "Mach-O", "", macho_header()),
        ("binary/sample.docx", "docx", "Microsoft Word", "", docx_bytes()),
        ("binary/sample.xlsx", "xlsx", "Microsoft Excel", "", xlsx_bytes()),
        ("binary/sample.jar", "jar", "Java archive", "JAR with a manifest and a class file", jar_bytes()),
        ("binary/sample.doc", "doc", "Composite Document File", "synthetic OLE with a WordDocument FIB", ole_word()),
    ]
    for rel, label, file_substr, notes, data in binaries:
        add(rel, label, file_substr, "0", notes, data)

    elf_path = out / "binary" / "hello.elf"
    elf_path.parent.mkdir(parents=True, exist_ok=True)
    build_elf(elf_path)
    records.append(
        (
            "binary/hello.elf",
            "elf",
            mime_for(kb, "elf"),
            "ELF",
            "0",
            "compiled by this script with gcc -static -nostdlib -Wl,--build-id=none",
        )
    )
    sqlite_path = out / "binary" / "sample.sqlite"
    write_sqlite(sqlite_path)
    records.append(
        (
            "binary/sample.sqlite",
            "sqlite",
            mime_for(kb, "sqlite"),
            "SQLite",
            "0",
            "KB mime_type is null; expected MIME is application/vnd.sqlite3",
        )
    )

    records.sort()
    lines = ["path\tlabel\tmime\tfile_substr\tknown_fail\tnotes"]
    for rec in records:
        lines.append("\t".join(rec))
    (out / "expected.tsv").write_text("\n".join(lines) + "\n", encoding="utf-8")
    total = sum(p.stat().st_size for p in out.rglob("*") if p.is_file())
    print(f"wrote {len(records)} samples, {total} bytes, under {out}")


if __name__ == "__main__":
    main()
