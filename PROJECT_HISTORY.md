# Step-CA UI — история проекта

Документ фиксирует фактическое состояние проекта: зачем он появился, как устроен,
какие версии уже были выпущены, что исправлено, что развернуто на production и
какой план версий принят дальше.

Обновлено: 2026-05-26  
Текущая версия: v1.4.7  
Репозиторий: https://github.com/UncleFi1/step-ca-ui

---

## 1. Назначение проекта

Step-CA UI — self-hosted веб-интерфейс для управления приватным PKI на базе
Smallstep step-ca.

Изначальная задача: получить удобное управление внутренним центром сертификации
без внешних CA и без ручной работы через `step` CLI для каждой операции.

Проект ориентирован на:

- homelab;
- небольшие команды;
- внутренние корпоративные сети;
- локальные сервисы, которым нужны private TLS-сертификаты;
- сценарии, где не нужен публичный CA или Kubernetes cert-manager.

Проект не позиционируется как публичный CA и не является заменой полноценной
enterprise PKI-платформы.

---

## 2. Текущий стек

| Слой | Технология | Фактическое состояние |
|------|------------|------------------------|
| Backend | Go 1.22 | Один server-rendered web app |
| Router | chi | Основной HTTP-router |
| Database | PostgreSQL 16 Alpine | Метаданные пользователей, сертификатов, логов |
| CA | smallstep/step-ca | Отдельный контейнер |
| Step CLI | smallstep/cli 0.26.1 | Внутри `step-ui` image |
| Frontend | Go templates + vanilla JS + CSS | Без frontend build step |
| Deploy | Docker Compose | 3 основных контейнера |
| TLS UI | self-signed cert | Генерируется при первом запуске |
| Password hashing | bcrypt | Legacy SHA-256 мигрирует при входе |
| Sessions | gorilla/sessions | Cookie store, `SECRET_KEY` из `.env` |
| License | GPL-3.0 | Файл `LICENSE` |

---

## 3. Архитектура

```text
Browser
  |
  | HTTPS :443 или UI_HTTPS_PORT
  v
step-ui
  - Go web app
  - слушает :8443 внутри контейнера
  - выпускает/импортирует/отзывает сертификаты
  - хранит UI TLS в step-ui-ssl
  - хранит выпущенные через UI сертификаты в step-ui-certs
  - хранит runtime secret provisioner password в step-ui-data
  |
  +--> PostgreSQL :5432
  |     - users
  |     - certificates
  |     - auth_log
  |     - cert_history
  |     - le_* tables
  |
  +--> step-ca :9443
        - root/intermediate CA
        - provisioners
        - CA config
        - Badger DB
```

Контейнеры:

- `postgres`
- `step-ca`
- `step-ui`

Docker volumes:

- `postgres-data` — PostgreSQL data;
- `step-ca-data` — root/intermediate certs, CA keys, `ca.json`, step-ca DB;
- `step-ui-certs` — сертификаты, созданные через UI;
- `step-ui-ssl` — self-signed TLS сертификат самого UI;
- `step-ui-data` — runtime данные UI, включая `provisioner_password`.

Важное решение после v1.4.5: пароль provisioner для UI больше не берется из
`/home/step/secrets/password`. Этот файл нужен step-ca для intermediate key.
Для UI используется отдельный файл:

```text
/opt/step-ui/data/provisioner_password
```

---

## 4. Текущая установка

Чистая установка:

```bash
git clone https://github.com/UncleFi1/step-ca-ui.git
cd step-ca-ui
sudo ./install.sh
```

Установщик делает:

1. Проверку окружения.
2. Проверку Docker / Docker Compose.
3. Автоопределение private IP.
4. Проверку порта `443`; если занят, предлагает `8443`.
5. Генерацию секретов.
6. Запись `.env`.
7. Запись `credentials.txt`.
8. Сборку и запуск контейнеров.
9. Ожидание healthcheck.
10. Вывод URL, логина и пароля admin.

Основные переменные `.env`:

```env
HOST_IP=192.168.1.100
UI_HTTPS_PORT=443
PROVISIONER=admin
CA_PASSWORD=<generated>
SECRET_KEY=<generated>
POSTGRES_PASSWORD=<generated>
STEPUI_ADMIN_PASSWORD=<generated>
TZ=UTC
STEPCA_DEFAULT_TLS_CERT_DURATION=8760h
STEPCA_MAX_TLS_CERT_DURATION=87600h
```

---

## 5. Схема базы данных

Основные таблицы:

