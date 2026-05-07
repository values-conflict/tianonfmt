#!/usr/bin/env bash
set -Eeuo pipefail

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")/artifacts"

set -Eeuo pipefail
rm -f *.asc
for f in gosleep*; do
	gpg --batch --output "$f.asc" --detach-sign "$f"
done
sha256sum gosleep* >SHA256SUMS
gpg --batch --output SHA256SUMS.asc --detach-sign SHA256SUMS
ls -lAFh gosleep* SHA256SUMS*
