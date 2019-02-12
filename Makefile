all: build
.PHONY: all

# Codegen module needs setting these required variables
CODEGEN_OUTPUT_PACKAGE :=github.com/openshift/cluster-kube-scheduler-operator/pkg/generated
CODEGEN_API_PACKAGE :=github.com/openshift/cluster-kube-scheduler-operator/pkg/apis
CODEGEN_GROUPS_VERSION :=kubescheduler:v1alpha1

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/library-go/alpha-build-machinery/make/, \
	operator.mk \
)

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - Dockerfile path
# $3 - context directory for image build
# It will generate target "image-$(1)" for builing the image an binding it as a prerequisite to target "images".
$(call build-image,origin-$(GO_PACKAGE),./Dockerfile,.)

# This will call a macro called "add-bindata" which will generate bindata specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - input dirs
# $3 - prefix
# $4 - pkg
# $5 - output
# It will generate targets {update,verify}-bindata-$(1) logically grouping them in unsuffixed versions of these targets
# and also hooked into {update,verify}-generated for broader integration.
$(call add-bindata,v3.11.0,./bindata/v3.11.0/...,bindata,v311_00_assets,pkg/operator/v311_00_assets/bindata.go)

e2e: GO_TEST_PACKAGES :=./test/e2e
e2e: test-unit
.PHONY: e2e

clean:
	$(RM) ./cluster-kube-scheduler-operator
.PHONY: clean

OUTPUT_CRD=kubescheduler_v1alpha1_kubescheduleroperatorconfig.yaml
CRD_MANIFEST=0000_11_kube-scheduler-operator_01_config.crd.yaml
update-crds:
	go get sigs.k8s.io/controller-tools/cmd/crd
	crd generate --domain operator.openshift.io --output-dir manifests/
	sed -i '/creationTimestamp/d' manifests/$(OUTPUT_CRD)
	mv manifests/$(OUTPUT_CRD) manifests/$(CRD_MANIFEST)

TMP_DIR:=$(shell mktemp -d)
verify-crds:
	go get sigs.k8s.io/controller-tools/cmd/crd
	crd generate --domain operator.openshift.io --output-dir $(TMP_DIR)
	sed -i '/creationTimestamp/d' $(TMP_DIR)/$(OUTPUT_CRD)
	if cmp -s $(TMP_DIR)/$(OUTPUT_CRD) manifests/$(CRD_MANIFEST); then \
		echo "verify-crds: OK"; \
	else \
		echo "CRDs not updated. Please run: make update-crds"; \
		exit 1; \
	fi
