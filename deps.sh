#!/usr/bin/env bash

GO_BIN_PATH="$(go env GOPATH)/bin"
if [[ ":$PATH:" == *":$GO_BIN_PATH:"* ]]; then
  echo "Your path is correctly set"
else
  echo "Your path is missing ~/bin, you might want to add it."
fi

go install github.com/go-task/task/v3@v3.41.0
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6