```text
users
certificates
auth_log
cert_history
le_certificates
le_logs
le_settings
```

Назначение:

- `users` — пользователи, роли, хэши паролей, временные аккаунты;
- `certificates` — сертификаты, выпущенные или импортированные через UI;
- `auth_log` — журнал входов;
- `cert_history` — история операций с сертификатами;
- `le_certificates` — Let's Encrypt сертификаты;
- `le_logs` — журнал Let's Encrypt операций;
- `le_settings` — настройки Let's Encrypt.

---

## 6. Роли

| Роль | Возможности |
|------|-------------|
| `viewer` | Просмотр |
| `manager` | Просмотр, выпуск, импорт, перевыпуск |
| `admin` | Все действия, включая отзыв и управление пользователями |

Временные пользователи могут иметь любую роль. Просроченные временные аккаунты
автоматически блокируются фоновой goroutine.

---

## 7. Что уже реализовано

На v1.4.7 реализовано:

- управление локальными X.509 сертификатами;
- выпуск сертификатов через step-ca;
- импорт сертификатов;
- отзыв сертификатов;
- перевыпуск сертификатов;
- список сертификатов;
- история операций;
- журнал входов;
- роли `admin`, `manager`, `viewer`;
- временные пользователи;
- кастомный date/time picker;
- 4 темы интерфейса;
- CSRF-токены и серверная проверка POST-маршрутов;
- rate limiting логина;
- IP block при неудачных входах;
- bcrypt для паролей;
- автоматическая миграция legacy SHA-256 хэшей при успешном входе;
- healthcheck для `step-ui`;
- quiet installer;
- Docker Compose deploy;
- `.env.example`;
- bilingual README;
- GPL-3.0;
- скачивание Root CA;
- скачивание Intermediate CA;
- скачивание Full Chain;
- конфигурируемый внешний HTTPS-порт через `UI_HTTPS_PORT`;
- конфигурируемая timezone через `TZ`;
- clean install с 10-летним `maxTLSCertDuration`.

---

## 8. Production-состояние на 2026-05-26

Production-сервер обновлен до v1.4.7.

Фактическое состояние после обновления:

- каталог: `/root/docker-project`;
- версия кода: `v1.4.7`;
- контейнеры `postgres`, `step-ca`, `step-ui` healthy;
- UI доступен по `https://192.168.100.102`;
- `PROVISIONER=admin@home.local` сохранен, потому что это реальное имя
  provisioner в существующем `ca.json`;
- `defaultTLSCertDuration=8760h`;
- `maxTLSCertDuration=87600h`;
- выпуск сертификата на `87600h` проверен;
- данные PostgreSQL сохранены;
- старые volumes не удалялись;
- добавлен новый volume `docker-project_step-ui-data`.

Перед production-обновлением был создан бэкап:

```text
/root/step-ca-ui-backups/20260526_010202
```

В бэкапе:

- архив файлов проекта;
- `pg_dump`;
- архивы Docker volumes:
  - `docker-project_postgres-data`;
  - `docker-project_step-ca-data`;
  - `docker-project_step-ui-certs`;
  - `docker-project_step-ui-ssl`.

---

## 9. История версий

### Initial commit

Первичная версия проекта:

- Go web app;
- PostgreSQL;
- step-ca integration;
- базовая структура handlers/db/templates/static;
- Docker-based запуск.

### v1.1.0 — большой UI-апдейт

В этой версии сформировалась основа интерфейса:

- основные страницы UI;
- управление сертификатами;
- роли пользователей;
- базовая навигация;
- темы оформления;
- журнал безопасности.

### v1.2.0 — UX-апдейт и профиль

Добавлены и улучшены:

- редактирование профиля;
- улучшения интерфейса;
- более удобная работа с пользователями и формами;
- стабилизация базовых пользовательских сценариев.

### v1.3.0 — Admin Workspace и modular CSS

Основные изменения:

- административное пространство;
- разделение CSS на модули;
- улучшение структуры шаблонов;
- подготовка к расширению admin-разделов.

### v1.4.0 — временные пользователи

Ключевая фича: временные пользователи.

Добавлено:

- поля `is_temporary`, `expires_at`, `temp_note`;
- страница `/admin/users-temp`;
- создание временных аккаунтов;
- автоэкспирация аккаунтов;
- одноразовый показ credentials через cookie;
- кастомный date/time picker;
- пресеты срока действия;
- изначальный хардкод timezone `Europe/Moscow`.

### v1.4.1 — первый публичный релиз

Проект подготовлен к публичному GitHub-релизу.

Добавлено:

