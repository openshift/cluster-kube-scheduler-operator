module github.com/openshift/cluster-kube-scheduler-operator

go 1.13

require (
	github.com/blang/semver v3.5.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/openshift/api master
	github.com/openshift/build-machinery-go v0.0.0-20200713135615-1f43d26dccc7
	github.com/openshift/client-go master
	github.com/openshift/library-go master
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.19.0-rc.1
	k8s.io/apimachinery v0.19.0-rc.1
	k8s.io/apiserver v0.19.0-rc.1
	k8s.io/client-go v0.19.0-rc.1
	k8s.io/component-base v0.19.0-rc.1
	k8s.io/klog v1.0.0
)

replace github.com/jteeuwen/go-bindata => github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
