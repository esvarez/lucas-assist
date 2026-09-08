.PHONY: build build-SkillsFunction build-ApiFunction build-WorkerFunction test test-integration local

# Cross-compiles all Lambda binaries.
build: build-SkillsFunction build-ApiFunction build-WorkerFunction

# SAM invokes this target (Metadata.BuildMethod: makefile in template.yaml)
# for the SkillsFunction resource, with $(ARTIFACTS_DIR) set to the build
# output directory. Consolidated entrypoint — dispatches by skill name,
# see cmd/skills/main.go.
build-SkillsFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/skills

# SAM invokes this target for the ApiFunction resource.
build-ApiFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/api

# No WorkerFunction resource in template.yaml yet — that lands with the
# Step Functions state machine wiring. This target just lets `make build`
# cross-compile cmd/worker locally in the meantime.
build-WorkerFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/worker

test:
	go test ./...

# Requires DynamoDB Local on :8000 (docker compose up -d).
test-integration:
	DYNAMODB_ENDPOINT=$${DYNAMODB_ENDPOINT:-http://localhost:8000} go test -tags integration ./...

# Runs the API as a normal http.Server on :8080. Requires DynamoDB Local
# on :8000 (docker compose up -d).
local:
	go run ./cmd/local
