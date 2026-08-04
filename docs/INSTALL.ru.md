# Установка

Инструкция для человека, который ставит сервис себе. Если хотите просто попробовать — хватит первого раздела.

---

## Что понадобится

| Компонент | Зачем | Обязателен |
|---|---|---|
| [Go 1.22+](https://go.dev/dl/) | Собрать и запустить сервер | Да |
| `yt-dlp` | Искать и скачивать с YouTube и SoundCloud | Для внешних источников |
| `ffmpeg` | Конвертировать звук в mp3 | Для скачивания |
| [Flutter](https://docs.flutter.dev/get-started/install) | Собрать мобильное приложение | Только для сборки клиента |
| [Node.js 20+](https://nodejs.org/) | Собрать веб-версию для ПК | Только для сборки клиента |

Готовые сборки клиентов лежат в репозитории, поэтому Flutter и Node нужны, только если вы что-то меняете в интерфейсе.

---

## 1. Запуск за пять минут

```bash
git clone https://github.com/Kisskin-Mister/music-orchestrator.git
cd music-orchestrator
cp .env.example .env
go run .
```

Сервер поднимется на `http://127.0.0.1:8080`. Откройте адрес в браузере — вас попросят придумать логин и пароль. Первый созданный аккаунт становится администратором.

Проверить, что сервер жив:

```bash
curl http://127.0.0.1:8080/health
```

---

## 2. Включить YouTube и SoundCloud

Из коробки внешние источники выключены — сервер безопасен для запуска «как есть». Чтобы включить:

**Шаг 1. Поставьте утилиты**

```bash
# macOS
brew install yt-dlp ffmpeg

# Ubuntu / Debian / Raspberry Pi
sudo apt-get update && sudo apt-get install -y ffmpeg
python3 -m pip install -U yt-dlp

# Проверка
yt-dlp --version && ffmpeg -version | head -1
```

**Шаг 2. Укажите путь к yt-dlp**, если он не в системном `PATH`:

```bash
which yt-dlp   # например /home/user/.local/bin/yt-dlp
```

Впишите этот путь в `.env`:

```env
APP_YT_DLP_BINARY=/home/user/.local/bin/yt-dlp
```

Это единственная настройка источников, которую **нужно** задавать в файле. Причина: сервер запускает этот файл как программу, поэтому менять путь через веб-интерфейс небезопасно — тот, кто получил доступ к админке, смог бы запустить на вашей машине что угодно.

**Шаг 3. Включите источники в интерфейсе**

**Настройки → Настройки сервера → Источники YouTube и SoundCloud**. Перезапуск не нужен.

> `yt-dlp` стоит обновлять раз в пару месяцев: YouTube меняется, старые версии перестают работать. `python3 -m pip install -U yt-dlp`

---

## 3. Настройки

Почти всё настраивается в интерфейсе: **Настройки → Настройки сервера**. Изменения применяются сразу и переживают перезапуск.

Через `.env` задаётся то, что читается один раз при старте:

| Переменная | Что это | По умолчанию |
|---|---|---|
| `APP_ADDR` | Адрес и порт | `:8080` |
| `APP_STORE_PATH` | Файл с данными | `./data/store.json` |
| `APP_MEDIA_ROOT` | Папка со скачанной музыкой | `./data/media` |
| `APP_WEB_ROOT` | Папка веб-клиента | `./mobile/build/web` |
| `APP_YT_DLP_BINARY` | Путь к yt-dlp | `yt-dlp` |
| `APP_API_KEYS` | Ключи для доступа к API | `change-me-local-dev-key` |

Полный список — в [.env.example](../.env.example).

**Обязательно поменяйте `APP_API_KEYS`,** если сервер будет доступен кому-то кроме вас.

---

## 4. Доступ с телефона

### В домашней сети

Узнайте адрес компьютера и запустите сервер так, чтобы он слушал не только себя:

```bash
# адрес в локальной сети
ipconfig getifaddr en0          # macOS
hostname -I | awk '{print $1}'  # Linux
```

В `.env`:

```env
APP_ADDR=:8080
APP_CORS_ORIGINS=*
```

Теперь на телефоне откройте `http://АДРЕС:8080` и добавьте на домашний экран.

### Из любой точки мира

Дома сервер в локальной сети, и из города телефон его не видит. Самый простой способ — **Tailscale**: приватная сеть между вашими устройствами без проброса портов.

```bash
# на сервере
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up
```

Поставьте Tailscale на телефон, войдите тем же аккаунтом — и сервер будет доступен по своему постоянному адресу откуда угодно.

Альтернатива — [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/), если нужен публичный HTTPS-адрес.

Если открываете сервер в интернет напрямую, обязательно включите **Secure-флаг у cookie** (Настройки → Настройки сервера) и поставьте HTTPS.

---

## 5. Автозапуск

### Linux (systemd)

```bash
go build -o music-orchestrator .
sudo mv music-orchestrator /usr/local/bin/
```

`/etc/systemd/system/music-orchestrator.service`:

```ini
[Unit]
Description=Music Orchestrator
After=network.target

[Service]
Type=simple
User=ВАШ_ПОЛЬЗОВАТЕЛЬ
WorkingDirectory=/home/ВАШ_ПОЛЬЗОВАТЕЛЬ/music-orchestrator
ExecStart=/usr/local/bin/music-orchestrator
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now music-orchestrator
sudo systemctl status music-orchestrator
```

### Docker

```bash
docker compose up -d
```

Файл [docker-compose.yml](../docker-compose.yml) уже в репозитории. Проверьте, что папки `data/` проброшены наружу — иначе музыка и аккаунты пропадут при пересоздании контейнера.

---

## 6. Сборка клиентов

Нужна, только если вы меняли интерфейс.

```bash
# мобильное приложение и веб-версия, которую раздаёт сервер
cd mobile
flutter build web --release        # → mobile/build/web
flutter build apk --release        # → mobile/build/app/outputs/flutter-apk/

# веб-версия для компьютера
cd frontend/frontend
npm install && npm run build       # → frontend/frontend/dist
```

---

## Резервная копия

Всё важное лежит в двух местах:

```bash
tar czf backup-$(date +%F).tar.gz data/
```

- `data/store.json` — аккаунты, избранное, плейлисты, настройки
- `data/media/` — скачанная музыка

Этого достаточно, чтобы восстановиться на другой машине: скопируйте папку и запустите сервер.

---

## Если что-то не работает

**Сервер не стартует, порт занят**

```bash
lsof -ti:8080 | xargs kill -9
```

**YouTube и SoundCloud не появляются в поиске**

Проверьте по очереди:

```bash
curl http://127.0.0.1:8080/health      # risky_extractors_enabled должно быть true
yt-dlp --version                        # утилита установлена
which yt-dlp                            # путь совпадает с APP_YT_DLP_BINARY
```

**Поиск идёт и обрывается**

Скорее всего, устарел `yt-dlp`: `python3 -m pip install -U yt-dlp`. Если источник просто медленный — увеличьте таймаут поиска в настройках сервера.

**Не заходит с телефона**

Убедитесь, что телефон в той же сети, а сервер слушает `:8080`, а не `127.0.0.1:8080` — во втором случае он доступен только сам себе.

**Скачанное на телефон перестало играть**

Приложение проверяет наличие файлов при запуске и убирает из списка пропавшие. Если файл пропал, скачайте трек заново: iOS могла очистить хранилище при нехватке места.
