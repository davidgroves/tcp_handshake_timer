#!/bin/sh
set -eu

VERSION="${SWAGGER_UI_VERSION:-5.18.3}"
ROOT="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
DIR="${ROOT}/internal/api/static/swagger-ui"
CDN="https://cdn.jsdelivr.net/npm/swagger-ui-dist@${VERSION}"

mkdir -p "${DIR}"

for file in swagger-ui.css swagger-ui-bundle.js; do
	curl -fsSL -o "${DIR}/${file}" "${CDN}/${file}"
done

echo "Fetched swagger-ui-dist ${VERSION} into ${DIR}"
