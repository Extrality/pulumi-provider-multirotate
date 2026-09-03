PROJECT      := pulumi-provider-multirotate
PACK         := multirotate
PROVIDER     := pulumi-resource-$(PACK)
MODULE       := github.com/Extrality/$(PROJECT)

# Bootstrap value, used only if the SDK has never been generated.
DEV_VERSION  := 0.0.1

# The version stamped into the SDK that is currently committed. Read at parse
# time (`:=`) because sdk_nodejs deletes the directory before regenerating.
# Deriving the expected version from the tree itself -- rather than from
# `git describe` -- is what makes `sdk_check` reproducible: it needs no tags
# fetched, and it still holds on the release PR, where the version has been
# bumped but the tag does not exist yet.
SDK_VERSION  := $(shell node -p "require('./sdk/nodejs/package.json').version" 2>/dev/null || echo $(DEV_VERSION))

# VERSION drives the plugin version, the schema version and the npm package
# version. It deliberately does *not* derive from `git describe`: that made
# `make sdk` produce different bytes depending on which tags you had fetched.
# Defaulting to SDK_VERSION keeps a plain `make sdk` idempotent on a clean tree,
# and keeps a locally built plugin version-matched with the local SDK.
# The release workflow passes it explicitly.
VERSION      ?= $(SDK_VERSION)

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

.PHONY: sdk_check
sdk_check: ## Fail if the committed SDK is not what the schema generates (CI gate)
	@$(MAKE) --no-print-directory sdk VERSION=$(SDK_VERSION)
	@git add --intent-to-add -A sdk
	@git diff --exit-code -- sdk || { \
		printf '\nsdk/ is out of date at version $(SDK_VERSION).\n'; \
		printf 'Run `make sdk` and commit the result.\n'; \
		exit 1; \
	}

.PHONY: install_plugin
install_plugin: build ## Install the built plugin into the local Pulumi plugin cache
	pulumi plugin install resource $(PACK) $(VERSION) --file $(BIN)/$(PROVIDER)

.PHONY: clean
clean:
	rm -rf $(BIN) $(WORKING_DIR)/dist $(NODE_SDK)/node_modules

.PHONY: help
help:
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'
