# Smart Inventory & Order Management API

REST API for managing products, inventory and orders. Built with Go + Gin + GORM.

## Stack

- Go 1.22 / Gin
- GORM with PostgreSQL (or SQLite for local dev)
- JWT auth (HS256) + bcrypt passwords
- Structured logging via `log/slog`, graceful shutdown

## Getting started

```bash
cp .env.example .env
# tweak .env if needed, then:
go mod tidy
go run ./cmd/api
```

Or with Docker:

```bash
docker compose up --build
```

For local dev without Postgres, set `DB_DRIVER=sqlite` and `DB_DSN=data.db` in `.env`.

An admin account is auto-created on first boot from the `ADMIN_EMAIL` / `ADMIN_PASSWORD` in `.env`.

Health check: `curl http://localhost:8080/health`

## Configuration

See `.env.example` for all available options. The important ones:

- `DB_DRIVER` — `postgres` (default) or `sqlite`
- `DB_DSN` — full connection string, or leave blank to build from `PG_*` vars
- `JWT_SECRET` — set something strong in production
- `ADMIN_EMAIL` / `ADMIN_PASSWORD` — bootstrap admin credentials

## API

All endpoints are JSON. Protected routes need `Authorization: Bearer <token>`.

See `api.http` for ready-to-use example requests.

### Auth

- `POST /api/v1/auth/register` — `{email, password, name}` → token + user
- `POST /api/v1/auth/login` — `{email, password}` → token + user
- `GET  /api/v1/auth/me` — current user

### Products

- `GET /api/v1/products` — public, filterable (`?material=`, `?category=`, `?q=`, `?page=`, `?size=`)
- `GET /api/v1/products/:id` — public
- `POST /api/v1/products` — admin, create (requires `sku`, `name`, `material`, `size`, `thickness_mm`, `price_cents`)
- `PUT /api/v1/products/:id` — admin, partial update
- `DELETE /api/v1/products/:id` — admin, soft delete

### Inventory

- `POST /api/v1/inventory/add` — admin, restock
- `POST /api/v1/inventory/remove` — admin, remove stock (409 if insufficient)
- `GET /api/v1/inventory/:product_id` — authenticated, view stock level
- `GET /api/v1/inventory/low-stock` — admin, products where `stock <= low_stock_at`
- `GET /api/v1/inventory/movements` — admin, audit trail

All stock ops are transactional and share one code path. Every product response includes a `low_stock` boolean.

### Orders

- `POST /api/v1/orders` — `{items: [{product_id, quantity}]}`, validates and deducts stock atomically
- `GET /api/v1/orders` — list (customers see own, admins see all)
- `GET /api/v1/orders/:id`
- `POST /api/v1/orders/:id/cancel` — restores stock
- `POST /api/v1/orders/:id/status` — admin, update status

## Quick example

```bash
# login
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"Admin123!"}' | jq -r .token)

# create a product
curl -X POST localhost:8080/api/v1/products \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"sku":"SKU-1","name":"Widget","material":"steel","size":"10x10","thickness_mm":2,"price_cents":1999,"stock":10}'
```

## Project layout

```
cmd/api/          entrypoint + graceful shutdown
config/           env-based config
db/               gorm setup + migrations
internal/
  models/         domain types
  auth/           jwt + bcrypt
  middleware/     logging, recovery, auth, rbac
  handlers/       request handlers
  httpx/          response helpers
  server/         router wiring
```

## Tests

```bash
go test ./...
```

## Notes

- Use Postgres for anything serious — SQLite is fine for dev but doesn't do row-level locking.
- CORS is wide open right now, tighten it before deploying.
- Set a real `JWT_SECRET` in production.
