#!/usr/bin/env bash

set -euo pipefail
umask 077

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
cd "$repository_root"

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  echo "error: source hygiene must run inside a Git worktree" >&2
  exit 2
}

for command_name in awk git grep mktemp rm tr wc; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "error: required command is unavailable: $command_name" >&2
    exit 2
  }
done

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  echo "error: sha256sum or shasum is required" >&2
  exit 2
}

approved_rom_fixture() {
  case "$1" in
    'testdata/esde/roms/gba/示例汉化版.gba')
      printf '%s\n' '37:44a4658b759ba6fd929647d1f742bb3aa23c621b52137edd88fd699d6b21f299'
      ;;
    'testdata/neutral/gba/recovery.gba')
      printf '%s\n' '82:3bc942f13efc3515fb59d6dd385b7ad3781e1116a368dfccd505f83935b3b45c'
      ;;
    'testdata/pegasus/gba/Advance Wars (USA).gba')
      printf '%s\n' '45:372163c67e18814c80dba2461f91f033ec68f6e8c967ffa76ff587b21c788124'
      ;;
    'testdata/pegasus/gba/Multi Demo Disc 1.gba')
      printf '%s\n' '39:fb21b48192ab4e5bfc27c79ddf7c17f5ae374e8a79592ebbd1183186e0026f3c'
      ;;
    'testdata/pegasus/gba/Multi Demo Disc 2.gba')
      printf '%s\n' '39:15197cdea1bef120ef3956c8e7309904529d0400d5a9063940655d44e7010b3f'
      ;;
    'testdata/portable-runtime-v2/gba/runtime-v2.gba')
      printf '%s\n' '24:2446c56e0d42853712e2e794b5cd42e58344fee5b1e3604519ecb79b998785bd'
      ;;
    'testdata/portable-standalone-v2/psp/standalone-v2.iso')
      printf '%s\n' '27:73ff0956416b04b11e8f24390c7ee4dfea4822a2849119532f2be2655502911a'
      ;;
    'testdata/portable-v6/roms/demo.opk')
      printf '%s\n' '24:f419f5465bbdec998b6b38dc1e3e33d044ea3c2245ed0ef793d8f94b74b61581'
      ;;
    'testdata/runtimehint/nds/Runtime Hint Demo.nds')
      printf '%s\n' '30:1dea34e35e7379ee59f5c205c995f90040e6ec86297fb22ea69f1e2646c7b693'
      ;;
    'testdata/runtime-batch/gba/Batch One.gba')
      printf '%s\n' '41:92e7273e4e1379e2d91cf9611bcea7954a3571209bc657017ae67136bac95d6b'
      ;;
    'testdata/runtime-batch/gba/Batch Two.gba')
      printf '%s\n' '41:bf4cc5d4addb6324bcb965cc948a1dd483d7ab68b81e97d6cb9b4d67940d2ff7'
      ;;
    *)
      return 1
      ;;
  esac
}

approved_media_fixture() {
  case "$1" in
    'testdata/esde/media/gba/example-cover.svg')
      printf '%s\n' '583:e2ca0fcc85b3420bb3c39e3a97031a77262240d76337f4a5c2de3a330532aead'
      ;;
    'testdata/pegasus/gba/media/advance-cover.svg')
      printf '%s\n' '559:3eb49863546bd1e169a75842b105383c0bba8daf495f566f13480c634fea693f'
      ;;
    *)
      return 1
      ;;
  esac
}

approved_brand_asset() {
  case "$1" in
    'internal/server/web/assets/favicon.svg')
      printf '%s\n' '444:a7a3681b8367bd930fff20115c2975e79adc68e64c7a76d8fa4cd1958ca4386f'
      ;;
    'internal/server/web/assets/kino-logo.svg')
      printf '%s\n' '624:a43499adea5bc39748a9eeb7ebf3854c5b16c4c6b70a63a74edae7ea09c61e4c'
      ;;
    'internal/server/web/assets/kino-mark.svg')
      printf '%s\n' '444:a7a3681b8367bd930fff20115c2975e79adc68e64c7a76d8fa4cd1958ca4386f'
      ;;
    *)
      return 1
      ;;
  esac
}

approved_rom_like_source_document() {
  case "$1" in
    'testdata/psp-homebrew/README.md')
      printf '%s\n' '479:04ce077790650002f6e05b5ddda20fca650f94b037010d6b256a9716303f58bf'
      ;;
    *)
      return 1
      ;;
  esac
}

