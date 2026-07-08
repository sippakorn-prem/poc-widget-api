# Widget API POC

Standalone Echo service for the deployed cross-origin widget POC. It is intentionally throwaway: no DB, no auth, no persistence, and a hardcoded demo widget key.

## Run Locally

```bash
go mod tidy
go run .
```

Defaults:

- `PORT=8080`
- `WIDGET_ORIGIN=http://localhost:5173`
- `ALLOWED_HOSTS=http://localhost:5174`

Try the happy path:

```bash
curl 'http://localhost:8080/widget/v1/bootstrap?key=wk_demo&host=http%3A%2F%2Flocalhost%3A5174'
```

Try the server-side allowlist block:

```bash
curl -i 'http://localhost:8080/widget/v1/bootstrap?key=wk_demo&host=https%3A%2F%2Fblocked.example'
```

## Deploy To Railway

1. Create a new Git repo from this folder and push it.
2. Create a Railway service from that repo. Nixpacks should detect Go from `go.mod`; no Dockerfile is needed.
3. Let Railway inject `PORT`.
4. After the widget and demo client have deployed, set:
   - `WIDGET_ORIGIN=https://<B>` where `<B>` is the Cloudflare Pages widget origin.
   - `ALLOWED_HOSTS=https://<C>` where `<C>` is the allowed demo client origin.
5. Redeploy the Railway service.

The service exposes:

- `GET /` for a basic health check.
- `GET /widget/v1/bootstrap?key=wk_demo&host=https%3A%2F%2F<client>` for widget config.

CORS allows only the widget origin plus `http://localhost*` for local testing. The customer origin is checked by the `host` query parameter so the POC matches the iframe architecture.
