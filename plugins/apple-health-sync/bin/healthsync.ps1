$ErrorActionPreference = "Stop"

function Fail([string]$Message, [int]$Code = 78) {
    [Console]::Error.WriteLine($Message)
    exit $Code
}

function Get-Sha256([string]$Path) {
    $Stream = [IO.File]::OpenRead($Path)
    $Hasher = [Security.Cryptography.SHA256]::Create()
    try {
        return ([BitConverter]::ToString($Hasher.ComputeHash($Stream))).Replace("-", "").ToLowerInvariant()
    } finally {
        $Hasher.Dispose()
        $Stream.Dispose()
    }
}

function Expand-RuntimeArchive([string]$ArchivePath, [string]$DestinationPath) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [IO.Compression.ZipFile]::ExtractToDirectory($ArchivePath, $DestinationPath)
}

$PluginRoot = Split-Path -Parent $PSScriptRoot
$PlatformTag = if ([Environment]::Is64BitOperatingSystem -and $env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
    "windows-x64"
} else {
    Fail "Apple Health Sync supports Windows x64 only on Windows."
}

$RuntimeAction = $null
$ForwardedArguments = @($args)
if ($ForwardedArguments.Count -ge 2 -and $ForwardedArguments[0] -eq "runtime") {
    $RuntimeAction = $ForwardedArguments[1]
    if ($ForwardedArguments.Count -ne 2 -or $RuntimeAction -notin @("status", "verify", "clean")) {
        Fail "Usage: healthsync runtime status|verify|clean" 64
    }
    $ForwardedArguments = @()
}

$BundledRuntime = Join-Path $PluginRoot "runtime\$PlatformTag\healthsync.exe"
if (Test-Path -LiteralPath $BundledRuntime -PathType Leaf) {
    switch ($RuntimeAction) {
        "status" { Write-Output "Apple Health Sync runtime: bundled ($PlatformTag)"; exit 0 }
        "verify" {
            & $BundledRuntime --version
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            & $BundledRuntime self-test | Out-Null
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            Write-Output "Apple Health Sync bundled runtime verified."
            exit 0
        }
        "clean" {
            Write-Output "The bundled offline runtime is part of the plugin and was not removed."
            exit 0
        }
        default {
            & $BundledRuntime @ForwardedArguments
            exit $LASTEXITCODE
        }
    }
}

$ManifestPath = if ($env:HEALTHSYNC_RUNTIME_MANIFEST) {
    $env:HEALTHSYNC_RUNTIME_MANIFEST
} else {
    Join-Path $PluginRoot "runtime-downloads\manifest.json"
}
if (-not (Test-Path -LiteralPath $ManifestPath -PathType Leaf)) {
    Fail "Apple Health Sync runtime manifest is missing: $ManifestPath"
}
$Manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
$EntryProperty = $Manifest.artifacts.PSObject.Properties[$PlatformTag]
if ($null -eq $EntryProperty) { Fail "Apple Health Sync has no runtime entry for $PlatformTag." }
$Entry = $EntryProperty.Value
$RuntimeVersion = [string]$Manifest.runtimeVersion
$RuntimeUrl = [string]$Entry.url
$RuntimeSha256 = ([string]$Entry.sha256).ToLowerInvariant()
$ExecutableSha256 = ([string]$Entry.executableSha256).ToLowerInvariant()
if ($RuntimeVersion -notmatch '^[A-Za-z0-9._-]+$') { Fail "Invalid runtime version." }
if ($RuntimeSha256 -notmatch '^[0-9a-f]{64}$') { Fail "Invalid runtime checksum." }
if ($ExecutableSha256 -notmatch '^[0-9a-f]{64}$') { Fail "Invalid executable checksum." }
if ([string]$Entry.archive -ne "zip" -or [string]$Entry.executable -ne "healthsync.exe") {
    Fail "Invalid runtime package metadata for $PlatformTag."
}

