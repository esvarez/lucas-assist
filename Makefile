.PHONY: build build-SkillsFunction build-ApiFunction test test-integration local cli

# Cross-compiles all Lambda binaries.
build: build-SkillsFunction build-ApiFunction

# SAM invokes this target (Metadata.BuildMethod: makefile in template.yaml)
# for the SkillsFunction resource, with $(ARTIFACTS_DIR) set to the build
# output directory. Consolidated entrypoint — dispatches by skill name,
# see cmd/skills/main.go.
build-SkillsFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/skills

# SAM invokes this target for the ApiFunction resource.
build-ApiFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/api

test:
	go test ./...

# Requires DynamoDB Local on :8000 (docker compose up -d).
test-integration:
	DYNAMODB_ENDPOINT=$${DYNAMODB_ENDPOINT:-http://localhost:8000} go test -tags integration ./...

# Runs the API as a normal http.Server on :8080. Requires DynamoDB Local
# on :8000 (docker compose up -d).
local:
	go run ./cmd/local

# Propose a project via create_project, confirm, then commit via
# POST /projects. Usage: make cli ARGS='"A CLI tool for indie developers"'
# Requires OPENAI_API_KEY (in-process propose) and, by default, cmd/local
# running for the commit step.
cli:
	go run ./cmd/cli $(ARGS)
