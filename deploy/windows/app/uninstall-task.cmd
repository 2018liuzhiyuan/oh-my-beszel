@echo off
rem Stops and removes the "Beszel Hub" logon task via Monitor.exe. The hub
rem itself keeps running until shutdown; Monitor.exe can still start it
rem directly on demand.
"%~dp0Monitor.exe" uninstall-task
