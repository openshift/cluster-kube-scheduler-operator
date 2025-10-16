all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps.mk \
	targets/openshift/operator/telepresence.mk \
)

# Exclude tests-ext from the main build target
GO_BUILD_PACKAGES :=./cmd/cluster-kube-scheduler-operator ./cmd/render

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/... ./bindata/...

IMAGE_REGISTRY :=registry.svc.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
$(call build-image,ocp-cluster-kube-scheduler-operator,$(IMAGE_REGISTRY)/ocp/4.2:cluster-kube-scheduler-operator, ./Dockerfile.ocp,.)

$(call verify-golang-versions,Dockerfile.ocp)

e2e: GO_TEST_PACKAGES :=./test/e2e
e2e: test-unit
.PHONY: e2e

test-e2e-preferred-host: GO_TEST_PACKAGES :=./test/e2e-preferred-host/...
test-e2e-preferred-host: GO_TEST_FLAGS += -timeout 1h
test-e2e-preferred-host: test-unit
.PHONY: test-e2e-preferred-host

clean:
	$(RM) ./cluster-kube-scheduler-operator
.PHONY: clean

# Configure the 'telepresence' target
# See vendor/github.com/openshift/build-machinery-go/scripts/run-telepresence.sh for usage and configuration details
export TP_DEPLOYMENT_YAML ?=./manifests/0000_25_kube-scheduler-operator_06_deployment.yaml
export TP_CMD_PATH ?=./cmd/cluster-kube-scheduler-operator

# OpenShift Tests Extension variables
TESTS_EXT_BINARY ?= cluster-kube-scheduler-operator-tests-ext
TESTS_EXT_PACKAGE ?= ./cmd/cluster-kube-scheduler-operator-tests-ext
TESTS_EXT_LDFLAGS ?= -X 'main.CommitFromGit=$(shell git rev-parse --short HEAD)' \
                     -X 'main.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' \
                     -X 'main.GitTreeState=$(shell if git diff-index --quiet HEAD --; then echo clean; else echo dirty; fi)'

# Build the openshift-tests-extension binary
.PHONY: tests-ext-build
tests-ext-build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) GO_COMPLIANCE_POLICY=exempt_all CGO_ENABLED=0 \
        go build -o $(TESTS_EXT_BINARY) -ldflags "$(TESTS_EXT_LDFLAGS)" $(TESTS_EXT_PACKAGE)

# Update test metadata
.PHONY: tests-ext-update
tests-ext-update:
	./$(TESTS_EXT_BINARY) update

# Clean tests extension artifacts
.PHONY: tests-ext-clean
tests-ext-clean:
	rm -f $(TESTS_EXT_BINARY) $(TESTS_EXT_BINARY).gz

# Run tests extension help
.PHONY: tests-ext-help
tests-ext-help:
	./$(TESTS_EXT_BINARY) --help


# Run sanity test
.PHONY: tests-ext-sanity
tests-ext-sanity:
	./$(TESTS_EXT_BINARY) run-suite "openshift/cluster-kube-scheduler-operator/conformance/parallel"

# List available tests
.PHONY: tests-ext-list
tests-ext-list:
	./$(TESTS_EXT_BINARY) list tests

# Show extension info
.PHONY: tests-ext-info
tests-ext-info:
	./$(TESTS_EXT_BINARY) info
