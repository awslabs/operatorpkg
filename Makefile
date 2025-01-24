.PHONY: help
help: ## Display help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: presubmit
presubmit: verify test ## Run before submitting code

.PHONY: verify
verify: ##
	go mod tidy
	go generate ./...
	go vet ./...
	go fmt ./...

.PHONY: test
test: ##
	go test ./...