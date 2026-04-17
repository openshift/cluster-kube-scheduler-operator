package e2e

import (
	"context"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	test "github.com/openshift/cluster-kube-scheduler-operator/test/library"
)

const (
	schedulerNamespace          = "openshift-kube-scheduler"
	schedulerOperatorNamespace  = "openshift-kube-scheduler-operator"
	defaultDenyAllPolicyName    = "default-deny-all"
	schedulerPolicyName         = "installer-pruner"
	schedulerOperatorPolicyName = "kube-scheduler-operator"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[NetworkPolicy] should ensure kube scheduler NetworkPolicies are defined", func() {
		testSchedulerNetworkPolicies()
	})
	g.It("[NetworkPolicy][Serial] should restore kube scheduler NetworkPolicies after delete or mutation[Timeout:30m]", func() {
		testSchedulerNetworkPolicyReconcile()
	})
})

func testSchedulerNetworkPolicies() {
	ctx := context.Background()
	g.By("Creating Kubernetes clients")
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for kube scheduler operator to be ready")
	err = test.WaitForOperator(ctx, kubeClient)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Validating NetworkPolicies in openshift-kube-scheduler")
	schedulerDefaultDeny := getNetworkPolicy(ctx, kubeClient, schedulerNamespace, defaultDenyAllPolicyName)
	logNetworkPolicySummary("scheduler/default-deny-all", schedulerDefaultDeny)
	logNetworkPolicyDetails("scheduler/default-deny-all", schedulerDefaultDeny)
	requireDefaultDenyAll(schedulerDefaultDeny)

	schedulerPolicy := getNetworkPolicy(ctx, kubeClient, schedulerNamespace, schedulerPolicyName)
	logNetworkPolicySummary("scheduler/installer-pruner", schedulerPolicy)
	logNetworkPolicyDetails("scheduler/installer-pruner", schedulerPolicy)
	requirePodSelectorMatchExpression(schedulerPolicy, "app", []string{"installer", "pruner"})
	requireEgressPort(schedulerPolicy, corev1.ProtocolTCP, 5353)
	requireEgressPort(schedulerPolicy, corev1.ProtocolUDP, 5353)
	logEgressAllowAll(schedulerPolicy)

	g.By("Validating NetworkPolicies in openshift-kube-scheduler-operator")
	schedulerOperatorDefaultDeny := getNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, defaultDenyAllPolicyName)
	logNetworkPolicySummary("schedulerOperator/default-deny-all", schedulerOperatorDefaultDeny)
	logNetworkPolicyDetails("schedulerOperator/default-deny-all", schedulerOperatorDefaultDeny)
	requireDefaultDenyAll(schedulerOperatorDefaultDeny)

	schedulerOperatorPolicy := getNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorPolicyName)
	logNetworkPolicySummary("schedulerOperator/kube-scheduler-operator", schedulerOperatorPolicy)
	logNetworkPolicyDetails("schedulerOperator/kube-scheduler-operator", schedulerOperatorPolicy)
	requirePodSelectorLabel(schedulerOperatorPolicy, "app", "openshift-kube-scheduler-operator")
	requireIngressPort(schedulerOperatorPolicy, corev1.ProtocolTCP, 8443)
	requireEgressPort(schedulerOperatorPolicy, corev1.ProtocolTCP, 5353)
	requireEgressPort(schedulerOperatorPolicy, corev1.ProtocolUDP, 5353)
	logEgressAllowAll(schedulerOperatorPolicy)

	g.By("Verifying pods are ready in kube scheduler namespaces")
	waitForPodsReadyByLabel(ctx, kubeClient, schedulerNamespace, "app=openshift-kube-scheduler")
	waitForPodsReadyByLabel(ctx, kubeClient, schedulerNamespace, "app=guard")
	waitForPodsReadyByLabel(ctx, kubeClient, schedulerOperatorNamespace, "app=openshift-kube-scheduler-operator")
}

func testSchedulerNetworkPolicyReconcile() {
	ctx := context.Background()
	g.By("Creating Kubernetes clients")
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Waiting for kube scheduler operator to be ready")
	err = test.WaitForOperator(ctx, kubeClient)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Capturing expected NetworkPolicy specs")
	expectedSchedulerPolicy := getNetworkPolicy(ctx, kubeClient, schedulerNamespace, schedulerPolicyName)
	expectedSchedulerOperatorPolicy := getNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorPolicyName)
	expectedSchedulerOperatorDefaultDeny := getNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, defaultDenyAllPolicyName)

	g.By("Deleting main policies and waiting for restoration")
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerPolicy)
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerOperatorPolicy)

	g.By("Deleting default-deny-all policy in operator namespace and waiting for restoration")
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerOperatorDefaultDeny)

	g.By("Mutating main policies and waiting for reconciliation")
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerNamespace, schedulerPolicyName)
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorPolicyName)

	g.By("Mutating default-deny-all policy in operator namespace and waiting for reconciliation")
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, defaultDenyAllPolicyName)

	g.By("Checking NetworkPolicy-related events (best-effort)")
	logNetworkPolicyEvents(ctx, kubeClient, []string{"openshift-kube-scheduler-operator", schedulerNamespace}, schedulerPolicyName)
}
