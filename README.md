# pinguin
Pinger with VK bot interface without WebHook — port of https://github.com/abakum/tmbPi

## Создание сообщества и получение токена

1. **Создайте сообщество**: [vk.com/groups](https://vk.com/groups) → «Создать сообщество» → выберите любой тип (например, «Бизнес» или «Группа по интересам») → задайте название.
2. **Включите возможности ботов**: управление сообществом → «Сообщения» → «Сообщения сообщества» — включите. Разрешите боту писать первым.
3. **Включите Long Poll**: «Сообщения» → «Настройки для ботов» → «Настройки Long Poll» — включите (API версии 3.x используйте актуальную).
4. **Получите ключ доступа** (токен сообщества): управление → «Работа с API» → «Ключи доступа» → «Создать ключ» → отметьте права: **Управление сообщениями** (`messages`) — достаточно для отправки/удаления сообщений и работы callback-кнопок. Скопируйте токен — он показывается один раз.
5. **Узнайте peer ID** разрешённых чатов:
   - ЛС с ботом: положительный ID пользователя (`vk.com/id…`);
   - беседа: отрицательный `2000000000 + id_беседы`;
   - чтобы бот работал в беседе, добавьте его в участники беседы.

## Usage
```
TOKEN=VK_COMMUNITY_TOKEN pinguin AllowedPeerID1 AllowedPeerID2 AllowedPeerIDx
```
- `TOKEN` — токен сообщества VK (Bots API, long polling)
- аргументы — разрешённые peer ID (положительные для ЛС, отрицательные для бесед)
- состояние пингов не переносится со старого `tmbPing.json` — начинается с чистого файла

## Запуск на Windows

`run.cmd` собирает (`go install`) и запускает бота в отдельном окне:

```cmd
@echo off
rem Run pinguin VK bot on Windows
cd /d %~dp0

set bot=PingBot
set TOKEN=YourCommunityTokenHere
set goBin=%USERPROFILE%\go\bin
set PEER_IDS=123456789 -2000000001

go install
start "%bot%" %goBin%\pinguin.exe %PEER_IDS%
```

Укажите в `run.cmd` свой `TOKEN` и список `PEER_IDS` перед запуском.

## Credits
- VK — for [Bots API](https://dev.vk.com/ru/api/bots/overview)
- Severe Cloud — for [vksdk](https://github.com/SevereCloud/vksdk)
- Prometheus Monitoring Community — for [pro-bing](https://github.com/prometheus-community/pro-bing)
