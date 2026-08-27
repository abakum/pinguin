@echo off
rem Example: store pinguin settings in Windows Credential Manager
rem /user is required by cmdkey but ignored when reading: wincred looks up by /generic name only
cmdkey /generic:pinguin_token /user:pinguin /pass:VK_COMMUNITY_TOKEN
cmdkey /generic:pinguin_ids   /user:pinguin /pass:"123456789 2000000001"

rem verify
cmdkey /list:pinguin_token
cmdkey /list:pinguin_ids
