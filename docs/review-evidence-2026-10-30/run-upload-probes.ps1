# 使用隔离的 Go overlay 复核审查发现；不调用真实网盘。
$ErrorActionPreference = 'Stop'
$reviewRepo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$reviewBaseline = git -C $reviewRepo rev-parse HEAD
if (-not $reviewBaseline.StartsWith('8dc22c0') -or (git -C $reviewRepo diff --name-only)) { throw '此脚本只用于干净的 8dc22c0 基线，当前源码请运行正式回归测试。' }
$reviewScratch = Join-Path $reviewRepo ('.trash/review-probes-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $reviewScratch | Out-Null
$reviewMapping = @{}
$reviewMapping[(Join-Path $reviewRepo 'internal/transfer/review_probe_test.go')] = Join-Path $PSScriptRoot 'transfer_probe_test.go.txt'
$reviewMapping[(Join-Path $reviewRepo 'internal/drive/providers/aliopen/review_probe_test.go')] = Join-Path $PSScriptRoot 'ali_probe_test.go.txt'
$reviewMapping[(Join-Path $reviewRepo 'internal/drive/providers/dropbox/review_probe_test.go')] = Join-Path $PSScriptRoot 'dropbox_probe_test.go.txt'
$reviewOverlay = Join-Path $reviewScratch 'overlay.json'
$reviewJSON = @{ Replace = $reviewMapping } | ConvertTo-Json
[System.IO.File]::WriteAllText($reviewOverlay, $reviewJSON, [System.Text.UTF8Encoding]::new($false))
Push-Location $reviewRepo
try {
    & go test '-overlay' $reviewOverlay './internal/transfer' './internal/drive/providers/aliopen' './internal/drive/providers/dropbox' '-run' '^TestReview' '-count=1' '-v'
    if ($LASTEXITCODE -ne 0) {
        throw '审查探针未通过，请检查输出。修复缺陷后，当前缺陷表现探针也会失败。'
    }
} finally {
    Pop-Location
}
