#!/usr/bin/env bash
# Fetches serial numbers via Redfish through a kubectl proxy pod and updates Netbox.
#
# Usage:
#   NETBOX_TOKEN=<token> ./hack/sync-serials.sh [--cluster <id>] [--site <slug>] [--dry-run] [--overwrite]
#
# Example (cluster cc-a0-qa-de-8):
#   NETBOX_TOKEN=<token> ./hack/sync-serials.sh --cluster 2996 --dry-run
#
# The proxy pod runs Alpine in the cluster so BMC IPs (oob_ip) are reachable.
# No auth is needed to read from Netbox or Redfish in this environment.

set -euo pipefail

k() { kubectl "$@"; }

NETBOX_URL="${NETBOX_URL:-https://netbox.global.cloud.sap}"
NETBOX_TOKEN="${NETBOX_TOKEN:-}"
BMC_USER="${BMC_USER:-root}"
BMC_PASSWORD="${BMC_PASSWORD:-}"
CLUSTER_ID=""
SITE=""
DRY_RUN=false
OVERWRITE=false
NAMESPACE="${NAMESPACE:-default}"
PROXY_POD="redfish-probe-$$"

while [[ $# -gt 0 ]]; do
  case $1 in
    --cluster)   CLUSTER_ID="$2"; shift 2 ;;
    --site)      SITE="$2";        shift 2 ;;
    --dry-run)   DRY_RUN=true;     shift   ;;
    --overwrite) OVERWRITE=true;   shift   ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$NETBOX_TOKEN" ]]; then
  echo "ERROR: NETBOX_TOKEN is required for writing to Netbox" >&2
  exit 1
fi

if [[ -z "$BMC_PASSWORD" ]]; then
  read -rsp "BMC password for user '${BMC_USER}': " BMC_PASSWORD
  echo
fi

# --- helpers -----------------------------------------------------------------

netbox_get() {
  curl -fsSL \
    -H "Authorization: Token ${NETBOX_TOKEN}" \
    -H "Accept: application/json" \
    "${NETBOX_URL}${1}"
}

netbox_patch() {
  curl -fsSL -X PATCH \
    -H "Authorization: Token ${NETBOX_TOKEN}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d "$2" \
    "${NETBOX_URL}${1}"
}

proxy_wget() {
  local url="$1"; shift
  # busybox wget doesn't support --user/--password, embed creds in URL if set
  if [[ -n "$BMC_USER" && -n "$BMC_PASSWORD" ]]; then
    url="${url/https:\/\//https://${BMC_USER}:${BMC_PASSWORD}@}"
  fi
  k exec -n "$NAMESPACE" "$PROXY_POD" -- wget -qO- --no-check-certificate -T 10 "$url"
}

fetch_serial() {
  local bmc_ip="$1"
  local serial response

  # Dell iDRAC exposes ServiceTag unauthenticated at /redfish/v1
  echo "    [redfish] GET https://${bmc_ip}/redfish/v1" >&2
  response=$(proxy_wget "https://${bmc_ip}/redfish/v1" 2>&1) || true
  echo "    [redfish] response: ${response}" >&2
  serial=$(echo "$response" | grep -o '"ServiceTag":"[^"]*"' | head -1 | cut -d'"' -f4)

  # Fall back to authenticated /Systems/1 (HPE iLO, Lenovo, etc.)
  if [[ -z "$serial" && -n "$BMC_PASSWORD" ]]; then
    echo "    [redfish] GET https://${bmc_ip}/redfish/v1/Systems/1" >&2
    response=$(proxy_wget "https://${bmc_ip}/redfish/v1/Systems/1" 2>&1) || true
    echo "    [redfish] response: ${response}" >&2
    serial=$(echo "$response" | grep -o '"SerialNumber":"[^"]*"' | head -1 | cut -d'"' -f4)
  fi

  echo "$serial"
}

# --- fetch devices from Netbox -----------------------------------------------

echo "Fetching devices from Netbox..."

query="?has_oob_ip=true&limit=200"
[[ -n "$CLUSTER_ID" ]] && query+="&cluster_id=${CLUSTER_ID}"
[[ -n "$SITE" ]]       && query+="&site=${SITE}"

devices=$(netbox_get "/api/dcim/devices/${query}")
count=$(echo "$devices" | python3 -c "import sys,json; print(json.load(sys.stdin)['count'])")
echo "Found ${count} devices with OOB IP"

# --- start proxy pod ---------------------------------------------------------

echo "Starting proxy pod '${PROXY_POD}' in namespace '${NAMESPACE}'..."
k run "$PROXY_POD" -n "$NAMESPACE" \
  --image=keppel.eu-de-2.cloud.sap/ccloud-dockerhub-mirror/library/alpine:3.21 \
  --restart=Never \
  --command -- sh -c "sleep 3600"

echo "Waiting for proxy pod to be ready..."
k wait pod "$PROXY_POD" -n "$NAMESPACE" \
  --for=condition=Ready --timeout=60s

cleanup() {
  echo "Deleting proxy pod..."
  k delete pod "$PROXY_POD" -n "$NAMESPACE" --ignore-not-found
}
trap cleanup EXIT

# --- iterate devices ---------------------------------------------------------

updated=0
skipped=0
failed=0

echo ""
echo "Processing devices..."

while IFS=$'\t' read -r dev_id dev_name bmc_ip existing_serial; do
  if [[ -z "$bmc_ip" ]]; then
    echo "  SKIP  ${dev_name}: no OOB IP"
    ((skipped++)) || true
    continue
  fi

  if [[ -n "$existing_serial" && "$OVERWRITE" == "false" ]]; then
    echo "  SKIP  ${dev_name} (${bmc_ip}): serial already set (${existing_serial})"
    ((skipped++)) || true
    continue
  fi

  echo -n "  PROBE ${dev_name} (${bmc_ip})... "
  serial=$(fetch_serial "$bmc_ip")

  if [[ -z "$serial" ]]; then
    echo "FAILED (no serial returned)"
    ((failed++)) || true
    continue
  fi

  echo -n "got '${serial}'"

  if [[ "$DRY_RUN" == "true" ]]; then
    echo " — DRY-RUN, skipping write"
    ((skipped++)) || true
  else
    netbox_patch "/api/dcim/devices/${dev_id}/" "{\"serial\":\"${serial}\"}" > /dev/null
    echo " — updated Netbox"
    ((updated++)) || true
  fi
done < <(echo "$devices" | python3 -c "
import sys, json
for dev in json.load(sys.stdin)['results']:
    oob = dev.get('oob_ip') or {}
    addr = (oob.get('address') or '').split('/')[0]
    print(dev['id'], dev['name'], addr, dev.get('serial') or '', sep='\t')
")

echo ""
echo "Done. updated=${updated} skipped=${skipped} failed=${failed}"
