# Smart Inventory API

Go REST API for managing products, inventory and orders.

## Quick start (local)

```bash
cp .env.example .env
go mod tidy
go run ./cmd/api
```

Health check: `curl http://localhost:8080/health`

## Deploy on Render (free)

1. Push this repo to GitHub
2. Go to [render.com](https://render.com) → "New +" → "Blueprint"
3. Connect your GitHub repo
4. Render reads `render.yaml` and deploys automatically

The blueprint provisions:
- A Go web service running the API
- A persistent disk for SQLite data

After deploy, check the **Environment** tab for the auto-generated `JWT_SECRET` and `ADMIN_PASSWORD`, or set your own before first boot.

## API

All endpoints are JSON. Protected routes need `Authorization: Bearer <token>`.

See `api.http` for example requests.

### Auth
- `POST /api/v1/auth/register` — `{email, password, name}` → token + user
- `POST /api/v1/auth/login` — `{email, password}` → token + user
- `GET  /api/v1/auth/me` — current user

### Products
- `GET /api/v1/products` — public, filterable
- `GET /api/v1/products/:id` — public
- `POST /api/v1/products` — admin, create
- `PUT /api/v1/products/:id` — admin, partial update
- `DELETE /api/v1/products/:id` — admin, soft delete

### Inventory
- `POST /api/v1/inventory/add` — admin, restock
- `POST /api/v1/inventory/remove` — admin, remove stock (409 if insufficient)
- `GET /api/v1/inventory/:product_id` — authenticated, view stock level
- `GET /api/v1/inventory/low-stock` — admin, products where `stock <= low_stock_at`

### Orders
- `POST /api/v1/orders` — `{items: [{product_id, quantity}]}`
- `GET /api/v1/orders` — list
- `GET /api/v1/orders/:id`
- `POST /api/v1/orders/:id/cancel` — restores stock
- `POST /api/v1/orders/:id/status` — admin, update status

## Tests

```bash
go test ./...
```

## Notes

- SQLite is fine for dev / small deploys. Switch to Postgres by setting `DB_DRIVER=postgres` and `DB_DSN`.
- CORS is open — tighten for production.
- Set a real `JWT_SECRET` in production.
