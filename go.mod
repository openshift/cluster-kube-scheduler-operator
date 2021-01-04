module github.com/openshift/cluster-kube-scheduler-operator

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/openshift/api v0.0.0-20201216151826-78a19e96f9eb
	github.com/openshift/build-machinery-go v0.0.0-20200917070002-f171684f77ab
	github.com/openshift/client-go v0.0.0-20201214125552-e615e336eb49
	github.com/openshift/library-go v0.0.0-20201223214116-830765adf874
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.10.0
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/apiserver v0.20.0
	k8s.io/client-go v0.20.0
	k8s.io/component-base v0.20.0
	k8s.io/klog/v2 v2.4.0
)

replace github.com/jteeuwen/go-bindata => github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible

replace vbom.ml/util => github.com/fvbommel/util v0.0.0-20180919145318-efcd4e0f9787
