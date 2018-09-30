# The standard name for this image is openshift/origin-cluster-kube-scheduler-operator
#
FROM openshift/origin-release:golang-1.10
COPY . /go/src/github.com/openshift/cluster-kube-scheduler-operator
RUN cd /go/src/github.com/openshift/cluster-kube-scheduler-operator && go build ./cmd/cluster-kube-scheduler-operator

FROM centos:7
RUN mkdir -p /usr/share/bootkube/manifests
COPY --from=0 /go/src/github.com/openshift/cluster-kube-scheduler-operator/bindata/bootkube/* /usr/share/bootkube/manifests/
COPY --from=0 /go/src/github.com/openshift/cluster-kube-scheduler-operator/cluster-kube-scheduler-operator /usr/bin/cluster-kube-scheduler-operator

COPY manifests /manifests
LABEL io.openshift.release.operator true