- `install.sh`;
- `LICENSE` GPL-3.0;
- `README.md`;
- `README.ru.md`;
- `.env.example`;
- `test_deploy.sh`;
- `STEPUI_ADMIN_PASSWORD` для первого admin;
- публичная документация.

Исправлено:

- ошибка `step ca certificate` с лишним `--key`;
- случайное попадание `.backup-*` директорий в коммиты.

### v1.4.2 — hotfix чистой установки

Исправлено:

- порядок `InitSchema`: сначала `CREATE TABLE`, потом `ALTER TABLE`;
- автоопределение IP в `install.sh`;
- отказ от `curl ifconfig.me` как источника адреса для LAN-сценариев.

### v1.4.3 — hotfix передачи admin password

Исправлено:

- `STEPUI_ADMIN_PASSWORD` теперь реально передается в контейнер `step-ui`;
- первый admin создается с паролем из `.env` / `credentials.txt`, а не с
  fallback `Admin123!`.

### v1.4.4 — healthcheck и quiet installer

Добавлено:

- healthcheck для `step-ui`;
- более тихий `install.sh`;
- подробный install log в `/var/log/step-ca-ui-install.log`;
- `--verbose` режим;
- fallback-проверка HTTPS в `test_deploy.sh`.

Удалено:

- устаревшая compose-директива `version: "3.8"`.

### v1.4.5 — runtime configuration fixes

Исправлено:

- CRLF в `entrypoint.sh` при Windows checkout;
- `Dockerfile` нормализует line endings для entrypoint;
- дефолтный provisioner в коде/compose выровнен на `admin`;
- добавлен `step-ui-data`;
- provisioner password перенесен в `/opt/step-ui/data/provisioner_password`;
- UI больше не использует `/home/step/secrets/password` как password file.

### v1.4.6 — security hardening

Добавлено и исправлено:

- серверная CSRF-проверка для защищенных POST-маршрутов;
- admin dashboard переведен с legacy таблиц `certs` / `security_log` на
  актуальные `certificates` / `auth_log`;
- bcrypt для новых и измененных паролей;
- прозрачная миграция legacy SHA-256 password hashes при успешном входе;
- тесты для password hashing.

### v1.4.7 — clean install и CA chain fixes

Добавлено:

- `step-ca-bootstrap.sh`;
- clean install claims для provisioner:
  - `defaultTLSCertDuration=8760h`;
  - `maxTLSCertDuration=87600h`;
- `UI_HTTPS_PORT`;
- `TZ`;
- скачивание Root CA;
- скачивание Intermediate CA;
- скачивание Full Chain;
- динамический GitHub release badge;
- `sudo` в README-командах.

Исправлено:

- удалены явные `dns: 8.8.8.8 / 1.1.1.1` из compose, потому что на Linux они
  ломали Docker service discovery и `step-ui` зависал на
  `https://step-ca:9443/health`;
- install flow проверен на Ubuntu 24.04;
- production update до v1.4.7 выполнен без потери данных.

---

## 10. Закрытые проблемы

Закрыто к v1.4.7:

- чистая установка после `InitSchema`;
- корректная передача initial admin password;
- healthcheck `step-ui`;
- quiet installer;
- CRLF entrypoint issue;
- неправильный provisioner default для новых установок;
- неправильное использование `/home/step/secrets/password`;
- 24h max certificate duration на clean install;
- CSRF только на login;
- legacy admin dashboard SQL;
- SHA-256 password hashing;
- отсутствие Intermediate / Full Chain download;
- timezone hardcode;
- README без `sudo`;
- статичный release badge;
- Docker DNS override, ломавший service discovery.

---

## 11. Открытые задачи

Остается:

- `SESSION_SECURE` через env;
- полноценная страница "О системе";
- полноценная страница "Активность CA";
- Route53 в Let's Encrypt: реализовать или скрыть;
- улучшить Let's Encrypt error handling;
- уведомления email/webhook/Telegram;
- 2FA / TOTP;
- password recovery через SMTP;
- admin web console;
- SSH inventory серверов;
- SSH certificate discovery;
- ручной deploy сертификатов на серверы;
- reload/restart сервисов;
- автоматический deploy и renewal.

---

## 12. План версий дальше

### v1.4.8 — Production polish

Цель: небольшой безопасный релиз после v1.4.7.

План:

- `SESSION_SECURE=true/false` через env;
- нормальная страница "О системе";
- расширенная health/status информация в UI.

### v1.4.9 — CA activity

Цель: видимость действий внутри CA.

План:

