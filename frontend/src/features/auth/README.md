# WorkOS AuthKit

`AuthKitProvider` loads `/auth/session` before mounting the router. It supplies
`SessionProvider` and the sidebar profile, with a hosted AuthKit sign-in action
and POST logout. A failed session request displays a retry screen, never local
access. Session changes cancel queries, clear cached data, and unmount private
views. Focus and periodic checks discover expiry or logout in another tab;
`AUTH_REQUIRED` from the management API clears private views immediately.

The browser receives only the user's ID, name, and email. API keys and access and
refresh tokens stay in the Go dashboard. See [setup](../../../../docs/authkit.md)
for environment settings, callback registration, and the access boundary.
