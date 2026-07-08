# Widget API POC

Standalone Echo service for the deployed cross-origin widget POC. It is intentionally throwaway and no-auth, but it has a tiny JSON-backed admin UI for creating widget keys and allowed hosts.

## Run Locally

```bash
go mod tidy
go run .
```

Defaults:

- `PORT=8080`
- `WIDGET_ORIGIN=http://localhost:5173`
- `ALLOWED_HOSTS=http://localhost:5174`
- `DATA_FILE=widgets.json`
- `PUBLIC_WIDGET_SHELL_URL=http://localhost:5173/shell.js`
- `PUBLIC_API_URL` defaults to the request origin

Open the admin UI:

```text
http://localhost:8080/admin
```

Use it to create a widget:

```text
Company: Company A
Allowed hosts:
  company-a.com
  localhost:8080
```

The UI normalizes those hosts to:

```text
https://company-a.com
http://localhost:8080
```

It also generates a public widget key such as:

```text
wk_abcd5678
```

Try the seeded demo happy path:

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
   - `PUBLIC_WIDGET_SHELL_URL=https://<B>/shell.js`
   - `PUBLIC_API_URL=https://<A>` where `<A>` is this Railway API origin.
   - `ALLOWED_HOSTS=https://<C>` is optional, only used to seed `wk_demo` on first boot.
5. Redeploy the Railway service.
6. Open `https://<A>/admin` to create demo companies, widget keys, and allowed hosts.

The service exposes:

- `GET /` for a basic health check.
- `GET /admin` for the no-auth demo config UI.
- `GET /widget/v1/bootstrap?key=<widget-key>&host=https%3A%2F%2F<client>` for widget config.

CORS allows only the widget origin plus `http://localhost*` for local testing. The customer origin is checked by the `host` query parameter so the POC matches the iframe architecture.

## Demo Security Model

The widget key is public. Access is allowed only when the key and host match:

```text
wk_abcd5678 + https://company-a.com -> 200
wk_abcd5678 + https://hacker.com    -> 403
```

The POC stores widgets in `DATA_FILE` as JSON. Railway's normal filesystem is suitable for a temporary demo, but use a Railway volume or real database if you need data to survive redeploys.
