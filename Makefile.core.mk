GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

BUILD_ARCH ?= linux/$(GOARCH)
OFFLINE_ARCH ?= amd64

HUB ?= ghcr.io/pluma-tools
PROD_NAME ?= pluma-operator
VERSION ?= 0.0.0-dev-$(shell git rev-parse --short=8 HEAD)

REGISTRY_USER_NAME?=
REGISTRY_PASSWORD?=

PUSH_IMAGES ?= 1
PLATFORMS ?= linux/amd64,linux/arm64

RETRY_LIMIT := 3

NPM_TOKEN ?=

OFFLINE ?= 0

CI_IMAGE_VER ?= $(UNIFIED_CI_IMAGE_VER)


gen-proto:
	make -C apis gen-proto
clean-proto:
	make -C apis clean-proto
generate:
	make -C apis generate

ctl-manifests:
	make -C apis manifests
	bash ./scripts/copy-crds.sh apis/config/crd/bases/operator.pluma.io_helmapps.yaml manifests/pluma-operator/templates

format-shell:
	shfmt -i 4 -l -w ./scripts
format-go:
	goimports -local gitlab.daocloud.cn/nicole.li/pluma-operator -w .
	gofmt -w .


format: format-go format-shell 

gen: clean-proto gen-proto generate ctl-manifests gen-client format

gen-client:
	make -C apis gen-client

gen-istio-values:
	./scripts/gen-istio-values.sh

ifeq ($(PUSH_IMAGES),1)
BUILD_CMD=buildx build --platform $(PLATFORMS) --push
else
BUILD_CMD=buildx build --platform $(PLATFORMS)
endif


define retry
	attempts=0; \
	max_attempts=$(RETRY_LIMIT); \
	while [ $$attempts -lt $$max_attempts ]; do \
		eval $(1) && break; \
		attempts=$$((attempts+1)); \
		echo "Attempt $$attempts/$$max_attempts failed. Retrying..."; \
		if [ $$attempts -eq $$max_attempts ]; then \
			echo "Command failed after $$attempts attempts."; \
			exit 1; \
		fi; \
	done
endef

build-docker:
	$(call retry, docker $(BUILD_CMD) $(DOCKER_BUILD_FLAGS) \
    		-t $(HUB)/$(PROD_NAME):$(VERSION) -f docker/Dockerfile .)

.PHONY: build-docker

release: build-docker build-chart push-chart

.PHONY: release

define in_place_replace
	yq eval $(1) $(2) -i
endef

ifeq ($(shell uname),Darwin)
SEDI=sed -i ""
else
SEDI=sed -i
endif

build-chart:
	@rm -rf dist/$(PROD_NAME) && mkdir -p dist/$(PROD_NAME)
	@cp -rf manifests/pluma-operator/. dist/$(PROD_NAME)
	$(SEDI) 's/version: .*/version: $(VERSION) # auto generated from build version/g' dist/$(PROD_NAME)/Chart.yaml
	$(call in_place_replace, '.image.tag = "$(VERSION)"', dist/$(PROD_NAME)/values.yaml)

	helm package dist/$(PROD_NAME) -d dist --version=$(VERSION)
	@rm -rf dist/$(PROD_NAME)

push-chart:
  helm push ./dist/$(PROD_NAME)-$(VERSION).tgz oci://$(HUB)/$(PROD_NAME)
