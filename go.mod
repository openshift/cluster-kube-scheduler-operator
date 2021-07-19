module github.com/openshift/cluster-kube-scheduler-operator

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/openshift/api v0.0.0-20210730095913-85e1d547cdee
	github.com/openshift/build-machinery-go v0.0.0-20210712174854-1bb7fd1518d3
	github.com/openshift/client-go v0.0.0-20210730113412-1811c1b3fc0e
	github.com/openshift/library-go v0.0.0-20210730114916-d82fae7e3feb
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.22.0-rc.0
	k8s.io/apimachinery v0.22.0-rc.0
	k8s.io/apiserver v0.22.0-rc.0
	k8s.io/client-go v0.22.0-rc.0
	k8s.io/component-base v0.22.0-rc.0
	k8s.io/klog/v2 v2.9.0
)
