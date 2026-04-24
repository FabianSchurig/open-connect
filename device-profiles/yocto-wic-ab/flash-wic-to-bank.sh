#!/usr/bin/env bash
# device-profiles/yocto-wic-ab/flash-wic-to-bank.sh
#
# Flash a Yocto-style .wic.zst into the inactive A/B bank using bmaptool.
# Distributed as part of the `yocto-wic-ab` device profile (Epic O).
#
# Conventions enforced by the FR-30 lint:
#   - set -euo pipefail
#   - trap cleanup EXIT
#   - reads parameters ONLY from OTA_* environment variables passed by the agent
#   - never calls reboot/shutdown (only the REBOOT primitive may reboot)
#   - never writes to /var/log
#   - target-safety interlock (FR-30 §5): refuses to flash the active root
#
# Required env:
#   OTA_WIC_PATH       absolute path to the downloaded .wic / .wic.zst file
#   OTA_BMAP_PATH      absolute path to the matching .bmap file
#   OTA_TARGET_BANK    "A" or "B" — the inactive bank to flash
# Optional env:
#   OTA_BANK_A_DEV     block device for bank A (default /dev/sda2)
#   OTA_BANK_B_DEV     block device for bank B (default /dev/sda3)

set -euo pipefail

cleanup() {
    local rc=$?
    # Best-effort: nothing persistent to remove; bmaptool writes directly.
    exit "$rc"
}
trap cleanup EXIT

: "${OTA_WIC_PATH:?OTA_WIC_PATH is required}"
: "${OTA_BMAP_PATH:?OTA_BMAP_PATH is required}"
: "${OTA_TARGET_BANK:?OTA_TARGET_BANK is required (A or B)}"

OTA_BANK_A_DEV="${OTA_BANK_A_DEV:-/dev/sda2}"
OTA_BANK_B_DEV="${OTA_BANK_B_DEV:-/dev/sda3}"

case "$OTA_TARGET_BANK" in
    A) target_dev="$OTA_BANK_A_DEV" ;;
    B) target_dev="$OTA_BANK_B_DEV" ;;
    *) echo "OTA_TARGET_BANK must be A or B (got '$OTA_TARGET_BANK')" >&2; exit 64 ;;
esac

# Target-safety interlock (FR-30 §5): refuse to overwrite the currently-mounted root.
current_root="$(findmnt -no SOURCE / || true)"
if [[ -n "$current_root" && "$current_root" == "$target_dev" ]]; then
    echo "REFUSING to flash $target_dev: it is the active root partition" >&2
    exit 65
fi

# Sanity: artefacts must exist.
[[ -s "$OTA_WIC_PATH" ]]  || { echo "missing wic at $OTA_WIC_PATH" >&2; exit 66; }
[[ -s "$OTA_BMAP_PATH" ]] || { echo "missing bmap at $OTA_BMAP_PATH" >&2; exit 66; }

echo "flashing $OTA_WIC_PATH (bmap=$OTA_BMAP_PATH) -> $target_dev"
bmaptool copy --bmap "$OTA_BMAP_PATH" "$OTA_WIC_PATH" "$target_dev"
sync
echo "flash to bank $OTA_TARGET_BANK complete"
