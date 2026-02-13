package e2e

import (
	"context"
	"testing"

	g "github.com/onsi/ginkgo/v2"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[Operator][Serial] should create configmap when scheduler CR is updated", func() {
		testConfigMapCreation(g.GinkgoTB())
	})
})

func testConfigMapCreation(t testing.TB) {
	kclient, err := getKubeClient()
	if err != nil {
		t.Fatalf("failed to get kubeclient with error %v", err)
	}
	configClient, err := getOpenShiftConfigClient()
	if err != nil {
		t.Fatalf("failed to get openshift config client with error %v", err)
	}

	ctx := context.Background()

	if _, err = createSchedulerConfigMap(ctx, kclient); err != nil {
		t.Fatalf("failed creating configmap with error %v\n", err)
	}

	// Get scheduler CR
	schedulerCR, err := configClient.ConfigV1().Schedulers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)

	}
	// Update scheduler CR to the name of the config map we just created.
	schedulerCR.Spec.Policy.Name = customPolicyCMName
	if _, err = configClient.ConfigV1().Schedulers().Update(ctx, schedulerCR, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}
	// Get the latest configmap running from openshift-kube-scheduler namespace, the configmap config should be
	// updated with latest changes
	if err := waitForConfigMapUpdate(ctx, kclient); err != nil {
		t.Fatal("Timed out waiting for configmap to be updated")
	}

	// Reset scheduler CR
	schedulerCR, err = configClient.ConfigV1().Schedulers().Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)
	}
	schedulerCR.Spec = configv1.SchedulerSpec{}
	if _, err = configClient.ConfigV1().Schedulers().Update(ctx, schedulerCR, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}

	// Delete the config map that was created
	err = kclient.CoreV1().ConfigMaps("openshift-config").Delete(ctx, customPolicyCMName, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("error waiting for config map to exist: %v\n", err)
	}
}
