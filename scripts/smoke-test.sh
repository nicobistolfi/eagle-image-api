#!/usr/bin/env bash
#
# Smoke test a deployed Eagle Image API.
#
# Checks that the deployment answers on its health endpoint, transforms a real
# image, negotiates WebP from the Accept header, and rejects bad requests. It
# is deliberately shallow: it proves the stack is wired together and serving,
# not that every transformation is correct — the Go test suite covers that.
#
# Usage:
#   scripts/smoke-test.sh <base-url> [api-endpoint]
#
# Example:
#   scripts/smoke-test.sh https://abc123.execute-api.us-west-1.amazonaws.com/dev
#
set -euo pipefail

BASE_URL="${1:-}"
API_ENDPOINT="${2:-/api/v1/image}"
TEST_IMAGE="${TEST_IMAGE:-https://eagle-image-test.s3.us-west-1.amazonaws.com/public/eagle-2.jpg}"

if [ -z "$BASE_URL" ]; then
  echo "usage: $0 <base-url> [api-endpoint]" >&2
  exit 2
fi

BASE_URL="${BASE_URL%/}"
IMAGE_URL="${BASE_URL}${API_ENDPOINT}"
# CloudFront and API Gateway can serve a stale or still-propagating deployment
# for a few seconds after the stack updates, so each check gets a few tries.
RETRIES="${RETRIES:-5}"
RETRY_DELAY="${RETRY_DELAY:-6}"

failures=0

# encoded_test_image URL-encodes the source image so it survives as a query
# parameter value.
encoded_test_image() {
  jq -rn --arg u "$TEST_IMAGE" '$u|@uri'
}

# fetch performs a request and writes the body to $body_file, echoing
# "<status> <content-type>" on stdout.
fetch() {
  local url="$1" accept="${2:-}"
  local args=(--silent --show-error --location --max-time 30
              --output "$body_file" --write-out '%{http_code} %{content_type}')
  if [ -n "$accept" ]; then
    args+=(--header "Accept: $accept")
  fi
  curl "${args[@]}" "$url"
}

# check runs one assertion, retrying while the response is not what we expect.
# Arguments: name, url, accept header, expected status, expected content-type
# substring (empty to skip the content-type assertion).
check() {
  local name="$1" url="$2" accept="$3" want_status="$4" want_type="${5:-}"
  local attempt=1 status type result

  while [ "$attempt" -le "$RETRIES" ]; do
    result="$(fetch "$url" "$accept" || true)"
    status="${result%% *}"
    type="${result#* }"

    if [ "$status" = "$want_status" ] &&
       { [ -z "$want_type" ] || [[ "$type" == *"$want_type"* ]]; }; then
      echo "PASS  $name (status $status${want_type:+, $type})"
      return 0
    fi

    if [ "$attempt" -lt "$RETRIES" ]; then
      echo "      $name: got status ${status:-none}${type:+ / $type}, retrying in ${RETRY_DELAY}s ($attempt/$RETRIES)"
      sleep "$RETRY_DELAY"
    fi
    attempt=$((attempt + 1))
  done

  echo "FAIL  $name"
  echo "      url:      $url"
  echo "      expected: status $want_status${want_type:+, content-type containing $want_type}"
  echo "      actual:   status ${status:-none}${type:+, content-type $type}"
  echo "      body:     $(head -c 300 "$body_file" 2>/dev/null || true)"
  failures=$((failures + 1))
  return 0
}

body_file="$(mktemp)"
trap 'rm -f "$body_file"' EXIT

encoded="$(encoded_test_image)"

echo "Smoke testing $BASE_URL (endpoint $API_ENDPOINT)"
echo

check "health endpoint" \
  "${BASE_URL}/health" "" "200"

check "transforms an image" \
  "${IMAGE_URL}?width=200&url=${encoded}" "" "200" "image/"

check "resizes and crops" \
  "${IMAGE_URL}?width=100&height=100&fit=cover&url=${encoded}" "" "200" "image/"

check "negotiates webp from Accept" \
  "${IMAGE_URL}?width=200&url=${encoded}" "image/webp,*/*" "200" "image/webp"

check "rejects a request with no url" \
  "${IMAGE_URL}" "" "400"

check "404s an unknown path" \
  "${BASE_URL}/not-a-real-path" "" "404"

echo
if [ "$failures" -gt 0 ]; then
  echo "$failures check(s) failed."
  exit 1
fi

echo "All checks passed."
