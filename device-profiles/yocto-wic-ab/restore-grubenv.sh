#!/usr/bin/env bash
# device-profiles/yocto-wic-ab/restore-grubenv.sh
#
# Rollback variant of update-grubenv.sh: switches `boot_part` back to
# `previous_part`. Invoked from the manifest's rollback_steps[] when a
# deployment step fails (FR-04 / FR-05).

set -euo pipefail

cleanup() {
    local rc=$?
    exit "$rc"
}
trap cleanup EXIT

GRUBENV="${OTA_GRUBENV_PATH:-/boot/grub/grubenv}"

prev="$(grub-editenv "$GRUBENV" list 2>/dev/null | awk -F= '$1=="previous_part"{print $2}' || true)"
if [[ -z "${prev:-}" ]]; then
    echo "no previous_part recorded; nothing to restore" >&2
    exit 0
fi

grub-editenv "$GRUBENV" set boot_part="$prev"
grub-editenv "$GRUBENV" unset boot_count
grub-editenv "$GRUBENV" set boot_success=1
echo "grubenv restored to boot_part=$prev"
