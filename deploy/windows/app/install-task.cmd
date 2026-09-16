@echo off
rem Registers and starts the "Beszel Hub" logon task via Monitor.exe. Going
rem through the binary instead of a .ps1 keeps fresh downloads working when
rem the PowerShell execution policy blocks downloaded scripts.
"%~dp0Monitor.exe" install-task
