@echo off
rem Launch YHagent-Pet detached from terminal job tree, capturing stderr to file
rem (PowerShell Start-Process makes the exe a short-lived PS child; PS exits and
rem  the exe survives, with panic/fatal logs landing in pet_stderr.log)
start "" /min powershell -NoProfile -Command "Start-Process -FilePath 'D:\code\YHagent\client\build\bin\YHagent-Pet.exe' -RedirectStandardError 'D:\code\YHagent\client\build\bin\pet_stderr.log' -RedirectStandardOutput 'D:\code\YHagent\client\build\bin\pet_stdout.log'"
