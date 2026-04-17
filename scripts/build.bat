@echo off
REM Build script for Windows with build number injection

REM Change to project root
cd %~dp0..

REM Read or initialize build number
if exist .build_number (
    set /p BUILD_NUM=<.build_number
    set /a BUILD_NUM+=1
) else (
    set BUILD_NUM=1
)

REM Save build number
echo %BUILD_NUM% > .build_number

REM Build with ldflags to inject VERSION and BUILD_NUMBER
echo Building with Build Number: %BUILD_NUM%
go build -ldflags "-X main.BUILD_NUMBER=%BUILD_NUM%" -o simple-torrent.exe

if %ERRORLEVEL% EQU 0 (
    echo Build successful! Build number: %BUILD_NUM%
    echo Starting simple-torrent.exe...
    .\simple-torrent.exe
) else (
    echo Build failed!
    exit /b 1
)
