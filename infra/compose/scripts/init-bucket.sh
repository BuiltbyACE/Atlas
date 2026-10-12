#!/bin/sh
# Create the local object-storage bucket, retrying until the SeaweedFS S3
# gateway is ready. Run by `task storage:init` (part of `task up`).
#
# Idempotent: SeaweedFS answers 409 Conflict when the bucket already exists,
# which we treat as success. Values come from the task environment (.env).
set -eu

endpoint="${S3_ENDPOINT:?S3_ENDPOINT must be set (copy .env.example to .env)}"
bucket="${S3_BUCKET:?S3_BUCKET must be set}"
access_key="${S3_ACCESS_KEY:?S3_ACCESS_KEY must be set}"
secret_key="${S3_SECRET_KEY:?S3_SECRET_KEY must be set}"
attempts="${ATTEMPTS:-30}"

i=1
while [ "$i" -le "$attempts" ]; do
  # `|| true` keeps `set -e` from aborting on a connection-refused exit code.
  code=$(curl -s -o /dev/null -w '%{http_code}' \
    --aws-sigv4 'aws:amz:us-east-1:s3' \
    --user "${access_key}:${secret_key}" \
    -X PUT "$endpoint/$bucket" || true)
  [ -n "$code" ] || code=000

  case "$code" in
    200 | 409)
      echo "atlas storage bucket -> $code"
      exit 0
      ;;
  esac

  echo "atlas storage not ready yet (attempt $i/$attempts, status $code)..."
  i=$((i + 1))
  sleep 1
done

echo "object storage bucket init failed after $attempts attempts (last status $code)" >&2
exit 1
