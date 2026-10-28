#!/usr/bin/env bash
# qemu-user CPU matrix for the amd64 GOEXPERIMENT=simd release build.
#
# The CPU guard re-execs with syscall.Exec. A direct "qemu-x86_64 -cpu … ./mfile"
# invocation does not survive that: the child is a normal x86_64 ELF, so the
# kernel runs it on the host CPU and the test stops measuring the emulated
# model. Registering a binfmt_misc interpreter fixes that. The interpreter is
# pinned with flag F (opened at registration) and QEMU_CPU stays in the
# environment the guard copies, so the re-exec'd child is launched by the same
# qemu and the same CPU model.
#
# The magic matches only ELF64 LE SYSV binaries whose e_ident[15] is 'M'.
# A normal Go binary has 0 there, so this entry does not capture host ELFs
# and does not replace qemu-user-binfmt's catch-all handlers. The matrix
# copies the release binary and writes that one marker byte.
#
# qemu is the Debian trixie static binary from qemu-user 1:10.0.13+ds-0+deb13u1,
# extracted, not installed. The qemu-user-static package in that release is
# only compatibility symlinks and depends on qemu-user-binfmt, which would
# register an interpreter for every x86_64 ELF. Skipping dpkg -i also skips
# that postinst.
#
# sudo registration is global on purpose. GitHub-hosted runners are one-shot
# VMs. A private user namespace (unshare -Urm) also works, but Ubuntu 24.04
# blocks unprivileged user namespaces unless apparmor_restrict_unprivileged_userns
# is turned off, which is a second sysctl and easier to get wrong.

set -euo pipefail

QEMU_DEB_URL="https://deb.debian.org/debian/pool/main/q/qemu/qemu-user_10.0.13+ds-0+deb13u1_amd64.deb"
QEMU_DEB_SHA256="ca6ede739327a20ae5a3498230c8a3a9a072c586e131bc6bcc1201eb1725e336"
QEMU_VERSION_NEEDLE="10.0.13"
BINFMT_NAME="mfile-qemu"
# e_ident[0:16] of a Go linux/amd64 ELF, then the same header with byte 15 = 'M'.
HOST_IDENT="7f454c46020101000000000000000000"
TAGGED_IDENT="7f454c4602010100000000000000004d"

# Four named inputs plus a dozen corpus files. Types are spread across source,
# config, text, image, ELF, an empty file, and a by-name Dockerfile.
# Haswell AVX2 under TCG is the slow model (a 97-file run was about 14 minutes
# locally), so the sample stays in this range instead of the full corpus.
sample_files=(
  cmd/mfile/main.go
  LICENSE
  README.md
  go.mod
  testdata/corpus/code/hello.go
  testdata/corpus/code/hello.py
  testdata/corpus/code/page.html
  testdata/corpus/code/with_shebang.sh
  testdata/corpus/config/app.json
  testdata/corpus/config/app.yaml
  testdata/corpus/config/data.csv
  testdata/corpus/text/plain.txt
  testdata/corpus/binary/pixel.png
  testdata/corpus/binary/hello.elf
  testdata/corpus/edge/empty
  testdata/corpus/byname/Dockerfile
)

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 1

out="$root/bin/qemu-matrix"
qemu="$root/bin/qemu-user/usr/bin/qemu-x86_64"
bin="$root/bin/mfile"
tagged="$root/bin/mfile.tagged"
fail=0

die() {
  echo "qemu-matrix: $*" >&2
  exit 1
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "required command not found: $1"
  fi
}

ident_of() {
  od -An -tx1 -N16 -v "$1" | tr -d ' \n'
}

field() {
  local key="$1" file="$2"
  awk -F= -v "k=$key" '$1 == k { print $2; found = 1 } END { if (!found) exit 1 }' "$file"
}

note() {
  echo "qemu-matrix: $*"
}

append_summary() {
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    printf '%s\n' "$*" >>"$GITHUB_STEP_SUMMARY"
  fi
}

