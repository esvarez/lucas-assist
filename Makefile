.PHONY: build build-SkillsFunction build-ApiFunction build-WorkerFunction test test-integration local deploy-web

# Override on the command line to target a different stack, e.g.
# `make deploy-web ENVIRONMENT=prod STACK_NAME=nudge-prod`.
ENVIRONMENT ?= dev
STACK_NAME ?= nudge-$(ENVIRONMENT)

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

# SAM invokes this target for the WorkerFunction resource (#101): consumes
# AgentJobsQueue, calls the model, saves the resulting Changeset.
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

# Builds the SPA and syncs it to WebBucket (#71), then invalidates
# index.html so CloudFront never serves a stale shell after a deploy.
# Assumes `sam deploy --stack-name $(STACK_NAME)` already provisioned
# WebBucket/WebDistribution — this only pushes web/dist's contents, it
# doesn't touch the SAM stack itself.
#
# Cache-Control is set here, not in the CloudFront distribution config:
# Managed-CachingOptimized (the default cache behavior's policy) honors
# whatever each object's own Cache-Control header says, so hashed assets
# (immutable, cache them for a year) and index.html (never cache, or a
# stale shell could reference asset hashes that no longer exist) need
# different values at upload time.
deploy-web:
	npm --prefix web run build
	$(eval WEB_BUCKET := $(shell aws cloudformation describe-stacks --stack-name $(STACK_NAME) --query "Stacks[0].Outputs[?OutputKey=='WebBucketName'].OutputValue" --output text))
	$(eval DISTRIBUTION_ID := $(shell aws cloudformation describe-stacks --stack-name $(STACK_NAME) --query "Stacks[0].Outputs[?OutputKey=='CloudFrontDistributionId'].OutputValue" --output text))
	aws s3 sync web/dist s3://$(WEB_BUCKET) --delete --cache-control "public,max-age=31536000,immutable" --exclude index.html
	aws s3 cp web/dist/index.html s3://$(WEB_BUCKET)/index.html --cache-control "no-cache"
	aws cloudfront create-invalidation --distribution-id $(DISTRIBUTION_ID) --paths "/index.html"
