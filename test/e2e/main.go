package e2e

import (
	"context"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	"k8s.io/klog/v2"
)

var _ = g.BeforeSuite(func() {
	kclient, err := getKubeClient()
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to instantiate clients")

	ctx := context.Background()
	// e2e test job does not guarantee our operator is up before
	// launching the test, so we need to do so.
	err = waitForOperator(ctx, kclient)
	if err != nil {
		klog.Errorf("failed waiting for operator to start: %v\n", err)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed waiting for operator to start")
	}
})
