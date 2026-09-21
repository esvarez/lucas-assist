package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
)

// lambdaProxyRequest builds a real *http.Request the way
// httpadapter.NewV2 would from an API Gateway v2 event carrying claims —
// the same context path LambdaJWTResolver reads in production — so the
// test exercises the actual core.GetAPIGatewayV2ContextFromContext
// round-trip instead of hand-constructing an unexported context value.
func lambdaProxyRequest(t *testing.T, claims map[string]string) *http.Request {
	t.Helper()

	event := events.APIGatewayV2HTTPRequest{
		RawPath: "/projects",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET", Path: "/projects"},
		},
	}
	if claims != nil {
		event.RequestContext.Authorizer = &events.APIGatewayV2HTTPRequestContextAuthorizerDescription{
			JWT: &events.APIGatewayV2HTTPRequestContextAuthorizerJWTDescription{Claims: claims},
		}
	}

	accessor := core.RequestAccessorV2{}
	req, err := accessor.EventToRequestWithContext(context.Background(), event)
	if err != nil {
		t.Fatalf("EventToRequestWithContext() error = %v", err)
	}
	return req
}

func TestLambdaJWTResolver_ResolveUserID(t *testing.T) {
	req := lambdaProxyRequest(t, map[string]string{"sub": "user-123", "email": "a@example.com"})

	got, err := LambdaJWTResolver{}.ResolveUserID(req)
	if err != nil {
		t.Fatalf("ResolveUserID() error = %v", err)
	}
	if got != "user-123" {
		t.Errorf("ResolveUserID() = %q, want %q", got, "user-123")
	}
}

func TestLambdaJWTResolver_NoAuthorizer(t *testing.T) {
	req := lambdaProxyRequest(t, nil)

	_, err := LambdaJWTResolver{}.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}

func TestLambdaJWTResolver_MissingSubClaim(t *testing.T) {
	req := lambdaProxyRequest(t, map[string]string{"email": "a@example.com"})

	_, err := LambdaJWTResolver{}.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}

func TestLambdaJWTResolver_NotALambdaProxyRequest(t *testing.T) {
	// A plain httptest request never had EventToRequestWithContext build
	// its context, mirroring what would happen if this resolver were ever
	// used outside a real Lambda proxy invocation.
	req := httptest.NewRequest(http.MethodGet, "/projects", nil)

	_, err := LambdaJWTResolver{}.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}

// stubGetUserAPI lets a test control GetUser's response without a real
// Cognito User Pool.
type stubGetUserAPI struct {
	out *cognitoidentityprovider.GetUserOutput
	err error
}

func (s stubGetUserAPI) GetUser(ctx context.Context, params *cognitoidentityprovider.GetUserInput, optFns ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.GetUserOutput, error) {
	return s.out, s.err
}

func strPtr(s string) *string { return &s }

func TestCognitoGetUserResolver_ResolveUserID(t *testing.T) {
	resolver := NewCognitoGetUserResolver(stubGetUserAPI{
		out: &cognitoidentityprovider.GetUserOutput{
			Username: strPtr("a@example.com"),
			UserAttributes: []types.AttributeType{
				{Name: strPtr("email"), Value: strPtr("a@example.com")},
				{Name: strPtr("sub"), Value: strPtr("user-123")},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("Authorization", "Bearer a-valid-access-token")

	got, err := resolver.ResolveUserID(req)
	if err != nil {
		t.Fatalf("ResolveUserID() error = %v", err)
	}
	if got != "user-123" {
		t.Errorf("ResolveUserID() = %q, want %q", got, "user-123")
	}
}

func TestCognitoGetUserResolver_MissingAuthorizationHeader(t *testing.T) {
	resolver := NewCognitoGetUserResolver(stubGetUserAPI{})

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)

	_, err := resolver.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}

func TestCognitoGetUserResolver_GetUserFails(t *testing.T) {
	resolver := NewCognitoGetUserResolver(stubGetUserAPI{err: errors.New("NotAuthorizedException: invalid token")})

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("Authorization", "Bearer an-expired-token")

	_, err := resolver.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}

func TestCognitoGetUserResolver_NoSubAttribute(t *testing.T) {
	resolver := NewCognitoGetUserResolver(stubGetUserAPI{
		out: &cognitoidentityprovider.GetUserOutput{
			Username:       strPtr("a@example.com"),
			UserAttributes: []types.AttributeType{{Name: strPtr("email"), Value: strPtr("a@example.com")}},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	req.Header.Set("Authorization", "Bearer a-valid-access-token")

	_, err := resolver.ResolveUserID(req)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("ResolveUserID() error = %v, want ErrNoCredentials", err)
	}
}
