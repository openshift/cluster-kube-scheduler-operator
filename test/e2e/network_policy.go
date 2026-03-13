package e2e

import (
	"context"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	test "github.com/openshift/cluster-kube-scheduler-operator/test/library"
)

const (
	schedulerNamespace            = "openshift-kube-scheduler"
	schedulerOperatorNamespace    = "openshift-kube-scheduler-operator"
	defaultDenyAllPolicyName      = "default-deny-all"
	schedulerPolicyName           = "installer-pruner"
	schedulerOperatorPolicyName   = "kube-scheduler-operator"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[NetworkPolicy][Disruptive][Serial] should ensure kube scheduler NetworkPolicies are defined", func() {
		testSchedulerNetworkPolicies()
	})
	g.It("[NetworkPolicy][Disruptive][Serial] should restore kube scheduler NetworkPolicies after delete or mutation[Timeout:30m]", func() {
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
	logNetworkPolicySummary("schedulerOperator/kube-scheduler-operator",schedulerOperatorPolicy)
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
	g.GinkgoWriter.Printf("deleting NetworkPolicy %s/%s\n", schedulerNamespace, schedulerPolicyName)
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerPolicy)
	g.GinkgoWriter.Printf("deleting NetworkPolicy %s/%s\n", schedulerOperatorNamespace, schedulerOperatorPolicyName)
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerOperatorPolicy)

	g.By("Deleting default-deny-all policy in operator namespace and waiting for restoration")
	g.GinkgoWriter.Printf("deleting NetworkPolicy %s/%s\n", schedulerOperatorNamespace, defaultDenyAllPolicyName)
	restoreNetworkPolicy(ctx, kubeClient, expectedSchedulerOperatorDefaultDeny)

	g.By("Mutating main policies and waiting for reconciliation")
	g.GinkgoWriter.Printf("mutating NetworkPolicy %s/%s\n", schedulerNamespace, schedulerPolicyName)
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerNamespace, schedulerPolicyName)
	g.GinkgoWriter.Printf("mutating NetworkPolicy %s/%s\n", schedulerOperatorNamespace, schedulerOperatorPolicyName)
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorPolicyName)

	g.By("Mutating default-deny-all policy in operator namespace and waiting for reconciliation")
	g.GinkgoWriter.Printf("mutating NetworkPolicy %s/%s\n",schedulerOperatorNamespace, defaultDenyAllPolicyName)
	mutateAndRestoreNetworkPolicy(ctx, kubeClient, schedulerOperatorNamespace, defaultDenyAllPolicyName)

	g.By("Checking NetworkPolicy-related events (best-effort)")
	logNetworkPolicyEvents(ctx, kubeClient, []string{"openshift-kube-scheduler-operator", schedulerNamespace}, schedulerPolicyName)
}

func getNetworkPolicy(ctx context.Context, client kubernetes.Interface, namespace, name string) *networkingv1.NetworkPolicy {
	g.GinkgoHelper()
	policy, err := client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to get NetworkPolicy %s/%s", namespace, name)
	return policy
}

func requireDefaultDenyAll(policy *networkingv1.NetworkPolicy) {
	g.GinkgoHelper()
	if len(policy.Spec.PodSelector.MatchLabels) != 0 || len(policy.Spec.PodSelector.MatchExpressions) != 0 {
		g.Fail(fmt.Sprintf("%s/%s: expected empty podSelector", policy.Namespace, policy.Name))
	}
	if len(policy.Spec.Ingress) != 0 || len(policy.Spec.Egress) != 0 {
		g.Fail(fmt.Sprintf("%s/%s: expected no ingress or egress rules, got ingress=%d egress=%d", policy.Namespace, policy.Name, len(policy.Spec.Ingress), len(policy.Spec.Egress)))
	}

	policyTypes := sets.NewString()
	for _, policyType := range policy.Spec.PolicyTypes {
		policyTypes.Insert(string(policyType))
	}
	if !policyTypes.Has(string(networkingv1.PolicyTypeIngress)) || !policyTypes.Has(string(networkingv1.PolicyTypeEgress)) {
		g.Fail(fmt.Sprintf("%s/%s: expected both Ingress and Egress policyTypes, got %v", policy.Namespace, policy.Name, policy.Spec.PolicyTypes))
	}
}

