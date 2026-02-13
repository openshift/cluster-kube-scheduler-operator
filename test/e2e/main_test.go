package e2e

import (
	"testing"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
)

// TestE2E is the entry point for the Ginkgo test suite
func TestE2E(t *testing.T) {
	o.RegisterFailHandler(g.Fail)
	g.RunSpecs(t, "Kube Scheduler Operator E2E Suite")
}