$CacheRoot = if ($env:HEALTHSYNC_RUNTIME_ROOT) {
    $env:HEALTHSYNC_RUNTIME_ROOT
} else {
    Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "HealthSync\runtime"
}
$VersionRoot = Join-Path $CacheRoot $RuntimeVersion
$RuntimeDirectory = Join-Path $VersionRoot $PlatformTag
$RuntimePath = Join-Path $RuntimeDirectory "healthsync.exe"
$VerifiedMarker = Join-Path $RuntimeDirectory ".verified-sha256"
$LockDirectory = Join-Path $VersionRoot ".$PlatformTag.installing"

function Test-VerifiedRuntime {
    if (-not (Test-Path -LiteralPath $RuntimePath -PathType Leaf)) { return $false }
    if (-not (Test-Path -LiteralPath $VerifiedMarker -PathType Leaf)) { return $false }
    if ((Get-Content -LiteralPath $VerifiedMarker -Raw).Trim().ToLowerInvariant() -ne $RuntimeSha256) {
        return $false
    }
    return ((Get-Sha256 $RuntimePath) -eq $ExecutableSha256)
}

function Test-RuntimeCommand {
    try {
        $ReportedVersion = (& $RuntimePath --version | Out-String).Trim()
        if ($LASTEXITCODE -ne 0 -or $ReportedVersion -ne "healthsync $RuntimeVersion") { return $false }
        & $RuntimePath self-test | Out-Null
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

switch ($RuntimeAction) {
    "status" {
        if (Test-VerifiedRuntime) {
            Write-Output "Apple Health Sync runtime $RuntimeVersion is cached for $PlatformTag at $RuntimePath"
        } else {
            Write-Output "Apple Health Sync runtime $RuntimeVersion is not cached for $PlatformTag."
        }
        exit 0
    }
    "verify" {
        if (-not (Test-VerifiedRuntime) -or -not (Test-RuntimeCommand)) {
            Fail "Apple Health Sync runtime $RuntimeVersion is not installed or failed verification." 1
        }
        Write-Output "Apple Health Sync runtime $RuntimeVersion verified for $PlatformTag."
        exit 0
    }
    "clean" {
        if (Test-Path -LiteralPath $RuntimeDirectory) {
            $RuntimeItem = Get-Item -LiteralPath $RuntimeDirectory -Force
            if ($RuntimeItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
                Fail "Refusing to remove a linked runtime directory: $RuntimeDirectory"
            }
            Remove-Item -LiteralPath $RuntimeDirectory -Recurse -Force
            Write-Output "Removed cached Apple Health Sync runtime $RuntimeVersion for $PlatformTag."
        } else {
            Write-Output "No cached Apple Health Sync runtime exists for $PlatformTag."
        }
        exit 0
    }
}

if ((Test-VerifiedRuntime) -and (Test-RuntimeCommand)) {
    & $RuntimePath @ForwardedArguments
    exit $LASTEXITCODE
}

New-Item -ItemType Directory -Path $VersionRoot -Force | Out-Null
$LockAcquired = $false
for ($Attempt = 0; $Attempt -lt 120; $Attempt++) {
    try {
        New-Item -ItemType Directory -Path $LockDirectory -ErrorAction Stop | Out-Null
        Set-Content -LiteralPath (Join-Path $LockDirectory "pid") -Value $PID -NoNewline
        $LockAcquired = $true
        break
    } catch {
        if ((Test-VerifiedRuntime) -and (Test-RuntimeCommand)) {
            & $RuntimePath @ForwardedArguments
            exit $LASTEXITCODE
        }
        $LockPidPath = Join-Path $LockDirectory "pid"
        if (Test-Path -LiteralPath $LockPidPath -PathType Leaf) {
            $LockItem = Get-Item -LiteralPath $LockDirectory -Force
            if (-not ($LockItem.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
                $LockPid = 0
                if ([int]::TryParse((Get-Content -LiteralPath $LockPidPath -Raw), [ref]$LockPid)) {
                    if ($null -eq (Get-Process -Id $LockPid -ErrorAction SilentlyContinue)) {
                        Remove-Item -LiteralPath $LockDirectory -Recurse -Force
                        continue
                    }
                }
            }
        }
        Start-Sleep -Seconds 1
    }
}
if (-not $LockAcquired) { Fail "Timed out waiting for another Apple Health Sync runtime installation." 75 }

$TemporaryDirectory = Join-Path $VersionRoot (".download." + [Guid]::NewGuid().ToString("N"))
try {
    New-Item -ItemType Directory -Path $TemporaryDirectory | Out-Null
    $ArchivePath = Join-Path $TemporaryDirectory "runtime.zip"
    $ExtractDirectory = Join-Path $TemporaryDirectory "extracted"
    New-Item -ItemType Directory -Path $ExtractDirectory | Out-Null

    [Console]::Error.WriteLine("Preparing Apple Health Sync runtime $RuntimeVersion for $PlatformTag...")
    [Console]::Error.WriteLine("Downloading the verified runtime package...")
    $ParsedUri = [Uri]$RuntimeUrl
    if ($ParsedUri.Scheme -eq "https") {
        Invoke-WebRequest -Uri $ParsedUri -OutFile $ArchivePath -MaximumRedirection 5 -UseBasicParsing
    } elseif ($ParsedUri.Scheme -eq "file" -and $env:HEALTHSYNC_ALLOW_FILE_URL_FOR_TESTS -eq "1") {
        Copy-Item -LiteralPath $ParsedUri.LocalPath -Destination $ArchivePath
    } else {
        Fail "Non-HTTPS runtime URLs are not allowed."
    }

    $ActualSha256 = Get-Sha256 $ArchivePath
    if ($ActualSha256 -ne $RuntimeSha256) { Fail "Apple Health Sync runtime checksum verification failed." 65 }

    [Console]::Error.WriteLine("Verifying and installing the runtime...")
    Expand-RuntimeArchive $ArchivePath $ExtractDirectory
    $StagedRuntime = Join-Path $ExtractDirectory "healthsync.exe"
    if (-not (Test-Path -LiteralPath $StagedRuntime -PathType Leaf)) {
        Fail "Downloaded runtime package has an invalid structure." 65
    }
    $ActualExecutableSha256 = Get-Sha256 $StagedRuntime
    if ($ActualExecutableSha256 -ne $ExecutableSha256) {
        Fail "Apple Health Sync executable checksum verification failed." 65
    }
    $StagedVersion = (& $StagedRuntime --version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $StagedVersion -ne "healthsync $RuntimeVersion") {
        Fail "Downloaded runtime reported an unexpected version: $StagedVersion" 65
    }
    & $StagedRuntime self-test | Out-Null
    if ($LASTEXITCODE -ne 0) { Fail "Downloaded runtime self-test failed." 65 }
    Set-Content -LiteralPath (Join-Path $ExtractDirectory ".verified-sha256") -Value $RuntimeSha256 -NoNewline

    if (Test-Path -LiteralPath $RuntimeDirectory) {
        $RuntimeItem = Get-Item -LiteralPath $RuntimeDirectory -Force
        if ($RuntimeItem.Attributes -band [IO.FileAttributes]::ReparsePoint) {
            Fail "Refusing to replace a linked runtime directory: $RuntimeDirectory"
        }
        Remove-Item -LiteralPath $RuntimeDirectory -Recurse -Force
    }
    Move-Item -LiteralPath $ExtractDirectory -Destination $RuntimeDirectory
    [Console]::Error.WriteLine("Apple Health Sync runtime is ready.")
} finally {
    if (Test-Path -LiteralPath $TemporaryDirectory) {
        Remove-Item -LiteralPath $TemporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $LockDirectory) {
        Remove-Item -LiteralPath $LockDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}

& $RuntimePath @ForwardedArguments
exit $LASTEXITCODE