func requirePodSelectorLabel(policy *networkingv1.NetworkPolicy, key, value string) {
	g.GinkgoHelper()
	actual, ok := policy.Spec.PodSelector.MatchLabels[key]
	if !ok || actual != value {
		g.Fail(fmt.Sprintf("%s/%s: expected podSelector %s=%s, got %v", policy.Namespace, policy.Name, key, value, policy.Spec.PodSelector.MatchLabels))
	}
}

func requirePodSelectorMatchExpression(policy *networkingv1.NetworkPolicy, key string, values []string) {
	g.GinkgoHelper()
	found := false
	for _, expr := range policy.Spec.PodSelector.MatchExpressions {
		if expr.Key == key && expr.Operator == metav1.LabelSelectorOpIn {
			if sets.NewString(expr.Values...).Equal(sets.NewString(values...)) {
				found = true
				break
			}
		}
	}
	if !found {
		g.Fail(fmt.Sprintf("%s/%s: expected podSelector matchExpression %s In %v, got %v", policy.Namespace, policy.Name, key, values, policy.Spec.PodSelector.MatchExpressions))
	}
}

func requireIngressPort(policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	g.GinkgoHelper()
	if !hasPortInIngress(policy.Spec.Ingress, protocol, port) {
		g.Fail(fmt.Sprintf("%s/%s: expected ingress port %s/%d", policy.Namespace, policy.Name, protocol, port))
	}
}

func requireIngressFromNamespace(policy *networkingv1.NetworkPolicy, port int32, namespace string) {
	g.GinkgoHelper()
	if !hasIngressFromNamespace(policy.Spec.Ingress, port, namespace) {
		g.Fail(fmt.Sprintf("%s/%s: expected ingress from namespace %s on port %d", policy.Namespace, policy.Name, namespace, port))
	}
}

func logIngressFromNamespaceOptional(policy *networkingv1.NetworkPolicy, port int32, namespace string) {
	g.GinkgoHelper()
	if hasIngressFromNamespace(policy.Spec.Ingress, port, namespace) {
		g.GinkgoWriter.Printf("networkpolicy %s/%s: ingress from namespace %s present on port %d\n", policy.Namespace, policy.Name, namespace, port)
		return
	}
	g.GinkgoWriter.Printf("networkpolicy %s/%s: no ingress from namespace %s on port %d\n", policy.Namespace, policy.Name, namespace, port)
}

func requireIngressFromNamespaceOrPolicyGroup(policy *networkingv1.NetworkPolicy, port int32, namespace, policyGroupLabelKey string) {
	g.GinkgoHelper()
	if hasIngressFromNamespace(policy.Spec.Ingress, port, namespace) {
		return
	}
	if hasIngressFromPolicyGroup(policy.Spec.Ingress, port, policyGroupLabelKey) {
		return
	}
	g.Fail(fmt.Sprintf("%s/%s: expected ingress from namespace %s or policy-group %s on port %d", policy.Namespace, policy.Name, namespace, policyGroupLabelKey, port))
}

func requireIngressAllowAll(policy *networkingv1.NetworkPolicy, port int32) {
	g.GinkgoHelper()
	if !hasIngressAllowAll(policy.Spec.Ingress, port) {
		g.Fail(fmt.Sprintf("%s/%s: expected ingress allow-all on port %d", policy.Namespace, policy.Name, port))
	}
}

