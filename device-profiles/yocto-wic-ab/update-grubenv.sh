#!/usr/bin/env bash
# device-profiles/yocto-wic-ab/update-grubenv.sh
#
# Set the next boot to the freshly-flashed bank, with a boot-count fallback
# that triggers an automatic revert if the new image fails to come up cleanly
# (FR-04 / FR-06).
#
# Required env:
#   OTA_BOOT_PART     "A" or "B"  — the bank that should be booted next
#   OTA_BOOT_COUNT    integer    — max number of boot attempts before revert

set -euo pipefail

cleanup() {
    local rc=$?
    exit "$rc"
}
trap cleanup EXIT

: "${OTA_BOOT_PART:?OTA_BOOT_PART is required (A or B)}"
: "${OTA_BOOT_COUNT:?OTA_BOOT_COUNT is required}"

GRUBENV="${OTA_GRUBENV_PATH:-/boot/grub/grubenv}"

# Capture the previous bank for restore-grubenv.sh.
prev="$(grub-editenv "$GRUBENV" list 2>/dev/null | awk -F= '$1=="boot_part"{print $2}' || true)"
prev="${prev:-A}"

grub-editenv "$GRUBENV" set boot_part="$OTA_BOOT_PART"
grub-editenv "$GRUBENV" set previous_part="$prev"
grub-editenv "$GRUBENV" set boot_count="$OTA_BOOT_COUNT"
grub-editenv "$GRUBENV" set boot_success=0

echo "grubenv updated: boot_part=$OTA_BOOT_PART previous_part=$prev boot_count=$OTA_BOOT_COUNT boot_success=0"
