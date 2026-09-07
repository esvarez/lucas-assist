.PHONY: build build-DecomposeTaskFunction build-ApiFunction test test-integration

# Cross-compiles all Lambda binaries.
build: build-DecomposeTaskFunction build-ApiFunction

# SAM invokes this target (Metadata.BuildMethod: makefile in template.yaml)
# for the DecomposeTaskFunction resource, with $(ARTIFACTS_DIR) set to the
# build output directory.
build-DecomposeTaskFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/skills/decompose_task

# SAM invokes this target for the ApiFunction resource.
build-ApiFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/api

test:
	go test ./...

# Requires DynamoDB Local on :8000 (docker compose up -d).
test-integration:
	DYNAMODB_ENDPOINT=$${DYNAMODB_ENDPOINT:-http://localhost:8000} go test -tags integration ./...