- страница "Активность CA";
- таблицы/графики выпусков, отзывов, импортов;
- последние ошибки step-cli;
- последние auth события;
- блок "истекают скоро".

### v1.5.0 — Let's Encrypt cleanup

Цель: привести заявленные LE-функции к фактическому состоянию.

План:

- Route53: реализовать или скрыть;
- проверить `http01`;
- проверить `cloudflare`;
- проверить renew/delete/autorenew;
- улучшить ошибки LE в UI.

### v1.5.1 — Notifications

Цель: система должна сама сообщать о проблемах.

План:

- email notifications;
- webhook notifications;
- Telegram notifications;
- уведомления об истекающих сертификатах;
- уведомления об ошибках issue/renew;
- уведомления о серии failed login.

### v1.6.0 — 2FA / TOTP

Цель: усилить безопасность admin/manager.

План:

- TOTP;
- QR-код для authenticator apps;
- recovery codes;
- enforcement policy для admin;
- audit log для 2FA событий.

### v1.6.1 — Admin audit hardening

Цель: сделать критичные admin/manager действия прослеживаемыми в журнале безопасности.

Реализовано:

- audit всех admin user actions: create, delete, role change, block/unblock, reset password;
- audit создания временных пользователей;
- audit скачивания private key для локальных и Let's Encrypt сертификатов;
- audit backup downloads;
- audit changes для notification settings и Let's Encrypt settings без записи секретов;
- отдельное отображение типов событий в security log: вход, выход, 2FA, аудит, отказ.

### v1.6.2 — Password recovery

Цель: восстановление доступа без ручного SQL.

План:

- SMTP config;
- password reset tokens;
- TTL reset tokens;
- rate limit reset requests;
- audit log reset flow.

### v1.7.0 — Admin web console

Цель: ограниченная web-консоль для администрирования.

Условия:

- только admin;
- желательно только при включенной 2FA;
- allowlist команд;
- полный audit log.

### v2.0.0 — SSH server inventory

Цель: база для управления сертификатами на серверах.

План:

- добавление серверов;
- SSH connection test;
- metadata серверов;
- роли доступа к inventory.

### v2.1.0 — Certificate discovery over SSH

Цель: read-only инвентаризация сертификатов на серверах.

План:

- поиск cert/key paths;
- nginx/apache/postfix discovery;
- отображение найденных сертификатов;
- без изменения файлов.

### v2.2.0 — Manual certificate deployment

Цель: ручная замена сертификатов через UI.

План:

- deploy cert/key на сервер;
- backup старых файлов;
- dry-run;
- проверка прав и путей.

### v2.3.0 — Service restart integration

Цель: безопасный reload сервисов после замены сертификатов.

План:

- reload/restart nginx;
- reload/restart apache;
- reload/restart postfix;
- проверка конфигов перед reload;
- rollback при ошибке.

### v2.4.0 — Automatic deployment / renewal

Цель: полный цикл автоматизации.

План:

- auto-renew;
- auto-deploy;
- notifications;
- audit trail;
- per-server/per-cert policy: manual или automatic.

---

## 13. Рабочие правила проекта

Принятые правила после первых релизов:

- не смешивать крупные фичи и hotfix в одном релизе;
- перед production update всегда делать бэкап;
- не использовать `docker compose down -v` при обновлении production;
- проверять чистую установку отдельно от обновления existing stack;
- перед релизом прогонять:
  - `bash -n install.sh`;
  - `bash -n step-ca-bootstrap.sh`;
  - `docker compose config --quiet`;
  - `go test ./...`;
  - `docker compose build step-ui`;
- если релиз уже опубликован, не переписывать тег, а выпускать следующую версию.

---

## 14. Быстрые команды

Проверить состояние:

```bash
docker compose ps
```

Логи UI:

```bash
docker compose logs -f step-ui
```

Логи CA:

```bash
docker compose logs -f step-ca
```

Обновить без удаления данных:

```bash
git fetch --tags
git checkout v1.4.7
docker compose up -d --build
```

Проверить claims:

```bash
docker exec step-ca jq -r '.authority.provisioners[] | .name, .claims' /home/step/config/ca.json
```

Проверить внешний UI:

```bash
curl -k https://SERVER_IP/login
```

---

## 15. Краткое состояние

На текущий момент проект находится в рабочем production-состоянии:

- последняя версия: v1.4.7;
- production update выполнен;
- данные сохранены;
- clean install проверен;
- сертификаты на 10 лет выпускаются;
- базовая безопасность усилена;
- следующий логичный релиз: v1.4.8.
