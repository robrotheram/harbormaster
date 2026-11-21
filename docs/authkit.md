# WorkOS AuthKit operator sign-in

Use WorkOS AuthKit to verify a hosted user's identity. The control plane checks project membership for project routes. Platform-wide operator functions use separate local authority.

## Configure WorkOS

1. Configure the [scoped dashboard credentials](../deploy/kubernetes/README.md#configure-hosted-dashboard-access) on the control plane and dashboard. Remove the dashboard's control token. If you adapt Compose for hosted sign-in, use an override that removes the inherited control-token setting and mount. The stock Compose deployment uses local Basic authentication.
2. Set `WORKOS_CLIENT_ID`, `WORKOS_API_KEY` or `WORKOS_API_KEY_FILE`, and `WORKOS_ALLOWED_EMAILS` on the dashboard. Use a comma-separated list of approved verified emails.
3. Set `WORKOS_REDIRECT_URI` to the dashboard origin followed by `/auth/callback`. Use HTTPS for a hosted deployment. HTTP is permitted on loopback.
4. In WorkOS, register the redirect URI, the same origin followed by `/auth/login` as the sign-in URI, and the same origin followed by `/app/` as the sign-out URI.
5. Route requests to the dashboard and preserve the public Host header. Use the same hostname in the browser and in WorkOS. Deploy one dashboard replica and verify an approved sign-in.

Incomplete WorkOS settings fail startup. Configure project membership separately. Set `PROJECT_CREATOR_EMAILS` on the control plane for approved accounts that need to create projects.

## Configure OAuth2 Proxy instead of WorkOS

Supply the same scoped dashboard credentials. Clear the WorkOS settings and configure these dashboard variables.

| Variable | Value |
| --- | --- |
| `DASHBOARD_PUBLIC_ORIGIN` | Dashboard HTTPS origin with no path |
| `OAUTH2_PROXY_PUBLIC_URL` | Public HTTPS origin for OAuth2 Proxy |
| `OAUTH2_PROXY_AUTH_URL` | Dashboard-reachable URL ending in `/oauth2/auth` |
| `OAUTH2_PROXY_ALLOWED_EMAILS` | Comma-separated list of approved verified emails |

Configure OAuth2 Proxy to return the verified email in its auth response. Check a real sign-in and verify that project access follows membership.

## WorkOS session and access boundary

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
