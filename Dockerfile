FROM registry.svc.ci.openshift.org/openshift/release:golang-1.12 AS builder
WORKDIR /go/src/github.com/openshift/cluster-kube-scheduler-operator
COPY . .
RUN go build ./cmd/cluster-kube-scheduler-operator

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base
RUN mkdir -p /usr/share/bootkube/manifests
COPY --from=builder /go/src/github.com/openshift/cluster-kube-scheduler-operator/bindata/bootkube/* /usr/share/bootkube/manifests/
COPY --from=builder /go/src/github.com/openshift/cluster-kube-scheduler-operator/cluster-kube-scheduler-operator /usr/bin/
COPY manifests /manifests
LABEL io.openshift.release.operator true
