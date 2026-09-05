.PHONY: build build-DecomposeTaskFunction test

# Cross-compiles all Lambda binaries.
build: build-DecomposeTaskFunction

# SAM invokes this target (Metadata.BuildMethod: makefile in template.yaml)
# for the DecomposeTaskFunction resource, with $(ARTIFACTS_DIR) set to the
# build output directory.
build-DecomposeTaskFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/skills/decompose_task

test:
	go test ./...
