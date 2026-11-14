# WorkOS AuthKit operator sign-in

The dashboard supports WorkOS hosted AuthKit through the official Go SDK. AuthKit
protects the existing **shared operator console**: approved users can manage all
projects. This does not add per-user or per-organization project tenancy. The
control plane and MCP retain their existing service/project bearer credentials.

## Configure

Set these in the deployment's ignored `.env` file:

```dotenv
WORKOS_CLIENT_ID=client_...
WORKOS_API_KEY=sk_...
WORKOS_ALLOWED_EMAILS=operator@example.com
WORKOS_REDIRECT_URI=http://127.0.0.1:8080/auth/callback
```

`WORKOS_ALLOWED_EMAILS` is a comma-separated allowlist. Only verified emails are
accepted; impersonated sessions are rejected. When running the dashboard directly,
`WORKOS_API_KEY_FILE` can supply the key instead of `WORKOS_API_KEY`. Never put the
key in frontend variables or commit it. Compose passes auth settings only to the
dashboard service.

In your WorkOS application's Redirects settings, register:

- Redirect URI: `http://127.0.0.1:8080/auth/callback`
- Sign-in URI: `http://127.0.0.1:8080/auth/login`
- Sign-out URI: `http://127.0.0.1:8080/app/`

Use the same host consistently; `localhost` and `127.0.0.1` are different cookie
origins. In HTTP loopback mode, entry pages on the other loopback name redirect
to the configured origin on the same port. API and callback requests still require
the configured host. For HTTPS hosting, use the public origin for all three URLs and preserve
the Host header through the trusted reverse proxy. HTTP is accepted only for
loopback development. Existing worker `.localhost` connection/preview URLs remain
local-development features; this change does not make them remotely routable.

Build and deploy the dashboard (the Go server changes as well as the UI):

```sh
docker compose build dashboard
docker compose up -d --no-deps --no-build --wait dashboard
```

All auth settings empty preserves the existing loopback local mode. Any partial
configuration fails startup; a configured deployment never falls back to local
operator access if WorkOS is unavailable.

## Session and access boundary

Sign-in uses PKCE and ten-minute, one-use state bound to an HttpOnly browser
cookie. The callback exchanges its code on the server, validates the signed token
with the application's WorkOS JWKS, checks the email allowlist, and issues a new
opaque session cookie. Cookies are host-only, HttpOnly, SameSite=Lax, and Secure
for HTTPS.

Session validation pins the issuer to
`https://api.workos.com/user_management/<WORKOS_CLIENT_ID>`, as published by the
application's `/.well-known/openid-configuration` endpoint under that URL. This
explicitly overrides the Go SDK's generic issuer default for both initial sign-in
and refresh. Custom authentication domains are not currently supported.

Sessions and sealed tokens are held only in the dashboard process, bounded to
1,024 sessions with a seven-day local maximum. A dashboard restart signs users
out. Run a single dashboard replica; shared durable session storage is required
before scaling to multiple replicas. WorkOS's access-token expiry and refresh
policy still apply. Refresh is serialized per session to avoid rotating a token
concurrently. Temporary WorkOS failures block access without destroying the
session; revoked refresh tokens require sign-in again.

All dashboard data routes and management mutations require authentication. The
static app shell, auth endpoints, and health check are public. Workspace preview,
console, and native desktop hosts keep their existing separate capability/bearer
checks. Logging out of the dashboard does not revoke already issued worker
connections or native desktop credentials.

Logout requires a same-origin POST with the application request header, revokes
the local opaque session immediately (including copied cookies), then navigates
to WorkOS's session logout endpoint to end the hosted session. User data queries
are cancelled and removed before navigation. WorkOS-side session revocation is
observed at the next token refresh; configure short access-token lifetimes in
WorkOS when prompt revocation is needed.

## Verification

`go test -race ./internal/dashboard ./cmd/dashboard` exercises PKCE, callback
state binding and replay, signature/issuer/expiry checks, verified email access,
route protection, CSRF, logout replay, and refresh serialization/failure handling.
`cd frontend && npx playwright test tests/authkit.spec.ts tests/user-profile.spec.ts`
checks signed-out gating, account display, logout, retry, expiry and local mode.
These use a synthetic WorkOS service; an actual hosted sign-in must also be
completed after configuring the WorkOS environment.

References: [AuthKit sessions](https://workos.com/docs/authkit/sessions) and
[authorization URLs](https://workos.com/docs/reference/authkit/authentication/get-authorization-url).