candidate_list=$(mktemp "${TMPDIR:-/tmp}/kino-source-files.XXXXXX")
cleanup() {
  rm -f -- "$candidate_list"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

git -c core.quotepath=false ls-files -z --cached --others --exclude-standard > "$candidate_list"

max_source_bytes=$((8 * 1024 * 1024))
file_count=0
byte_count=0
fixture_count=0
media_fixture_count=0
brand_asset_count=0
rom_like_source_document_count=0
failure_count=0
mac_user_root='/''Users/'
mac_private_temp='/var/''folders/'
private_key_marker='BEGIN ''PRIVATE KEY'
vendor_secret_pattern='(gh''[pousr]_[A-Za-z0-9]{20,}|AK''IA[0-9A-Z]{16}|xox''[aboprs]-[A-Za-z0-9-]{20,})'

report_failure() {
  printf 'error: source hygiene rejected %s: %s\n' "$1" "$2" >&2
  failure_count=$((failure_count + 1))
}

while IFS= read -r -d '' path; do
  if [[ ! -e "$path" && ! -L "$path" ]]; then
    continue
  fi
  file_count=$((file_count + 1))

  if [[ -L "$path" ]]; then
    report_failure "$path" 'symbolic links are not accepted in the release source snapshot'
    continue
  fi
  if [[ ! -f "$path" ]]; then
    report_failure "$path" 'only regular files may enter the release source snapshot'
    continue
  fi

  size=$(wc -c < "$path" | tr -d '[:space:]')
  byte_count=$((byte_count + size))
  if ((size > max_source_bytes)); then
    report_failure "$path" "file exceeds the ${max_source_bytes}-byte source limit"
  fi

  lower_path=$(printf '%s' "$path" | tr '[:upper:]' '[:lower:]')
  case "$lower_path" in
    '.env.example'|'.env.nas.example'|'.env.ghcr.example') ;;
    '.env'|'*/.env'|'.env.'*|'*/.env.'*|*.jks|*.keystore|*.p12|*.pfx|*.mobileprovision|*/local.properties|local.properties|*/google-services.json|google-services.json|*/id_rsa|id_rsa|*/id_ed25519|id_ed25519)
      report_failure "$path" 'credential or machine-local configuration filename'
      ;;
  esac

  case "$lower_path" in
    *.db|*.db-shm|*.db-wal|*.sqlite|*.sqlite3|*.srm|*.sav|*.state|*.state[0-9]*|*.mcr|*.ps2|*.dsv|*.gci|*.vmu)
      report_failure "$path" 'database or save-state artifact'
      ;;
  esac

  case "$lower_path" in
    *.3ds|*.3dsx|*.cia|*.xci|*.nsp|*.iso|*.cso|*.chd|*.rvz|*.wbfs|*.wad|*.pbp|*.opk|*.nes|*.fds|*.sfc|*.smc|*.gba|*.gbc|*.gb|*.nds|*.z64|*.n64|*.v64|testdata/*.md|*.gen|*.smd|*.sms|*.gg|*.pce|*.cue|*.bin|*.rom|*.img|*.zip|*.7z|*.rar|*.tar|*.tgz|*.gz|*.xz|*.tkzlm|*.7z.[0-9][0-9][0-9]|*.zip.[0-9][0-9][0-9]|*.part[0-9]*.rar|*.r[0-9][0-9])
      expected=$(approved_rom_fixture "$path" || true)
      if [[ -z "$expected" ]]; then
        expected=$(approved_rom_like_source_document "$path" || true)
        if [[ -z "$expected" ]]; then
          report_failure "$path" 'ROM-like content is not an approved synthetic fixture or locked source document'
        else
          expected_size=${expected%%:*}
          expected_sha=${expected#*:}
          actual_sha=$(sha256_file "$path")
          if [[ "$size" != "$expected_size" || "$actual_sha" != "$expected_sha" ]]; then
            report_failure "$path" 'approved ROM-like source document identity drifted'
          else
            rom_like_source_document_count=$((rom_like_source_document_count + 1))
          fi
        fi
      else
        expected_size=${expected%%:*}
        expected_sha=${expected#*:}
        actual_sha=$(sha256_file "$path")
        if [[ "$size" != "$expected_size" || "$actual_sha" != "$expected_sha" ]]; then
          report_failure "$path" 'approved synthetic fixture identity drifted'
        else
          fixture_count=$((fixture_count + 1))
        fi
      fi
      ;;
  esac

  case "$lower_path" in
    *.png|*.jpg|*.jpeg|*.gif|*.webp|*.avif|*.bmp|*.ico|*.svg)
      expected=$(approved_brand_asset "$path" || true)
      if [[ -n "$expected" ]]; then
        expected_size=${expected%%:*}
        expected_sha=${expected#*:}
        actual_sha=$(sha256_file "$path")
        if [[ "$size" != "$expected_size" || "$actual_sha" != "$expected_sha" ]]; then
          report_failure "$path" 'approved brand asset identity drifted'
        else
          brand_asset_count=$((brand_asset_count + 1))
        fi
      else
        expected=$(approved_media_fixture "$path" || true)
        if [[ -z "$expected" ]]; then
          report_failure "$path" 'media content is not an approved synthetic fixture'
        else
          expected_size=${expected%%:*}
          expected_sha=${expected#*:}
          actual_sha=$(sha256_file "$path")
          if [[ "$size" != "$expected_size" || "$actual_sha" != "$expected_sha" ]]; then
            report_failure "$path" 'approved synthetic media fixture identity drifted'
          else
            media_fixture_count=$((media_fixture_count + 1))
          fi
        fi
      fi
      ;;
  esac

  if LC_ALL=C grep -Iq . "$path"; then
    if grep -Fq "$mac_user_root" "$path"; then
      report_failure "$path" 'machine-specific macOS user path'
    fi
    if grep -Fq "$mac_private_temp" "$path"; then
      report_failure "$path" 'machine-specific macOS temporary path'
    fi
    if grep -Fq "$private_key_marker" "$path"; then
      report_failure "$path" 'private key material marker'
    fi
    if grep -Eq "$vendor_secret_pattern" "$path"; then
      report_failure "$path" 'provider credential signature'
    fi
  fi
done < "$candidate_list"

if ((failure_count > 0)); then
  printf 'source_hygiene=failed files=%d failures=%d\n' "$file_count" "$failure_count" >&2
  exit 1
fi

printf 'source_hygiene=passed files=%d bytes=%d synthetic_rom_fixtures=%d synthetic_media_fixtures=%d brand_assets=%d locked_rom_source_documents=%d\n' "$file_count" "$byte_count" "$fixture_count" "$media_fixture_count" "$brand_asset_count" "$rom_like_source_document_count"