install_qemu() {
  need_cmd curl
  need_cmd sha256sum
  need_cmd dpkg-deb
  need_cmd tar
  local tmp deb
  tmp=$(mktemp -d)
  deb="$tmp/qemu-user.deb"
  note "downloading $QEMU_DEB_URL"
  curl -fsSL --retry 5 --retry-delay 2 -o "$deb" "$QEMU_DEB_URL"
  printf '%s  %s\n' "$QEMU_DEB_SHA256" "$deb" | sha256sum -c -
  mkdir -p "$(dirname "$qemu")"
  dpkg-deb --fsys-tarfile "$deb" | tar -x -C "$root/bin/qemu-user" ./usr/bin/qemu-x86_64
  rm -rf "$tmp"
  [[ -x "$qemu" ]] || die "extracted qemu is not executable: $qemu"
  local ver
  # Do not pipe through head: qemu exits with SIGPIPE and pipefail would fail.
  ver=$("$qemu" --version)
  ver=${ver%%$'\n'*}
  note "qemu: $ver"
  if [[ "$ver" != *"$QEMU_VERSION_NEEDLE"* ]]; then
    die "qemu version ${ver} does not contain ${QEMU_VERSION_NEEDLE}"
  fi
  printf '%s\n' "$ver" >"$out/qemu-version.txt"
  printf '%s\n' "$QEMU_DEB_URL" >"$out/qemu-source.txt"
  printf '%s\n' "$QEMU_DEB_SHA256" >"$out/qemu-sha256.txt"
}

build_mfile() {
  need_cmd go
  note "building CGO_ENABLED=0 GOEXPERIMENT=simd GOAMD64=v1"
  mkdir -p "$root/bin"
  CGO_ENABLED=0 GOEXPERIMENT=simd GOAMD64=v1 \
    go build -trimpath -ldflags='-s -w' -o "$bin" ./cmd/mfile
  [[ -x "$bin" ]] || die "build did not produce $bin"
}

tag_binary() {
  local host tagged_hex
  host=$(ident_of "$bin")
  if [[ "$host" != "$HOST_IDENT" ]]; then
    echo "e_ident of $bin is $host" >&2
    die "refusing to tag: expected Go linux/amd64 e_ident ${HOST_IDENT}"
  fi
  cp -a "$bin" "$tagged"
  printf 'M' | dd of="$tagged" bs=1 seek=15 conv=notrunc status=none
  tagged_hex=$(ident_of "$tagged")
  if [[ "$tagged_hex" != "$TAGGED_IDENT" ]]; then
    die "tagged e_ident is ${tagged_hex}, expected ${TAGGED_IDENT}"
  fi
}

register_binfmt() {
  if ! sudo -n true; then
    die "passwordless sudo is required to register the private binfmt_misc entry"
  fi
  if [[ ! -e /proc/sys/fs/binfmt_misc/register ]]; then
    sudo mount -t binfmt_misc binfmt_misc /proc/sys/fs/binfmt_misc
  fi
  if [[ -e "/proc/sys/fs/binfmt_misc/${BINFMT_NAME}" ]]; then
    echo -1 | sudo tee "/proc/sys/fs/binfmt_misc/${BINFMT_NAME}" >/dev/null
  fi
  # Text \xNN escapes. binfmt_misc decodes them. Byte 15 of the magic is 'M'.
  local line
  line=$(printf '%s' ":${BINFMT_NAME}:M::\\x7fELF\\x02\\x01\\x01\\x00\\x00\\x00\\x00\\x00\\x00\\x00\\x00M::${qemu}:F")
  if ! printf '%s\n' "$line" | sudo tee /proc/sys/fs/binfmt_misc/register >/dev/null; then
    echo "binfmt register failed. current entries:" >&2
    ls -l /proc/sys/fs/binfmt_misc >&2 || true
    die "could not register ${BINFMT_NAME}"
  fi
  local status
  status=$(cat "/proc/sys/fs/binfmt_misc/${BINFMT_NAME}")
  printf '%s\n' "$status" >"$out/binfmt.txt"
  if ! grep -q "magic ${TAGGED_IDENT}" <<<"$status"; then
    die "binfmt magic is not the tagged e_ident"
  fi
  if ! grep -q 'flags: F' <<<"$status"; then
    die "binfmt entry is missing flag F"
  fi
  if ! grep -F -q "interpreter ${qemu}" <<<"$status"; then
    die "binfmt interpreter is not ${qemu}"
  fi
  note "binfmt ${BINFMT_NAME} matches only e_ident[15]='M'"
}

