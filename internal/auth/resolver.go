package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
)

// ErrNoCredentials means the request carried nothing to resolve an
// identity from — a missing bearer token, or (for LambdaJWTResolver) a
// request that somehow reached this code without having passed through
// API Gateway's Cognito JWT authorizer at all. Middleware maps this (and
// any other Resolver error) to 401, never a more specific status — the
// caller gets no information about which failure mode occurred.
var ErrNoCredentials = errors.New("no credentials in request")

// Resolver resolves the verified user ID a request is authenticated as.
type Resolver interface {
	ResolveUserID(r *http.Request) (string, error)
}

// LambdaJWTResolver is the production Resolver: it trusts the claims API
// Gateway's Cognito JWT authorizer already verified and attached to the
// Lambda proxy event, and performs no signature verification of its own
// (AGENTS.MD: "the API Lambda trusts the authorizer's claims and never
// re-verifies a signature itself"). Cognito's sub claim is used directly
// as the internal user ID (AGENTS.MD).
type LambdaJWTResolver struct{}

func (LambdaJWTResolver) ResolveUserID(r *http.Request) (string, error) {
	reqCtx, ok := core.GetAPIGatewayV2ContextFromContext(r.Context())
	if !ok || reqCtx.Authorizer == nil || reqCtx.Authorizer.JWT == nil {
		return "", ErrNoCredentials
	}

	sub, ok := reqCtx.Authorizer.JWT.Claims["sub"]
	if !ok || sub == "" {
		return "", ErrNoCredentials
	}
	return sub, nil
}

// cognitoGetUserAPI is the slice of *cognitoidentityprovider.Client
// CognitoGetUserResolver actually calls, narrowed so tests can stub it
// without a real Cognito User Pool.
type cognitoGetUserAPI interface {
	GetUser(ctx context.Context, params *cognitoidentityprovider.GetUserInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.GetUserOutput, error)
}

// CognitoGetUserResolver is cmd/local's dev-time Resolver: there's no API
// Gateway JWT authorizer in front of a plain http.Server, so it verifies
// the presented bearer token itself by asking Cognito directly whether
// it's valid (cognito-idp:GetUser) instead of reimplementing JWT
// signature verification. This still exercises the real deployed User
// Pool during local development rather than trusting a fake bypass.
type CognitoGetUserResolver struct {
	client cognitoGetUserAPI
}

// NewCognitoGetUserResolver builds a CognitoGetUserResolver backed by
// client (typically *cognitoidentityprovider.Client via NewFromConfig).
func NewCognitoGetUserResolver(client cognitoGetUserAPI) CognitoGetUserResolver {
	return CognitoGetUserResolver{client: client}
}

func (c CognitoGetUserResolver) ResolveUserID(r *http.Request) (string, error) {
	token := bearerToken(r)
	if token == "" {
		return "", ErrNoCredentials
	}

	out, err := c.client.GetUser(r.Context(), &cognitoidentityprovider.GetUserInput{AccessToken: &token})
	if err != nil {
		return "", fmt.Errorf("%w: get user: %w", ErrNoCredentials, err)
	}

	for _, attr := range out.UserAttributes {
		if attr.Name != nil && *attr.Name == "sub" && attr.Value != nil {
			return *attr.Value, nil
		}
	}
	return "", fmt.Errorf("%w: GetUser response had no sub attribute", ErrNoCredentials)
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header, or "" if the header is missing or a different scheme.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}
