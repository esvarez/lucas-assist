// Command api is the API Lambda: all CRUD and changeset commits behind a
// plain net/http.ServeMux, adapted for API Gateway HTTP API (v2) via
// awslabs/aws-lambda-go-api-proxy's httpadapter. Handlers are plain
// net/http so they run unchanged under cmd/local. No VPC — see
// architecture.md §9.
package main

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/esvarez/lucas-assist/internal/api"
	"github.com/esvarez/lucas-assist/internal/store"
)

// adapter is constructed once at package init and reused across
// invocations — never inside the handler.
var adapter *httpadapter.HandlerAdapterV2

func init() {
	ctx := context.Background()

	client, err := store.NewDynamoDBClient(ctx, os.Getenv("DYNAMODB_ENDPOINT"))
	if err != nil {
		log.Fatalf("new dynamodb client: %v", err)
	}

	repo := store.NewDynamoRepository(client, os.Getenv("DYNAMODB_TABLE"))
	adapter = httpadapter.NewV2(api.NewRouter(repo))
}

func handler(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return adapter.ProxyWithContext(ctx, req)
}

func main() {
	lambda.Start(handler)
}
