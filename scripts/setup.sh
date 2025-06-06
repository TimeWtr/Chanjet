#!/bin/sh

# shellcheck disable=SC2034
SOURCE_COMMIT=./scripts/git/pre-commit
TARGET_COMMIT=.git/hooks/pre-commit
SOURCE_PUSH=./scripts/git/pre-push
TARGET_PUSH=.git/hooks/pre-push

# Copy pre-commit to git hooks if not exist.
cp $SOURCE_COMMIT $TARGET_COMMIT
# Copy pre-push to git hooks if not exist.
cp $SOURCE_PUSH $TARGET_PUSH

# Add permission to TARGET_COMMIT and TARGET_PUSH files.
test -x $TARGET_COMMIT || chmod +x $TARGET_COMMIT
test -x $TARGET_PUSH || chmod +x $TARGET_PUSH

echo "Installing golangci-lint..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

echo "Installing goimports..."
go install golang.org/x/tools/cmd/goimports@latest