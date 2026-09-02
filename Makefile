PROJECT      := pulumi-provider-multirotate
PACK         := multirotate
PROVIDER     := pulumi-resource-$(PACK)
MODULE       := github.com/Extrality/$(PROJECT)

# VERSION drives the plugin version, the schema version and the npm package
# version. Derived from the current git tag, falling back to 0.0.1 for dev
# builds. Override with `make VERSION=1.2.3 <target>`.
VERSION      ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || true)
VERSION      := $(if $(VERSION),$(VERSION),0.0.1)

LDFLAGS      := -X $(MODULE).version=$(VERSION)
WORKING_DIR  := $(shell pwd)
BIN          := $(WORKING_DIR)/bin
NODE_SDK     := $(WORKING_DIR)/sdk/nodejs

.PHONY: default
default: build

.PHONY: build
build: ## Build the provider plugin binary into ./bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN)/$(PROVIDER) ./cmd/$(PROVIDER)

.PHONY: test
test: ## Run the Go unit tests
	go test -race ./...

.PHONY: lint
lint: ## Vet + gofmt check
	go vet ./...
	@test -z "$$(gofmt -l . | grep -v '^sdk/')" || (gofmt -l . | grep -v '^sdk/'; echo "gofmt: files need formatting"; exit 1)

.PHONY: schema
schema: build ## Print the schema the built plugin serves
	@pulumi package get-schema $(BIN)/$(PROVIDER)

.PHONY: sdk_nodejs
sdk_nodejs: build ## Generate + compile the Node.js SDK into ./sdk/nodejs
	rm -rf $(NODE_SDK)
	pulumi package gen-sdk $(BIN)/$(PROVIDER) --language nodejs -o $(WORKING_DIR)/sdk
	cd $(NODE_SDK) && npm install --no-audit --no-fund
	cd $(NODE_SDK) && npx tsc --outDir . --sourceMap false --declarationMap false
	# The published package is the SDK directory itself, so entrypoints must be
	# relative to it (Pulumi's own layout publishes ./bin instead).
	cd $(NODE_SDK) && node -e "\
		const fs=require('fs'); \
		const p=JSON.parse(fs.readFileSync('package.json','utf8')); \
		p.main='index.js'; p.types='index.d.ts'; \
		fs.writeFileSync('package.json', JSON.stringify(p,null,4)+'\n');"
	cp $(WORKING_DIR)/LICENSE $(NODE_SDK)/LICENSE
	rm -f $(NODE_SDK)/package-lock.json
	rm -rf $(NODE_SDK)/node_modules

.PHONY: sdk
sdk: sdk_nodejs ## Generate all SDKs

.PHONY: install_plugin
install_plugin: build ## Install the built plugin into the local Pulumi plugin cache
	pulumi plugin install resource $(PACK) $(VERSION) --file $(BIN)/$(PROVIDER)

.PHONY: clean
clean:
	rm -rf $(BIN) $(WORKING_DIR)/dist $(NODE_SDK)/node_modules

.PHONY: help
help:
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'
