# Create the local object-storage bucket, retrying until the SeaweedFS S3
# gateway is ready. Run by `task storage:init` (part of `task up`).
#
# Idempotent: SeaweedFS answers 409 Conflict when the bucket already exists,
# which we treat as success. Values come from the task environment (.env).
param(
    [string]$Endpoint  = $env:S3_ENDPOINT,
    [string]$Bucket    = $env:S3_BUCKET,
    [string]$AccessKey = $env:S3_ACCESS_KEY,
    [string]$SecretKey = $env:S3_SECRET_KEY,
    [int]$Attempts     = 30
)

$ErrorActionPreference = 'Stop'

if (-not $Endpoint -or -not $Bucket) {
    throw 'S3_ENDPOINT and S3_BUCKET must be set (copy .env.example to .env).'
}

for ($i = 1; $i -le $Attempts; $i++) {
    $code = & curl.exe -s -o NUL -w '%{http_code}' `
        --aws-sigv4 'aws:amz:us-east-1:s3' `
        --user "${AccessKey}:${SecretKey}" `
        -X PUT "$Endpoint/$Bucket"

    if ($code -in @('200', '409')) {
        Write-Output "atlas storage bucket -> $code"
        exit 0
    }

    Write-Output "atlas storage not ready yet (attempt $i/$Attempts, status $code)..."
    Start-Sleep -Seconds 1
}

throw "object storage bucket init failed after $Attempts attempts (last status $code)"
