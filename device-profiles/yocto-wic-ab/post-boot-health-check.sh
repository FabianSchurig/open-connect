#!/usr/bin/env bash
# device-profiles/yocto-wic-ab/post-boot-health-check.sh
#
# Minimal post-boot health probe (FR-06). Runs after the new bank has
# rebooted; on success clears boot_count and sets boot_success=1, sealing
# the deployment. On failure the bootloader's boot-count limit triggers
# auto-revert on the next reboot.

set -euo pipefail

cleanup() {
    local rc=$?
    exit "$rc"
}
trap cleanup EXIT

GRUBENV="${OTA_GRUBENV_PATH:-/boot/grub/grubenv}"

# 1. systemd default target reached.
if ! systemctl is-system-running --wait >/dev/null 2>&1; then
    state="$(systemctl is-system-running 2>/dev/null || true)"
    if [[ "$state" != "running" && "$state" != "degraded" ]]; then
        echo "systemd not running (state=$state)" >&2
        exit 70
    fi
fi

# 2. Open-Connect agent is up.
if ! systemctl is-active --quiet open-connect-agent.service; then
    echo "open-connect-agent.service is not active" >&2
    exit 71
fi

grub-editenv "$GRUBENV" set boot_success=1
grub-editenv "$GRUBENV" unset boot_count
echo "post-boot health check OK"
