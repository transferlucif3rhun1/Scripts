@echo off

rem Automatically set the path to serverRouting.exe relative to this batch file's location
set "EXECUTABLE_PATH=%~dp0getNumber.exe"

rem Run the executable
echo Running executable...
start "getNumber" "%EXECUTABLE_PATH%"

rem Wait for 1 second to ensure the executable has started
timeout /t 1 >nul

rem Send an Enter key stroke to the running executable using SendKeys
echo Set WshShell = WScript.CreateObject("WScript.Shell") > "%TEMP%\SendKeys.vbs"
echo WshShell.AppActivate "serverRouting" >> "%TEMP%\SendKeys.vbs"
echo WshShell.SendKeys "{ENTER}" >> "%TEMP%\SendKeys.vbs"
cscript //nologo "%TEMP%\SendKeys.vbs"
