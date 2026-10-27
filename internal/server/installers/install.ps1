# UEM agent installer for Windows. Run from an elevated PowerShell:
#   & ([scriptblock]::Create((irm https://uem.example.com/install.ps1))) -Server 'https://uem.example.com' -Token 'enr_xxx'
param(
    [Parameter(Mandatory = $true)][string]$Server,
    [Parameter(Mandatory = $true)][string]$Token
)
$ErrorActionPreference = 'Stop'

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'The UEM agent installer must run as Administrator.'
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$dir = Join-Path $env:ProgramFiles 'UEM Agent'
$bin = Join-Path $dir 'uem-agent.exe'
New-Item -ItemType Directory -Force -Path $dir | Out-Null

if (Get-Service -Name 'uem-agent' -ErrorAction SilentlyContinue) {
    & $bin stop 2>$null | Out-Null
}

Write-Host "Downloading UEM agent (windows/$arch)..."
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -UseBasicParsing -Uri "$Server/downloads/uem-agent-windows-$arch.exe" -OutFile $bin

Write-Host "Enrolling with $Server..."
& $bin enroll --server $Server --token $Token
if ($LASTEXITCODE -ne 0) { throw 'Enrollment failed.' }
# The config holds the device secret: restrict it to SYSTEM and Administrators.
$cfgDir = Join-Path $env:ProgramData 'UEM Agent'
icacls $cfgDir /inheritance:r /grant:r 'SYSTEM:(OI)(CI)F' 'Administrators:(OI)(CI)F' | Out-Null
& $bin install
& $bin start
Write-Host 'UEM agent installed and running.'
