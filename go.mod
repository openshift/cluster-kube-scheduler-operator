module github.com/openshift/cluster-kube-scheduler-operator

go 1.13

require (
	github.com/blang/semver v3.5.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	github.com/openshift/api v0.0.0-20200210091934-a0e53e94816b
	github.com/openshift/build-machinery-go v0.0.0-20200211121458-5e3d6e570160
	github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/openshift/library-go v0.0.0-20200226171210-caa110959f91
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/common v0.6.0
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/apiserver v0.17.1
	k8s.io/client-go v0.17.1
	k8s.io/component-base v0.17.1
	k8s.io/klog v1.0.0
)

replace (
	github.com/jteeuwen/go-bindata => github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	github.com/openshift/library-go => github.com/tnozicka/library-go v0.0.0-20200303090052-264cf31d26e6
)
