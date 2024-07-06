all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps.mk \
	targets/openshift/operator/telepresence.mk \
)

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
