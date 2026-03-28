@(REM coding:CP866
SETLOCAL ENABLEEXTENSIONS
go build -o UpdateRAMDiskLinks.exe -trimpath
IF EXIST "%LocalAppData%\Programs\bin\UpdateRAMDiskLinks.exe" (
    REN "%LocalAppData%\Programs\bin\UpdateRAMDiskLinks.exe" UpdateRAMDiskLinks.bak
    MKLINK /H "%LocalAppData%\Programs\bin\UpdateRAMDiskLinks.exe" "%~dp0UpdateRAMDiskLinks.exe"
    DEL "%LocalAppData%\Programs\bin\UpdateRAMDiskLinks.bak"
)
)
