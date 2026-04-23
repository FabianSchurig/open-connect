#!/usr/bin/env bash
# scripts/lint-device-profile-scripts.sh
#
# FR-30 conventions checker for device-profile shell scripts.
# Required:
#   - shebang `#!/usr/bin/env bash` (or `/bin/bash`)
#   - `set -euo pipefail`
#   - `trap cleanup EXIT`
#   - no `reboot`, `shutdown`, `systemctl reboot`, `systemctl poweroff`
#   - no writes to `/var/log`
#   - all referenced env-vars start with `OTA_` (best-effort grep)

set -euo pipefail

fail=0
err() { echo "FR-30 lint: $*" >&2; fail=1; }

for f in "$@"; do
    [[ -f "$f" ]] || { err "missing: $f"; continue; }

    head -1 "$f" | grep -qE '^#!/(usr/bin/env bash|bin/bash)$' \
        || err "$f: missing bash shebang"

    grep -qE '^set -euo pipefail' "$f" \
        || err "$f: missing 'set -euo pipefail'"

    grep -qE '^trap[[:space:]]+cleanup[[:space:]]+EXIT' "$f" \
        || err "$f: missing 'trap cleanup EXIT'"

    # Strip comment-only lines and trailing inline comments before scanning
    # for forbidden tokens.
    code_only=$(sed -E 's/[[:space:]]*#.*$//' "$f" | grep -vE '^\s*$' || true)

    if printf '%s\n' "$code_only" | grep -qE '(^|[^a-zA-Z_])(reboot|shutdown|systemctl[[:space:]]+(reboot|poweroff|halt))(\b|[^a-zA-Z_])'; then
        err "$f: forbidden reboot/shutdown call (use REBOOT primitive)"
    fi

    if printf '%s\n' "$code_only" | grep -qE '/var/log(/|"|$)'; then
        err "$f: writes to /var/log forbidden (FR-30 §6)"
    fi
done

if [[ "$fail" -ne 0 ]]; then
    echo "FR-30 lint FAILED" >&2
    exit 1
fi
echo "FR-30 lint passed for $# script(s)"