func logIngressHostNetworkOrAllowAll(policy *networkingv1.NetworkPolicy, port int32) {
	g.GinkgoHelper()
	if hasIngressAllowAll(policy.Spec.Ingress, port) {
		g.GinkgoWriter.Printf("networkpolicy %s/%s: ingress allow-all present on port %d\n", policy.Namespace, policy.Name, port)
		return
	}
	if hasIngressFromPolicyGroup(policy.Spec.Ingress, port, "policy-group.network.openshift.io/host-network") {
		g.GinkgoWriter.Printf("networkpolicy %s/%s: ingress host-network policy-group present on port %d\n", policy.Namespace, policy.Name, port)
		return
	}
	g.GinkgoWriter.Printf("networkpolicy %s/%s: no ingress allow-all/host-network rule on port %d\n", policy.Namespace, policy.Name, port)
}

func requireEgressPort(policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	g.GinkgoHelper()
	if !hasPortInEgress(policy.Spec.Egress, protocol, port) {
		g.Fail(fmt.Sprintf("%s/%s: expected egress port %s/%d", policy.Namespace, policy.Name, protocol, port))
	}
}

func hasPortInIngress(rules []networkingv1.NetworkPolicyIngressRule, protocol corev1.Protocol, port int32) bool {
	for _, rule := range rules {
		if hasPort(rule.Ports, protocol, port) {
			return true
		}
	}
	return false
}

func hasPortInEgress(rules []networkingv1.NetworkPolicyEgressRule, protocol corev1.Protocol, port int32) bool {
	for _, rule := range rules {
		if hasPort(rule.Ports, protocol, port) {
			return true
		}
	}
	return false
}

func hasPort(ports []networkingv1.NetworkPolicyPort, protocol corev1.Protocol, port int32) bool {
	for _, p := range ports {
		if p.Port == nil || p.Port.IntValue() != int(port) {
			continue
		}
				actualProtocol := corev1.ProtocolTCP
		if p.Protocol != nil {
			actualProtocol = *p.Protocol
		}
		if actualProtocol == protocol {
			return true
		}
	}
	return false
}

func hasIngressFromNamespace(rules []networkingv1.NetworkPolicyIngressRule, port int32, namespace string) bool {
	for _, rule := range rules {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		for _, peer := range rule.From {
			if namespaceSelectorMatches(peer.NamespaceSelector, namespace) {
				return true
			}
		}
	}
	return false
}

func hasIngressAllowAll(rules []networkingv1.NetworkPolicyIngressRule, port int32) bool {
	for _, rule := range rules {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		if len(rule.From) == 0 {
			return true
		}
	}
	return false
}

func namespaceSelectorMatches(selector *metav1.LabelSelector, namespace string) bool {
	if selector == nil {
		return false
	}
	if selector.MatchLabels != nil {
		if selector.MatchLabels["kubernetes.io/metadata.name"] == namespace {
			return true
		}
	}
	for _, expr := range selector.MatchExpressions {
		if expr.Key != "kubernetes.io/metadata.name" {
			continue
		}
		if expr.Operator != metav1.LabelSelectorOpIn {
			continue
		}
		for _, value := range expr.Values {
			if value == namespace {
				return true
			}
		}
	}
	return false
}

func hasIngressFromPolicyGroup(rules []networkingv1.NetworkPolicyIngressRule, port int32, policyGroupLabelKey string) bool {
	for _, rule := range rules {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		for _, peer := range rule.From {
			if peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels == nil {
				continue
			}
			if _, ok := peer.NamespaceSelector.MatchLabels[policyGroupLabelKey]; ok {
				return true
			}
		}
	}
	return false
}

func logEgressAllowAllTCP(policy *networkingv1.NetworkPolicy) {
	g.GinkgoHelper()
	if hasEgressAllowAllTCP(policy.Spec.Egress) {
		g.GinkgoWriter.Printf("networkpolicy %s/%s: egress allow-all TCP rule present\n", policy.Namespace, policy.Name)
		return
	}
	g.GinkgoWriter.Printf("networkpolicy %s/%s: no egress allow-all TCP rule\n", policy.Namespace, policy.Name)
}

