@echo off
rem Launch YHagent-Pet (Electron) detached from terminal job tree
cd /d d:\code\YHagent\client
start "YHagent-Pet" /min node_modules\electron\dist\electron.exe .
