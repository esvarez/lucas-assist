package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// localRegion is used only when targeting DynamoDB Local, which ignores
// region for anything but SigV4 signing.
const localRegion = "us-east-1"

// NewLocalDynamoDBClient builds a DynamoDB client targeting DynamoDB Local,
// for cmd/local and integration tests. If endpoint is empty, it defaults to
// "http://localhost:8000". It uses static dummy credentials and a fixed
// region instead of the default AWS credential chain — DynamoDB Local
// doesn't validate them, and requiring real AWS credentials for local dev
// would be a needless barrier (and, without them, the default chain falls
// through to EC2 IMDS and hangs off-EC2).
func NewLocalDynamoDBClient(ctx context.Context, endpoint string) (*dynamodb.Client, error) {
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(localRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("local", "local", "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	}), nil
}

// NewDynamoDBClient builds a DynamoDB client targeting the real AWS
// endpoint, resolved via the default AWS credential chain (IAM auth, no
// VPC — see architecture.md §9). This is what cmd/api uses.
func NewDynamoDBClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return dynamodb.NewFromConfig(cfg), nil
}