func logEgressAllowAll(policy *networkingv1.NetworkPolicy) {
	g.GinkgoHelper()
	if hasEgressAllowAll(policy.Spec.Egress) {
		g.GinkgoWriter.Printf("networkpolicy %s/%s: egress allow-all rule present\n", policy.Namespace, policy.Name)
		return
	}
	g.GinkgoWriter.Printf("networkpolicy %s/%s: no egress allow-all rule\n", policy.Namespace, policy.Name)
}

func hasEgressAllowAllTCP(rules []networkingv1.NetworkPolicyEgressRule) bool {
	for _, rule := range rules {
		if len(rule.To) != 0 {
			continue
		}
		if hasAnyTCPPort(rule.Ports) {
			return true
		}
	}
	return false
}

func hasAnyTCPPort(ports []networkingv1.NetworkPolicyPort) bool {
	if len(ports) == 0 {
		return true
	}
	for _, p := range ports {
		if p.Protocol != nil && *p.Protocol != corev1.ProtocolTCP {
			continue
		}
		return true
	}
	return false
}

func restoreNetworkPolicy(ctx context.Context, client kubernetes.Interface, expected *networkingv1.NetworkPolicy) {
	g.GinkgoHelper()
	namespace := expected.Namespace
	name := expected.Name
	g.GinkgoWriter.Printf("deleting NetworkPolicy %s/%s\n", namespace, name)
	o.Expect(client.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})).NotTo(o.HaveOccurred())
	err := wait.PollImmediate(5*time.Second, 20*time.Minute, func() (bool, error) {
		current, err := client.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return equality.Semantic.DeepEqual(expected.Spec, current.Spec), nil
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "timed out waiting for NetworkPolicy %s/%s spec to be restored", namespace, name)
	g.GinkgoWriter.Printf("NetworkPolicy %s/%s spec restored after delete\n", namespace, name)
}

func mutateAndRestoreNetworkPolicy(ctx context.Context, client kubernetes.Interface, namespace, name string) {
	g.GinkgoHelper()
	original := getNetworkPolicy(ctx, client, namespace, name)
	g.GinkgoWriter.Printf("mutating NetworkPolicy %s/%s (podSelector override)\n", namespace, name)
	patch := []byte(`{"spec":{"podSelector":{"matchLabels":{"np-reconcile":"mutated"}}}}`)
	_, err := client.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	err = wait.PollImmediate(5*time.Second, 20*time.Minute, func() (bool, error) {
		current := getNetworkPolicy(ctx, client, namespace, name)
		return equality.Semantic.DeepEqual(original.Spec, current.Spec), nil
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "timed out waiting for NetworkPolicy %s/%s spec to be restored", namespace, name)
	g.GinkgoWriter.Printf("NetworkPolicy %s/%s spec restored\n", namespace, name)
}

func waitForPodsReadyByLabel(ctx context.Context, client kubernetes.Interface, namespace, labelSelector string) {
	g.GinkgoHelper()
	g.GinkgoWriter.Printf("waiting for pods ready in %s with selector %s\n", namespace, labelSelector)
	err := wait.PollImmediate(5*time.Second, 5*time.Minute, func() (bool, error) {
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			return false, err
		}
		if len(pods.Items) == 0 {
			return false, nil
		}
		for _, pod := range pods.Items {
			if !isPodReady(&pod) {
				return false, nil
			}
		}
		return true, nil
	})
	o.Expect(err).NotTo(o.HaveOccurred(), "timed out waiting for pods in %s with selector %s to be ready", namespace, labelSelector)
}

