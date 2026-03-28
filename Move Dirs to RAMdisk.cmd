@(REM coding:CP866
SETLOCAL ENABLEEXTENSIONS
	SET "vscodeRemoteWSLDistSubdir=Soft FOSS\Office Text Publishing\Text Documents\Visual Studio Code Addons\Remote Server\vscode-remote-wsl"
)
@(
	CALL "%~dp0_Distributives.find_subpath.cmd" Distributives "%vscodeRemoteWSLDistSubdir%"
	IF DEFINED Distributives IF EXIST %Distributives%\%vscodeRemoteWSLDistSubdir% SET "vscodeRemoteWSLDist=%Distributives%\%vscodeRemoteWSLDistSubdir%"
	"%LocalAppData%\Programs\bin\UpdateRamdiskLinks.exe" "%~dp0ramdisk-config.yaml"
	EXIT /B
)
