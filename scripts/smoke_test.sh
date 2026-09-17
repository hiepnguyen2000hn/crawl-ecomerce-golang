#!/usr/bin/env bash
# scripts/smoke_test.sh
set -euo pipefail

KEYWORD="golang"
PATH_TYPE="trend"
COUNTRY="US"
GEO=""
while [ $# -gt 0 ]; do
  case "$1" in
    --keyword)
      KEYWORD="$2"
      shift 2
      ;;
    --path)
      PATH_TYPE="$2"
      shift 2
      ;;
    --country)
      COUNTRY="$2"
      shift 2
      ;;
    --geo)
      GEO="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

case "$PATH_TYPE" in
  trend)
    ENDPOINT="jobs/trend"
    BODY="{\"keyword\":\"${KEYWORD}\",\"geo\":\"${GEO}\"}"
    ;;
  fbads)
    ENDPOINT="jobs/fbads"
    BODY="{\"query\":\"${KEYWORD}\",\"country\":\"${COUNTRY}\"}"
    ;;
  *)
    echo "Unknown --path: $PATH_TYPE (expected 'trend' or 'fbads')" >&2
    exit 1
    ;;
esac

JOB_ID=$(curl -sf -X POST http://localhost:8888/${ENDPOINT} \
  -H 'Content-Type: application/json' \
  -d "${BODY}" | jq -r .job_id)

echo "Created job: $JOB_ID"

for i in $(seq 1 30); do
  STATUS=$(curl -sf http://localhost:8888/${ENDPOINT}/$JOB_ID | jq -r .status)
  echo "Status: $STATUS"
  if [ "$STATUS" = "done" ]; then
    curl -sf http://localhost:8888/${ENDPOINT}/$JOB_ID | jq .
    exit 0
  fi
  if [ "$STATUS" = "failed" ]; then
    echo "Job failed"
    exit 1
  fi
  sleep 2
done

echo "Timed out waiting for job to complete"
exit 1
