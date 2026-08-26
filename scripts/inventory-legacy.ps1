param(
  [Parameter(Mandatory = $true)][string[]]$SitePaths,
  [string]$OutputPath = ".\migration-reports\legacy-inventory.json"
)

$ErrorActionPreference = 'Stop'
$resourceNames = @('Book', 'Passage', 'PPT', 'Mentor', 'Video', 'videos', 'Audio', 'assets', 'public')
$sites = foreach ($sitePathValue in $SitePaths) {
  $root = (Resolve-Path -LiteralPath $sitePathValue).Path
  $configPath = Join-Path $root 'config.json'
  $sourceSite = Split-Path -Leaf $root
  [string[]]$recordsCandidates = @(
    (Join-Path $root 'data\records.json'),
    (Join-Path $root 'records.json')
  ) | Where-Object { Test-Path -LiteralPath $_ }
  $config = if (Test-Path -LiteralPath $configPath) { Get-Content -Raw -LiteralPath $configPath | ConvertFrom-Json } else { $null }
  $records = @()
  if ($recordsCandidates.Count) {
    $parsed = Get-Content -Raw -LiteralPath ($recordsCandidates[0]) | ConvertFrom-Json
    if ($parsed -is [System.Collections.IEnumerable] -and $parsed -isnot [string]) { $records = @($parsed) }
  }
  $dates = @($records | ForEach-Object { $_.logical_date ?? $_.date } | Where-Object { $_ -match '^\d{4}-\d{2}-\d{2}$' } | Sort-Object)
  $resources = foreach ($resourceName in $resourceNames) {
    $resourceRoot = Join-Path $root $resourceName
    if (-not (Test-Path -LiteralPath $resourceRoot -PathType Container)) { continue }
    Get-ChildItem -LiteralPath $resourceRoot -File -Recurse | Where-Object { $_.FullName -notmatch '[\\/]node_modules[\\/]' } | ForEach-Object {
      $relative = "$sourceSite/$($_.FullName.Substring($root.Length).TrimStart('\', '/').Replace('\', '/'))"
      [pscustomobject][ordered]@{ path = $relative; size = $_.Length; sha256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
    }
  }
  $members = @($config.members ?? $config.users ?? @())
  $weeks = @($config.weekly_schedule ?? $config.weeks ?? @())
  [pscustomobject][ordered]@{
    source_site = $sourceSite
    root = $root
    config_found = [bool]$config
    member_count = $members.Count
    task_week_count = $weeks.Count
    checkin_count = $records.Count
    earliest_date = if ($dates.Count) { $dates[0] } else { $null }
    latest_date = if ($dates.Count) { $dates[-1] } else { $null }
    resource_count = @($resources).Count
    resource_bytes = (@($resources) | Measure-Object size -Sum).Sum ?? 0
    resources = @($resources)
  }
}

$allFiles = @($sites | ForEach-Object { $_.resources })
$duplicates = @($allFiles | Group-Object sha256 | Where-Object Count -gt 1 | ForEach-Object {
  [pscustomobject][ordered]@{ sha256 = $_.Name; copies = $_.Count; paths = @($_.Group.path) }
})
$report = [pscustomobject][ordered]@{ generated_at = (Get-Date).ToUniversalTime().ToString('o'); mode = 'read-only'; sites = @($sites); duplicate_resources = $duplicates }
$parent = Split-Path -Parent $OutputPath
if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
$report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding utf8
Write-Output "Inventory written to $OutputPath"
