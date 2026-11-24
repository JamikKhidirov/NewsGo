#!/usr/bin/env bash
# Run a GoNews k6 scenario. Usage: ./run.sh smoke|read|mixed|stress|spike
set -euo pipefail

SCENARIO="${1:-smoke}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is not installed. See https://k6.io/docs/get-started/installation/" >&2
  exit 1
fi

echo "GoNews load test: ${SCENARIO} against ${BASE_URL}"
BASE_URL="${BASE_URL}" k6 run "${DIR}/${SCENARIO}.js" "${@:2}"
