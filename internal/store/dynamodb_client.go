package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// NewDynamoDBClient builds a DynamoDB client using the default AWS
// credential chain (IAM auth, no VPC — see architecture.md §9). Pass a
// non-empty endpoint to target DynamoDB Local for local dev and
// integration tests; leave it empty to reach the real AWS endpoint.
func NewDynamoDBClient(ctx context.Context, endpoint string) (*dynamodb.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
	}), nil
}