# A catch-all x86_64 handler would make the host reference run under qemu.
assert_untagged_stays_native() {
  local native westmere
  native=$(env -u QEMU_CPU -u MFILE_CPU_GUARD "$bin" --version | awk -F= '/^simd=/{print $2}')
  westmere=$(env -u MFILE_CPU_GUARD QEMU_CPU=Westmere "$bin" --version | awk -F= '/^simd=/{print $2}')
  printf '%s\n' "$native" >"$out/host-simd.txt"
  if [[ -z "$native" || "$native" != "$westmere" ]]; then
    die "untagged binary changed simd under QEMU_CPU=Westmere (${native} -> ${westmere}); binfmt captured a normal ELF"
  fi
  note "untagged binary stays native (simd=${native}) when QEMU_CPU is set"
}

assert_host_avx2() {
  local flags simd
  flags=$(awk '/^flags[[:space:]]*:/{print; exit}' /proc/cpuinfo || true)
  if [[ " $flags " != *" avx2 "* ]]; then
    echo "FAIL Haswell: host CPU does not advertise AVX2 in /proc/cpuinfo." >"$out/haswell.error"
    die "host CPU has no AVX2. The Haswell reference is a native GOMLX_GO_SIMD_AVX512=0 run and is only an avx2 result on an AVX2 host. Not skipping."
  fi
  simd=$(env -u QEMU_CPU -u MFILE_CPU_GUARD -u GODEBUG GOMLX_GO_SIMD_AVX512=0 "$bin" --version | awk -F= '/^simd=/{print $2}')
  printf '%s\n' "$simd" >"$out/ref-avx2.simd"
  if [[ "$simd" != "avx2" ]]; then
    echo "FAIL Haswell: native GOMLX_GO_SIMD_AVX512=0 reports simd=${simd}, not avx2." >"$out/haswell.error"
    die "Haswell reference is not the avx2 path (simd=${simd}). Not skipping."
  fi
}

write_references() {
  local rc
  set +e
  env -u QEMU_CPU -u MFILE_CPU_GUARD -u GOMLX_GO_SIMD_AVX2 -u GOMLX_GO_SIMD_AVX512 \
    GODEBUG=cpu.all=off \
    "$bin" --json "${sample_files[@]}" \
    >"$out/ref-cpu-all-off.json" 2>"$out/ref-cpu-all-off.err"
  rc=$?
  set -e
  echo "$rc" >"$out/ref-cpu-all-off.rc"
  [[ "$rc" -eq 0 ]] || die "host GODEBUG=cpu.all=off reference exited ${rc}"
  if grep -Eiq 'SIGILL|illegal instruction' "$out/ref-cpu-all-off.err"; then
    die "host GODEBUG=cpu.all=off reference hit SIGILL"
  fi
  local soft
  soft=$(env -u QEMU_CPU -u MFILE_CPU_GUARD GODEBUG=cpu.all=off "$bin" --version | awk -F= '/^simd=/{print $2}')
  printf '%s\n' "$soft" >"$out/ref-cpu-all-off.simd"
  if [[ "$soft" != "software" ]]; then
    die "host GODEBUG=cpu.all=off reports simd=${soft}, expected software"
  fi

  set +e
  env -u QEMU_CPU -u MFILE_CPU_GUARD -u GODEBUG -u GOMLX_GO_SIMD_AVX2 \
    GOMLX_GO_SIMD_AVX512=0 \
    "$bin" --json "${sample_files[@]}" \
    >"$out/ref-avx2.json" 2>"$out/ref-avx2.err"
  rc=$?
  set -e
  echo "$rc" >"$out/ref-avx2.rc"
  [[ "$rc" -eq 0 ]] || die "host GOMLX_GO_SIMD_AVX512=0 reference exited ${rc}"
}

run_version() {
  local cpu="$1" slug="$2"
  local rc
  set +e
  env -u MFILE_CPU_GUARD -u GODEBUG -u GOMLX_GO_SIMD_AVX2 -u GOMLX_GO_SIMD_AVX512 \
    QEMU_CPU="$cpu" \
    "$tagged" --version \
    >"$out/${slug}.version" 2>"$out/${slug}.version.err"
  rc=$?
  set -e
  echo "$rc" >"$out/${slug}.version.rc"
}