func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func logNetworkPolicyEvents(ctx context.Context, client kubernetes.Interface, namespaces []string, policyName string) {
	g.GinkgoHelper()
	found := false
	_ = wait.PollImmediate(5*time.Second, 2*time.Minute, func() (bool, error) {
		for _, namespace := range namespaces {
			events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				g.GinkgoWriter.Printf("unable to list events in %s: %v\n", namespace, err)
				continue
			}
			for _, event := range events.Items {
				if event.InvolvedObject.Kind == "NetworkPolicy" && event.InvolvedObject.Name == policyName {
					g.GinkgoWriter.Printf("event in %s: %s %s %s\n", namespace, event.Type, event.Reason, event.Message)
					found = true
				}
				if event.Message != "" && (event.InvolvedObject.Name == policyName || event.InvolvedObject.Kind == "NetworkPolicy") {
					g.GinkgoWriter.Printf("event in %s: %s %s %s\n", namespace, event.Type, event.Reason, event.Message)
					found = true
				}
			}
		}
		if found {
			return true, nil
		}
		g.GinkgoWriter.Printf("no NetworkPolicy events yet for %s (namespaces: %v)\n", policyName, namespaces)
		return false, nil
	})
	if !found {
		g.GinkgoWriter.Printf("no NetworkPolicy events observed for %s (best-effort)\n", policyName)
	}
}

func logNetworkPolicySummary(label string, policy *networkingv1.NetworkPolicy) {
	g.GinkgoWriter.Printf("networkpolicy %s namespace=%s name=%s podSelector=%v policyTypes=%v ingress=%d egress=%d\n",
		label,
		policy.Namespace,
		policy.Name,
		policy.Spec.PodSelector.MatchLabels,
		policy.Spec.PolicyTypes,
		len(policy.Spec.Ingress),
		len(policy.Spec.Egress),
	)
}

func logNetworkPolicyDetails(label string, policy *networkingv1.NetworkPolicy) {
	g.GinkgoHelper()
	g.GinkgoWriter.Printf("networkpolicy %s details:\n", label)
	g.GinkgoWriter.Printf("  podSelector=%v policyTypes=%v\n", policy.Spec.PodSelector.MatchLabels, policy.Spec.PolicyTypes)
	for i, rule := range policy.Spec.Ingress {
		g.GinkgoWriter.Printf("  ingress[%d]: ports=%s from=%s\n", i, formatPorts(rule.Ports), formatPeers(rule.From))
	}
	for i, rule := range policy.Spec.Egress {
		g.GinkgoWriter.Printf("  egress[%d]: ports=%s to=%s\n", i, formatPorts(rule.Ports), formatPeers(rule.To))
	}
}

func formatPorts(ports []networkingv1.NetworkPolicyPort) string {
	if len(ports) == 0 {
		return "[]"
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		proto := "TCP"
		if p.Protocol != nil {
			proto = string(*p.Protocol)
		}
		if p.Port == nil {
			out = append(out, fmt.Sprintf("%s:any", proto))
			continue
		}
		out = append(out, fmt.Sprintf("%s:%s", proto, p.Port.String()))
	}
	return fmt.Sprintf("[%s]", joinStrings(out))
}

func formatPeers(peers []networkingv1.NetworkPolicyPeer) string {
	if len(peers) == 0 {
		return "[]"
	}
	out := make([]string, 0, len(peers))
	for _, peer := range peers {
		ns := formatSelector(peer.NamespaceSelector)
		pod := formatSelector(peer.PodSelector)
		if ns == "" && pod == "" {
			out = append(out, "{}")
			continue
		}
		out = append(out, fmt.Sprintf("ns=%s pod=%s", ns, pod))
	}
	return fmt.Sprintf("[%s]", joinStrings(out))
}

func formatSelector(sel *metav1.LabelSelector) string {
	if sel == nil {
		return ""
	}
	if len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0 {
		return "{}"
	}
	return fmt.Sprintf("labels=%v exprs=%v", sel.MatchLabels, sel.MatchExpressions)
}

func joinStrings(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := items[0]
	for i := 1; i < len(items); i++ {
		out += ", " + items[i]
	}
	return out
}

func hasEgressAllowAll(rules []networkingv1.NetworkPolicyEgressRule) bool {
	for _, rule := range rules {
		if len(rule.To) == 0 && len(rule.Ports) == 0 {
			return true
		}
	}
	return false
}
