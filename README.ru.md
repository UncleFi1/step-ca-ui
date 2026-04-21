<div align="center">

# Step-CA UI

**Self-hosted веб-интерфейс для [Smallstep step-ca](https://smallstep.com/docs/step-ca/) — управляйте собственным PKI прямо из браузера.**

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Made with Go](https://img.shields.io/badge/Made%20with-Go%201.22-00ADD8.svg)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED.svg)](https://docs.docker.com/compose/)
[![Latest release](https://img.shields.io/badge/release-v1.4.0-success.svg)](https://github.com/UncleFi1/step-ca-ui/releases/latest)

[🇬🇧 English](README.md) · 🇷🇺 **Русский**

</div>

---

> Удобный веб-интерфейс над `smallstep/step-ca` для небольших команд. Никаких облаков, телеметрии и vendor lock-in — всё работает на вашем сервере в трёх Docker-контейнерах.

## Возможности

- 📋 **Управление сертификатами** — выпуск, перевыпуск, отзыв и импорт X.509
- 👥 **Ролевая модель** — `admin` / `manager` / `viewer`
- ⏱️ **Временные пользователи** — гостевые аккаунты с автоматическим истечением *(новинка v1.4.0)*
- 📅 **Кастомный date picker** — в стиле сайта, без браузерного виджета *(новинка v1.4.0)*
- 🌍 **Учёт часового пояса** — Москва по умолчанию, легко меняется
- 🎨 **4 темы** — тёмная, светлая, синяя, авто (по системе)
- 🛡️ **Встроенная безопасность** — CSRF-токены, rate limiting, блокировка IP, журнал
- 🌐 **Provisioner'ы step-ca** — список и редактирование

## Быстрый старт

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui
./install.sh
```

И всё. Скрипт сам:
1. Определит ОС и установит Docker, если его нет
2. Автоопределит IP сервера (с подтверждением)
3. Сгенерирует надёжные пароли
4. Создаст `.env` и `credentials.txt` (chmod 600)
5. Соберёт и запустит контейнеры
6. Покажет URL и пароль администратора

На свежей виртуалке всё занимает 2–4 минуты.

## Системные требования

|                | Минимум | Рекомендуется | Высокая нагрузка |
|----------------|---------|---------------|------------------|
| **CPU**        | 1 vCPU  | 2 vCPU        | 4+ vCPU          |
| **RAM**        | 1 ГБ    | 2 ГБ          | 4+ ГБ            |
| **Диск**       | 5 ГБ    | 20 ГБ SSD     | 50+ ГБ NVMe      |
| **Сеть**       | 10 Мбит/с | 100 Мбит/с  | 1 Гбит/с         |
| **Пользователи** | до 50  | до 500        | 500+             |
| **Сертификаты** | до 500  | до 10 000     | 10 000+          |

**Программное обеспечение:**
- Linux kernel 4.4+ (Ubuntu 20.04+, Debian 11+, CentOS Stream 9+, Rocky 9+, Alma 9+)
- Docker Engine 20.10+ с плагином Compose v2+
- Открытые порты: `443/tcp` (HTTPS UI), опционально `9000/tcp` (API step-ca)

> Не тестировано, но должно работать: macOS / Windows через Docker Desktop (только для разработки). \
> **Не поддерживается:** shared hosting без Docker, Raspberry Pi Zero (мало RAM).

## Стек

| Слой         | Технология                  |
|--------------|-----------------------------|
| Backend      | Go 1.22, [chi](https://github.com/go-chi/chi) router |
| Frontend     | Server-side HTML + чистый JS, без сборки |
| База данных  | PostgreSQL 16 |
| CA           | [smallstep/step-ca](https://hub.docker.com/r/smallstep/step-ca) |
| Деплой       | Docker Compose |
| OS контейнера| Alpine 3.19 + tzdata        |

## Архитектура

```
                          ┌────────────┐
   Браузер  ─── HTTPS ───►│  step-ui   │  Go-приложение, порт 8443
                          │  (chi)     │
                          └──┬─────┬───┘
                             │     │
                  SQL ◄──────┘     └──────► HTTPS API
                             │     │
                          ┌──▼──┐ ┌▼──────────┐
                          │ pg  │ │ step-ca   │  порт 9000
                          │ 16  │ │ (PKI)     │
                          └─────┘ └───────────┘

   step-ui наружу :443  →  внутри редирект на :8443
   step-ca наружу :9000 →  по умолчанию закрыт от внешней сети
```

## Роли

| Роль    | Просмотр | Выпуск/Импорт | Отзыв | Управление пользователями |
|---------|----------|---------------|-------|---------------------------|
| viewer  | ✅       | ❌            | ❌    | ❌                        |
| manager | ✅       | ✅            | ❌    | ❌                        |
| admin   | ✅       | ✅            | ✅    | ✅                        |

**Временные пользователи** могут иметь любую роль; они автоматически блокируются по истечении `expires_at` (отдельная горутина проверяет раз в минуту).

## Безопасность

- ✅ **CSRF-токены** на каждой форме
- ✅ **Rate limiting** — 5 неудачных попыток входа → блокировка IP на 15 минут
- ✅ **Security headers** — HSTS, CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy
- ✅ **Таймаут сессии** — 8 часов, скользящий
- ✅ **Журнал входов** — каждая попытка с IP и User-Agent
- ✅ **Self-signed TLS** — генерируется при первом запуске, валидность 10 лет
- ⚠️ **Хэширование паролей** — пока SHA-256 *(legacy с v1.0; миграция на bcrypt запланирована в [#1](https://github.com/UncleFi1/step-ca-ui/issues))*

> 🔒 **Совет для production:** поставьте step-ui за reverse proxy (Caddy/nginx) с настоящим TLS-сертификатом, ограничьте доступ через VPN/Tailscale, регулярно бэкапьте том `step-ca-data`.

## Конфигурация

Все настройки в `.env`. Установщик создаёт его автоматически, но вы можете редактировать вручную:

```env
HOST_IP=192.168.1.100              # SAN в self-signed серте; DNS-имя для step-ca
PROVISIONER=admin@example.com      # идентификатор provisioner'а step-ca
CA_PASSWORD=<сгенерировано>        # пароль provisioner'а step-ca
SECRET_KEY=<сгенерировано>         # ключ подписи сессий и CSRF
POSTGRES_PASSWORD=<сгенерировано>  # пароль базы
```

После изменения `.env` пересоздайте контейнеры:

```bash
docker compose up -d --force-recreate
```

## FAQ

<details>
<summary><b>Как изменить порт HTTPS с 443?</b></summary>

Отредактируйте `docker-compose.yml`:
```yaml
services:
  step-ui:
    ports:
      - "8443:8443"   # было "443:8443"
```
И перезапустите: `docker compose up -d --force-recreate step-ui`.
</details>

<details>
<summary><b>Как сделать бэкап?</b></summary>

Всё состояние хранится в двух томах:
- `step-ca-data` — корневые ключи CA, intermediate-сертификаты, provisioner'ы
- том PostgreSQL — пользователи, метаданные сертификатов, логи

```bash
# Бэкап
docker compose exec postgres pg_dump -U stepui stepui > backup.sql
docker run --rm -v step-ca-data:/src -v "$PWD":/dst alpine tar czf /dst/ca-data.tgz -C /src .

# Восстановление
docker compose exec -T postgres psql -U stepui stepui < backup.sql
docker run --rm -v step-ca-data:/dst -v "$PWD":/src alpine tar xzf /src/ca-data.tgz -C /dst
```
</details>

<details>
<summary><b>Как сбросить пароль admin'а?</b></summary>

```bash
docker compose exec postgres psql -U stepui -d stepui -c \
  "UPDATE users SET password_hash = encode(sha256('newpass'::bytea), 'hex') WHERE username='admin';"
```
Войдите как `admin` / `newpass` и сразу смените пароль через интерфейс.
</details>

<details>
<summary><b>Браузер ругается на self-signed сертификат. Как поставить свой?</b></summary>

Замените `step-ui-go/ssl/server.crt` и `server.key` на свой сертификат + ключ (например от Let's Encrypt или вашего внутреннего CA), затем перезапустите `step-ui`. Убедитесь, что серт покрывает ваш `HOST_IP` или hostname.
</details>

<details>
<summary><b>Можно поставить за Cloudflare / Caddy / nginx?</b></summary>

Да. Направьте reverse proxy на `step-ui:8443` (HTTPS upstream) либо переключите step-ui на чистый HTTP и обрабатывайте TLS на прокси. Не забудьте передавать `X-Forwarded-Proto: https`, иначе step-ui будет генерировать неверные URL.
</details>

<details>
<summary><b>Как обновиться на новую версию?</b></summary>

```bash
git pull
docker compose up -d --build
```
Миграции запускаются автоматически. Перед обновлением всегда смотрите [release notes](https://github.com/UncleFi1/step-ca-ui/releases) — мажорные версии могут содержать breaking changes.
</details>

## Участие в разработке

Pull request'ы приветствуются. Для крупных изменений сначала откройте issue, чтобы обсудить.

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui/step-ui-go
go mod download
go run .  # нужны запущенные postgres + step-ca
```

При сабмите:
- Запустите `gofmt -w .` и `go vet ./...`
- Обновите соответствующие тесты
- Делайте коммиты сфокусированными и с понятными сообщениями

## Структура проекта

```
.
├── docker-compose.yml         # 3 сервиса: postgres, step-ca, step-ui
├── .env.example               # шаблон конфигурации
├── install.sh                 # установщик в одну команду
├── LICENSE                    # GPL-3.0
├── README.md                  # английская версия
├── README.ru.md               # этот файл (русская)
└── step-ui-go/
    ├── main.go                # точка входа, настройка роутера
    ├── config/                # загрузка конфига из env
    ├── db/                    # все SQL-запросы
    ├── handlers/              # HTTP-хендлеры (по файлу на раздел)
    ├── middleware/            # auth, security headers, CSRF
    ├── models/                # структуры данных
    ├── security/              # хэширование паролей, rate limiting, CSRF
    ├── templates/             # HTML-шаблоны (Go html/template)
    ├── static/                # CSS, JS, favicon, картинки
    ├── Dockerfile             # multi-stage Alpine build
    └── entrypoint.sh          # ждёт зависимости, генерит SSL, запускает app
```

## Лицензия

Проект распространяется по лицензии **GNU General Public License v3.0** — см. файл [LICENSE](LICENSE).

Кратко: вы можете использовать, изменять и распространять это ПО, но любые производные работы тоже должны быть выпущены под GPLv3.