# TODO: 第 3 项合并后收紧。cpu_guard 只接受 reexec（去掉 true），
# simd 只接受 software（去掉 avx128+nosimd）。反向对照只设置 MFILE_CPU_GUARD=reexec。
check_version() {
  local slug="$1" kind="$2"
  local rc simd guard go_line
  rc=$(cat "$out/${slug}.version.rc")
  if [[ "$rc" != "0" ]]; then
    echo "FAIL ${slug} --version exited ${rc}"
    return 1
  fi
  if grep -Eiq 'SIGILL|illegal instruction' "$out/${slug}.version.err"; then
    echo "FAIL ${slug} --version stderr contains SIGILL"
    return 1
  fi
  simd=$(field simd "$out/${slug}.version")
  guard=$(field cpu_guard "$out/${slug}.version")
  go_line=$(awk '/^go/{print; exit}' "$out/${slug}.version")
  if [[ "$go_line" != *simd* ]]; then
    echo "FAIL ${slug} Go version line ${go_line} is not a GOEXPERIMENT=simd build"
    return 1
  fi
  if ! grep -q '^GOAMD64=v1$' "$out/${slug}.version"; then
    echo "FAIL ${slug} --version is not GOAMD64=v1"
    return 1
  fi
  case "$kind" in
    guard)
      case "$guard" in
        true | reexec) ;;
        *)
          echo "FAIL ${slug} cpu_guard=${guard} (want true or reexec)"
          return 1
          ;;
      esac
      case "$simd" in
        software | avx128+nosimd) ;;
        *)
          echo "FAIL ${slug} simd=${simd} (want software or avx128+nosimd)"
          return 1
          ;;
      esac
      ;;
    plain)
      if [[ "$guard" != "false" ]]; then
        echo "FAIL ${slug} cpu_guard=${guard} (want false)"
        return 1
      fi
      case "$simd" in
        software | avx128+nosimd) ;;
        *)
          echo "FAIL ${slug} simd=${simd} (want software or avx128+nosimd, not avx2)"
          return 1
          ;;
      esac
      ;;
    avx2)
      if [[ "$guard" != "false" ]]; then
        echo "FAIL ${slug} cpu_guard=${guard} (want false)"
        return 1
      fi
      if [[ "$simd" != "avx2" ]]; then
        echo "FAIL ${slug} simd=${simd} (want avx2)"
        return 1
      fi
      ;;
    *)
      die "unknown version kind ${kind}"
      ;;
  esac
  echo "PASS ${slug} version cpu_guard=${guard} simd=${simd}"
}

# Direct qemu, untagged binary. With the guard marker set, SandyBridge must
# SIGILL inside this qemu. Without the marker, syscall.Exec would leave qemu
# and the child would succeed on the host, which would not prove anything.
# Today's marker is 1. After 第 3 项 it is reexec. Whichever value actually
# suppresses the re-exec must die with SIGILL.
run_negative() {
  local marker="$1" rc
  set +e
  timeout -k 20 180 \
    env MFILE_CPU_GUARD="$marker" \
    "$qemu" -cpu SandyBridge "$bin" --json cmd/mfile/main.go \
    >"$out/sandybridge-noguard-${marker}.stdout" \
    2>"$out/sandybridge-noguard-${marker}.err"
  rc=$?
  set -e
  echo "$rc" >"$out/sandybridge-noguard-${marker}.rc"
  if [[ "$rc" -ne 0 ]] && grep -Eiq 'SIGILL|illegal instruction' "$out/sandybridge-noguard-${marker}.err"; then
    echo "PASS negative MFILE_CPU_GUARD=${marker} SIGILL rc=${rc}"
    return 0
  fi
  echo "negative MFILE_CPU_GUARD=${marker} is not the active skip marker (rc=${rc}, no SIGILL)"
  return 1
}

run_infer() {
  local cpu="$1" slug="$2"
  local start end rc
  start=$(date +%s)
  set +e
  timeout -k 30 20m \
    env -u MFILE_CPU_GUARD -u GODEBUG -u GOMLX_GO_SIMD_AVX2 -u GOMLX_GO_SIMD_AVX512 \
    QEMU_CPU="$cpu" \
    "$tagged" --json "${sample_files[@]}" \
    >"$out/${slug}.json" 2>"$out/${slug}.err"
  rc=$?
  set -e
  end=$(date +%s)
  echo "$rc" >"$out/${slug}.rc"
  echo $((end - start)) >"$out/${slug}.seconds"
  note "${slug} inference rc=${rc} seconds=$((end - start))"
}

