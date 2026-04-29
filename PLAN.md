# PLAN: Reimplementing YOURLS in Go

## Проблема и цель

YOURLS — это self-hosted URL-сокращатель на PHP + MySQL. Цель — переписать на Go: единый бинарь,
**SQLite** в качестве БД, без REST API.

**Публичный доступ:** только редирект по короткой ссылке (`GET /:keyword` → 301 или 404).  
**Администратор:** логин в `/admin`, CRUD ссылок, статистика кликов.

Исходник: `u.hmdw.me/` (PHP YOURLS), схема БД: `u.hmdw.me/dump.sql`.

---

## Схема БД (SQLite, адаптирована из dump.sql)

```sql
-- Таблица ссылок
CREATE TABLE links (
  keyword    TEXT PRIMARY KEY,
  url        TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT (datetime('now')),
  ip         TEXT NOT NULL DEFAULT '',
  clicks     INTEGER NOT NULL DEFAULT 0
);

-- Лог кликов (для детальной статистики)
CREATE TABLE clicks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  keyword    TEXT NOT NULL REFERENCES links(keyword) ON DELETE CASCADE,
  clicked_at DATETIME NOT NULL DEFAULT (datetime('now')),
  referrer   TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  ip         TEXT NOT NULL DEFAULT ''
);
```

> `yourls_options` не нужна: следующий ID для keyword берётся как `MAX(rowid)+1` по таблице `links`,
> конфигурация хранится в файле.

---

## Функциональность

### 1. Публичный редирект
- `GET /:keyword` → найти URL в таблице `links`, вернуть **301**, залогировать клик асинхронно
- Если keyword не найден → **404**

### 2. Панель администратора (`/admin`)
- `GET /admin/login` + `POST /admin/login` — форма логина (bcrypt-пароль из конфига)
- `POST /admin/logout` — выход (сброс cookie)
- `GET /admin` — список ссылок: поиск, сортировка, пагинация, счётчик кликов
- `POST /admin/links` — создать ссылку (url, keyword?, title?)
- `POST /admin/links/:keyword/edit` — обновить ссылку
- `POST /admin/links/:keyword/delete` — удалить ссылку
- `GET /admin/links/:keyword/stats` — детальная статистика кликов (по дням)

Всё через server-side HTML-шаблоны + минимальный JS для UX (без SPA).

---

## Структура проекта

```
u/
├── cmd/u/main.go              # точка входа, HTTP-сервер, роутинг
├── internal/
│   ├── config/config.go       # конфиг (yaml + env override)
│   ├── db/
│   │   ├── db.go              # инициализация SQLite, миграции
│   │   ├── links.go           # CRUD для links
│   │   └── clicks.go          # запись кликов, статистика
│   ├── auth/auth.go           # bcrypt-проверка пароля, HMAC-cookie сессии
│   ├── shorturl/shorturl.go   # base36 генерация keyword
│   ├── admin/admin.go         # хендлеры /admin/*
│   └── redirect/redirect.go  # хендлер GET /:keyword
├── templates/
│   ├── base.html              # layout
│   ├── login.html
│   ├── admin.html             # список ссылок
│   └── stats.html             # статистика кликов по ссылке
├── static/                    # CSS, JS (минимальный, встраивается в бинарь)
├── go.mod
├── go.sum
└── config.yaml.example
```

---

## Конфигурация

Через `config.yaml` или переменные окружения:

```yaml
site_url:   "https://u.hmdw.me"
db_path:    "./u.db"          # путь к SQLite-файлу
cookie_key: "random-secret"  # для подписи HMAC-cookie сессий
debug:      false

admin:
  username: "rail"
  password: "$2a$08$..."     # bcrypt hash
```

---

## Зависимости Go

- `github.com/mattn/go-sqlite3` — SQLite драйвер (CGO) **или** `modernc.org/sqlite` (pure Go, без CGO)
- `golang.org/x/crypto` — bcrypt
- `github.com/go-chi/chi/v5` — роутинг
- `gopkg.in/yaml.v3` — конфиг

---

## Шаги реализации

### Шаг 1 — Инициализация проекта
- `go mod init` в `u/`
- Добавить зависимости
- Скелет `cmd/u/main.go`

### Шаг 2 — Конфиг и БД
- `internal/config`: загрузка yaml + env override
- `internal/db/db.go`: открытие SQLite, авто-миграция (CREATE TABLE IF NOT EXISTS)
- `internal/db/links.go`: GetByKeyword, Insert, Update, Delete, List (с поиском/сортировкой/пагинацией), NextID
- `internal/db/clicks.go`: InsertClick, GetStatsByKeyword (клики по дням)

### Шаг 3 — Логика keyword
- `internal/shorturl`: base36 encode (int → строка из `[0-9a-z]`), генерация keyword через NextID

### Шаг 4 — Аутентификация
- `internal/auth`: проверка bcrypt-пароля, генерация/проверка HMAC-SHA256 cookie

### Шаг 5 — Редирект
- `GET /{keyword}` → lookup → 301 + async InsertClick; 404 если не найдено

### Шаг 6 — Панель администратора
- Логин/logout
- Список ссылок (поиск, сортировка, пагинация)
- Создание, редактирование, удаление ссылки
- Страница статистики кликов по ссылке

### Шаг 7 — Шаблоны и статика
- HTML-шаблоны (base layout + страницы)
- Минимальный CSS (можно взять из `u.hmdw.me/css/`)
- `//go:embed` для статики и шаблонов

### Шаг 8 — Тесты
- Unit: base36, auth (cookie sign/verify, bcrypt)
- Integration: redirect 301/404, CRUD ссылок через HTTP

### Шаг 9 — Docker / деплой
- `Dockerfile` (multi-stage: build → alpine)
- SQLite-файл монтируется как volume

---

## Примечания

- Ключевой алгоритм генерации keyword: `base36(NextID)`, где NextID = `SELECT COALESCE(MAX(rowid), 0) + 1 FROM links` при вставке.
- Сессии — stateless (HMAC-SHA256 cookie), серверного хранилища не нужно.
- `modernc.org/sqlite` предпочтительнее `go-sqlite3`: не требует CGO, проще кросс-компиляция.
