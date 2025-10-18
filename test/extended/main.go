package extended

import (
	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

var _ = g.Describe("[Jira:kube-scheduler][sig-kube-scheduler] sanity test", func() {
	g.It("should always pass [Suite:openshift/cluster-kube-scheduler-operator/conformance/parallel]", func() {
		o.Expect(true).To(o.BeTrue())
	})
})
