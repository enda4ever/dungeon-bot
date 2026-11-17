#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

KEEP=${BACKUP_KEEP:-1}
OUT=backups
SRC=data/state.json

# The privacy policy promises a crawler's user ID is deleted as soon as they
# lose the role, or 24 hours after their escape. A lasting copy of
# crawler_since would outlive that, so it never reaches the backup. If the
# state ever gains another field holding user IDs, add it here too: the kept
# keys are printed below so a new one is noticeable.
DROP='crawler_since'

[ -f "$SRC" ] || { echo "no $SRC to back up, has the bot ever run?" >&2; exit 1; }

mkdir -p "$OUT"
DEST="$OUT/state-$(date +%Y%m%d-%H%M%S).json"

python3 - "$SRC" "$DEST" "$DROP" <<'PY'
import json, sys

src, dest, drop = sys.argv[1], sys.argv[2], sys.argv[3].split(',')
with open(src, encoding='utf-8') as f:
    state = json.load(f)

removed = [k for k in drop if k in state]
for k in removed:
    del state[k]

with open(dest, 'w', encoding='utf-8', newline='\n') as f:
    json.dump(state, f, indent='\t')
    f.write('\n')

print('dropped:', ', '.join(removed) if removed else '(nothing)')
print('kept:   ', ', '.join(sorted(state)))
PY

ls -1t "$OUT"/state-*.json 2>/dev/null | tail -n +$((KEEP + 1)) | while read -r old; do
	rm -f "$old"
done

echo "$DEST"