check_infer() {
  local slug="$1" ref="$2"
  local rc seconds
  rc=$(cat "$out/${slug}.rc")
  seconds=$(cat "$out/${slug}.seconds")
  if [[ "$rc" == "124" ]]; then
    echo "FAIL ${slug} inference timed out after ${seconds}s"
    return 1
  fi
  if [[ "$rc" != "0" ]]; then
    echo "FAIL ${slug} inference exited ${rc} after ${seconds}s"
    return 1
  fi
  if grep -Eiq 'SIGILL|illegal instruction' "$out/${slug}.err"; then
    echo "FAIL ${slug} inference stderr contains SIGILL"
    return 1
  fi
  if ! cmp -s "$out/${ref}.json" "$out/${slug}.json"; then
    echo "FAIL ${slug} json differs from ${ref} (${seconds}s)"
    diff -u "$out/${ref}.json" "$out/${slug}.json" | head -n 80 >&2 || true
    return 1
  fi
  echo "PASS ${slug} json matches ${ref} seconds=${seconds}"
}

mkdir -p "$out"
: >"$out/summary.txt"

for f in "${sample_files[@]}"; do
  [[ -f "$f" ]] || die "sample file missing: $f"
done
[[ "$(uname -m)" == "x86_64" ]] || die "this matrix runs on x86_64, not $(uname -m)"

install_qemu
build_mfile
tag_binary
register_binfmt
assert_untagged_stays_native
assert_host_avx2
write_references

# cpu|slug|version-kind|reference
models=(
  "SandyBridge|sandybridge|guard|ref-cpu-all-off"
  "Haswell,-fma|haswell-nofma|guard|ref-cpu-all-off"
  "Westmere|westmere|plain|ref-cpu-all-off"
  "Haswell|haswell|avx2|ref-avx2"
)

for spec in "${models[@]}"; do
  IFS='|' read -r cpu slug kind _ref <<<"$spec"
  run_version "$cpu" "$slug"
  if ! check_version "$slug" "$kind"; then
    fail=1
  fi
done

neg_ok=0
if run_negative 1; then
  neg_ok=1
fi
if run_negative reexec; then
  neg_ok=1
fi
if [[ "$neg_ok" != "1" ]]; then
  echo "FAIL SandyBridge negative control: neither MFILE_CPU_GUARD=1 nor =reexec SIGILL'd under qemu -cpu SandyBridge"
  fail=1
fi

if [[ "$fail" != "0" ]]; then
  note "version or negative control failed; skipping inference"
  {
    echo "qemu-matrix: FAILED before inference"
    echo "qemu: $(cat "$out/qemu-version.txt")"
    echo "source: $(cat "$out/qemu-source.txt")"
    echo "sha256: $(cat "$out/qemu-sha256.txt")"
  } | tee -a "$out/summary.txt"
  exit 1
fi

pids=()
for spec in "${models[@]}"; do
  IFS='|' read -r cpu slug _kind _ref <<<"$spec"
  run_infer "$cpu" "$slug" &
  pids+=("$!")
done
for pid in "${pids[@]}"; do
  wait "$pid" || die "inference worker ${pid} failed to record a result"
done

for spec in "${models[@]}"; do
  IFS='|' read -r _cpu slug _kind ref <<<"$spec"
  if ! check_infer "$slug" "$ref"; then
    fail=1
  fi
done

{
  echo "qemu: $(cat "$out/qemu-version.txt")"
  echo "source: $(cat "$out/qemu-source.txt")"
  echo "sha256: $(cat "$out/qemu-sha256.txt")"
  echo "sample_files: ${#sample_files[@]}"
  printf '%-16s %-6s %-8s %-8s %s\n' "model" "result" "seconds" "cpu_guard" "simd"
  for spec in "${models[@]}"; do
    IFS='|' read -r _cpu slug _kind ref <<<"$spec"
    simd=$(field simd "$out/${slug}.version")
    guard=$(field cpu_guard "$out/${slug}.version")
    seconds=$(cat "$out/${slug}.seconds")
    result="FAIL"
    if [[ "$(cat "$out/${slug}.rc")" == "0" ]] && cmp -s "$out/${ref}.json" "$out/${slug}.json"; then
      result="PASS"
    fi
    printf '%-16s %-6s %-8s %-8s %s\n' "$slug" "$result" "$seconds" "$guard" "$simd"
  done
  if [[ "$fail" == "0" ]]; then
    echo "negative: SandyBridge without the guard SIGILL'd"
    echo "qemu-matrix: PASS"
  else
    echo "qemu-matrix: FAIL"
  fi
} | tee "$out/summary.txt"

append_summary "## qemu-cpu-matrix"
append_summary ""
append_summary '```'
append_summary "$(cat "$out/summary.txt")"
append_summary '```'

if [[ "$fail" != "0" ]]; then
  exit 1
fi
