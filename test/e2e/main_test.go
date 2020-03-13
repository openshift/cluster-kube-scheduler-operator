package e2e

import (
	"context"
	"k8s.io/klog"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	kclient, err := getKubeClient()
	if err != nil {
		klog.Errorf("error while instantiating clients with %v", err)
		os.Exit(1)
	}
	ctx := context.Background()
	// e2e test job does not guarantee our operator is up before
	// launching the test, so we need to do so.
	err = waitForOperator(ctx, kclient)
	if err != nil {
		klog.Errorf("failed waiting for operator to start: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
