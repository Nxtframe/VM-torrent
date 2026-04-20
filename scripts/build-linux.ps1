# Build for Linux AMD64
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o cloud-torrent -ldflags "-s -w"

# Reset
$env:GOOS = "windows"
$env:GOARCH = "amd64"

Write-Host "Built: cloud-torrent"
