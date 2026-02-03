package e2e

import (
	"context"
	"testing"

	g "github.com/onsi/ginkgo/v2"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[Operator][Serial] should update policy configmap when source configmap changes", func() {
		testPolicyConfigMapUpdate(g.GinkgoTB())
	})
})

func testPolicyConfigMapUpdate(t testing.TB) {
	kclient, err := getKubeClient()
	if err != nil {
		t.Fatalf("failed to get kubeclient with error %v", err)
	}
	configClient, err := getOpenShiftConfigClient()
	if err != nil {
		t.Fatalf("failed to get openshift config client with error %v", err)
	}

	ctx := context.Background()

	cm, err := createSchedulerConfigMap(ctx, kclient)
	if err != nil {
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

	// Update the configmap data in openshift-config namespace.
	// Once the update in openshift-configmap namespace is successful
	// the policy configmap in openshift-kube-scheduler namespace
	// should have been updated
	cm.Data = map[string]string{
		"policy.cfg": "{\n\"kind\" : \"Policy\",\n\"apiVersion\" : \"v1\",\n\"predicates\" : [\n\t{\"name\" : \"PodFitsHostPorts\"},\n\t{\"name\" : \"PodFitsResources\"},\n\t{\"name\": \"NoDiskConflict\"},\n\t{\"name\" : \"NoVolumeZoneConflict\"},\n\t{\"name\": \"MatchNodeSelector\"},\n\t{\"name\" : \"HostName\"}\n\t],\n\"priorities\" : [\n\t{\"name\" : \"LeastRequestedPriority\", \"weight\" : 1},\n\t{\"name\" : \"ServiceSpreadingPriority\", \"weight\" : 5},\n\t{\"name\" : \"EqualPriority\", \"weight\" : 1}\n\t]\n}\n",
	}
	if _, err = kclient.CoreV1().ConfigMaps("openshift-config").Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("Configmap updating failed with error %v", err)
	}

	cm, err = kclient.CoreV1().ConfigMaps("openshift-config").Get(ctx, cm.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Configmap updating failed with error %v", err)
	}
	policyConfigMap, err := waitForPolicyConfigMapCreation(ctx, kclient, cm)
	if err != nil {
		t.Fatalf("Configmap mismatch: CM generated in openshift-kube-scheduler namespace is %v and configmap in openshift-config namespace is %v", policyConfigMap.Data["policy.cfg"], cm.Data["policy.cfg"])
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
	// We expect the configmap in openshift-kube-scheduler namespace to be deleted as well
	if err := waitForPolicyConfigMapDeletion(ctx, kclient); err != nil {
		t.Fatal("Expected policy configmap to be delete in openshift-kube-scheduler namespace but it still exists")
	}
}
