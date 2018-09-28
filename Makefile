default: help

test: ## Run all tests excluding the vendor dependencies
	gofmt -s -l .
	go vet .
	golint .
	ineffassign .
	misspell .
	gosec ./...
	gocyclo -over 15 `find . -iname '*.go' | grep -v 'vendor' | grep -v '_test.go'`
	go test -v -race .

help: ## Display available make targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[33m%-16s\033[0m %s\n", $$1, $$2}'
