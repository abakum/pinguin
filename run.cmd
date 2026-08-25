@echo off
rem Run pinguin VK bot on Windows
cd /d %~dp0

set bot=PingBot
set TOKEN=YourCommunityTokenHere
set goBin=%USERPROFILE%\go\bin
set PEER_IDS=123456789 -2000000001

go install
start "%bot%" %goBin%\pinguin.exe %PEER_IDS%
