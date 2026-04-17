# 🔐 Step-CA UI

Веб-интерфейс для управления [Step-CA](https://smallstep.com/docs/step-ca/) — центром сертификации для локальной сети.

Написан на **Go** + **PostgreSQL**, работает в Docker.

## Возможности

- 📋 Управление сертификатами — выпуск, перевыпуск, отзыв, импорт
- 👥 Управление пользователями с ролями (admin, manager, viewer)
- 🛡️ Безопасность — rate limiting, CSRF защита, журнал входов, блокировка IP
- 🎨 4 темы — тёмная, светлая, синяя, авто (системная)
- 📊 История операций и журнал безопасности
- 🔍 Провижионеры Step-CA

## Стек

| Компонент | Технология |
|-----------|-----------|
| Backend   | Go 1.22 + chi router |
| Frontend  | HTML/CSS (без фреймворков) |
| База данных | PostgreSQL 16 |
| CA | Smallstep step-ca |
| Деплой | Docker Compose |

## Быстрый старт

### 1. Клонируй репозиторий

```bash
git clone https://github.com/YOUR_USERNAME/step-ca-ui.git
cd step-ca-ui
```

### 2. Настрой окружение

```bash
cp .env.example .env
nano .env  # заполни своими значениями
```

### 3. Запусти

```bash
docker compose up -d --build
```

Открой `https://YOUR_IP` в браузере.

**По умолчанию:** `admin` / `Admin123!` — смени пароль сразу после входа!

## Структура проекта

```
docker-project/
├── docker-compose.yml
├── .env.example
└── step-ui-go/
    ├── main.go              # Точка входа, роутер
    ├── config/              # Конфигурация из env
    ├── db/                  # Все SQL запросы
    ├── handlers/            # HTTP хендлеры
    ├── middleware/          # Auth, security headers
    ├── models/              # Структуры данных
    ├── security/            # Пароли, CSRF, rate limiting
    ├── templates/           # HTML шаблоны
    ├── static/              # Статические файлы
    ├── Dockerfile
    └── entrypoint.sh
```

## Роли пользователей

| Роль    | Просмотр | Выпуск/Импорт | Отзыв | Пользователи |
|---------|----------|---------------|-------|--------------|
| viewer  | ✅ | ❌ | ❌ | ❌ |
| manager | ✅ | ✅ | ❌ | ❌ |
| admin   | ✅ | ✅ | ✅ | ✅ |

## Безопасность

- Rate limiting — блокировка на 15 минут после 5 неудачных попыток
- CSRF токены на всех формах
- Security headers (HSTS, CSP, X-Frame-Options и др.)
- Таймаут сессии 8 часов
- Хэширование паролей SHA-256
- Журнал всех входов с IP адресами

## Требования

- Docker и Docker Compose
- Открытый порт 443 (HTTPS)

## Лицензия

MIT
