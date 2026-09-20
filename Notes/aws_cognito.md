## What is Amazon Cognito?

A managed identity service — it issues, verifies, and rotates tokens so the application doesn't have to write or run any of that itself. Two pieces matter most:

- **User Pool** — holds user identities, handles sign-up/sign-in, issues tokens.
- **Hosted UI** — a ready-made login page Cognito serves for the User Pool, so no custom login form has to be built or styled.

## Tokens Cognito issues

- **ID token** — a JWT describing who the user is (claims like `sub`, email, custom attributes). Meant for the client to read, not for authorizing API calls.
- **Access token** — a JWT the client sends to the API on every request. Short-lived, scoped to what the User Pool grants.
- **Refresh token** — long-lived, used to get new ID/access tokens without making the user log in again. Cognito owns its rotation and revocation entirely — an application built on Cognito never signs a token, never stores a refresh token, and never re-verifies a signature itself.

## Authorization code + PKCE

The OAuth2 flow both a web app and a CLI can use against Cognito's Hosted UI:

1. Client generates a random `code_verifier`, hashes it into a `code_challenge`.
2. Client sends the user to the Hosted UI with that `code_challenge`.
3. User logs in (optionally via a federated identity provider).
4. Hosted UI redirects back with a short-lived authorization `code`.
5. Client exchanges the `code` plus the original `code_verifier` for tokens.

PKCE (Proof Key for Code Exchange) exists so a *public* client — no client secret, like a CLI or a browser SPA — can't have its authorization code intercepted and exchanged by someone else. Only whoever holds the original `code_verifier` can complete the exchange.

### Why a CLI can't just use "device flow"

OAuth's device-authorization grant (RFC 8628) is the flow behind "visit https://.../device and enter this code" — the pattern the GitHub and Google CLIs use. **Cognito's Hosted UI does not support it at all.** A CLI built on Cognito instead runs a **local loopback redirect**: it starts a small HTTP server on `localhost`, opens the user's browser at the Hosted UI with `redirect_uri=http://localhost:<port>/callback`, and captures the authorization code when the browser redirects back to that local server. Functionally similar outcome (no code to hand-type), different mechanism — and it's easy to reach for "device flow" out of habit when writing CLI auth, which is exactly the mistake this project's own auth issue made before being corrected.

## API Gateway's Cognito JWT authorizer

Rather than writing middleware to verify token signatures, API Gateway can be configured with a **Cognito JWT authorizer**: it fetches Cognito's public signing keys (JWKS), validates every incoming request's access token *before* the request reaches the Lambda, and rejects invalid or expired tokens with a 401 automatically. Application code never sees a request that failed verification, and it never re-checks the signature itself — it just trusts the claims the authorizer forwards.

## Federating an external identity provider

A User Pool can let users log in via an external provider instead of (or alongside) a Cognito-native password. Two shapes that takes:

1. **Standards-based federation (OIDC or SAML)** — the User Pool trusts the external provider's discovery document and public keys directly; this is what "Login with Google" usually is. **GitHub doesn't support this** — it doesn't publish an OIDC discovery document, so it can't be registered as a Cognito OIDC (or SAML) provider out of the box.
2. **Native accounts + Lambda triggers** — the User Pool doesn't federate at the protocol level at all. The application runs its own OAuth2 exchange against the external provider's (non-OIDC) endpoints, and a Cognito **Lambda trigger** (e.g. `PreSignUp`, `PostConfirmation`, or `PreTokenGeneration`) links that verified external identity to a native Cognito user account — typically by writing the external provider's user ID into a **custom attribute** on the Cognito user.

The alternative to option 2, when a provider doesn't support option 1, is standing up a small OIDC-shim service that fakes just enough of the OIDC surface (discovery document, token endpoint) in front of the provider's real OAuth endpoints, so Cognito can federate against the shim as if it were a normal OIDC IdP. That trades "logic in a sign-in trigger" for "a second always-on service to run and secure."

## Where this project's actual decision lives

Everything above is how Cognito behaves in general, independent of this project. The specific decision — GitHub via native accounts + a sign-in trigger, not a shim — and how that trigger ties into this app's own DynamoDB identity-lookup pattern live in `architecture.md` §12 and ADR 015, and the day-to-day rules for building against it live in `AGENTS.MD`'s Auth rules section. This file is background reading, not the source of truth for what to build.
