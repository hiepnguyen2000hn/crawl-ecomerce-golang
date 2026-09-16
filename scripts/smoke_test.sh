#!/usr/bin/env bash
# scripts/smoke_test.sh
set -euo pipefail

JOB_ID=$(curl -sf -X POST http://localhost:8888/jobs/trend \
  -H 'Content-Type: application/json' \
  -d '{"keyword":"golang"}' | jq -r .job_id)

echo "Created job: $JOB_ID"

for i in $(seq 1 30); do
  STATUS=$(curl -sf http://localhost:8888/jobs/trend/$JOB_ID | jq -r .status)
  echo "Status: $STATUS"
  if [ "$STATUS" = "done" ]; then
    curl -sf http://localhost:8888/jobs/trend/$JOB_ID | jq .
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
