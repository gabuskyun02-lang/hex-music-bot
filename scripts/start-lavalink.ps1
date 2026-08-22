# Downloads Lavalink (latest release) if missing and starts it with the
# bundled config. Requires Java 17+.
$ErrorActionPreference = "Stop"

$lavalinkDir = Join-Path (Split-Path $PSScriptRoot -Parent) "lavalink"
$jar = Join-Path $lavalinkDir "Lavalink.jar"

if (-not (Test-Path $jar)) {
    Write-Host "Downloading Lavalink..."
    Invoke-WebRequest `
        -Uri "https://github.com/lavalink-devs/Lavalink/releases/latest/download/Lavalink.jar" `
        -OutFile $jar
}

Set-Location $lavalinkDir
java -jar Lavalink.jar
