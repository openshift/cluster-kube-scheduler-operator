package e2e

import (
	"context"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"

	test "github.com/openshift/cluster-kube-scheduler-operator/test/library"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[NetworkPolicy] should enforce generic NetworkPolicies", func() {
		testGenericNetworkPolicyEnforcement()
	})
	g.It("[NetworkPolicy] should enforce kube scheduler NetworkPolicies", func() {
		testSchedulerNetworkPolicyEnforcement()
	})
	g.It("[NetworkPolicy] should enforce cross-namespace ingress traffic", func() {
		testCrossNamespaceIngressEnforcement()
	})
	g.It("[NetworkPolicy] should allow metrics but block other ports", func() {
		testMetricsOpenButOtherPortsBlocked()
	})
	g.It("[NetworkPolicy] should allow metrics ingress from any namespace", func() {
		testMetricsIngressOpenAccess()
	})
})

func testGenericNetworkPolicyEnforcement() {
	ctx := context.Background()
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Creating a temporary namespace for policy enforcement checks")
	nsName := "np-enforcement-" + rand.String(5)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	_, err = kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	g.DeferCleanup(func() {
		g.GinkgoWriter.Printf("deleting test namespace %s\n", nsName)
		_ = kubeClient.CoreV1().Namespaces().Delete(ctx, nsName, metav1.DeleteOptions{})
	})

	serverName := "np-server"
	clientLabels := map[string]string{"app": "np-client"}
	serverLabels := map[string]string{"app": "np-server"}

	g.GinkgoWriter.Printf("creating netexec server pod %s/%s\n", nsName, serverName)
	serverPod := netexecPod(serverName, nsName, serverLabels, 8080)
	_, err = kubeClient.CoreV1().Pods(nsName).Create(ctx, serverPod, metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(waitForPodReady(ctx, kubeClient, nsName, serverName)).NotTo(o.HaveOccurred())

	server, err := kubeClient.CoreV1().Pods(nsName).Get(ctx, serverName, metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(server.Status.PodIPs).NotTo(o.BeEmpty())
	serverIPs := podIPs(server)
	g.GinkgoWriter.Printf("server pod %s/%s ips=%v\n", nsName, serverName, serverIPs)

	g.By("Verifying allow-all when no policies select the pod")
	expectConnectivity(ctx, kubeClient, nsName, clientLabels, serverIPs, 8080, true)

	g.By("Applying default deny and verifying traffic is blocked")
	g.GinkgoWriter.Printf("creating default-deny policy in %s\n", nsName)
	_, err = kubeClient.NetworkingV1().NetworkPolicies(nsName).Create(ctx, defaultDenyPolicy("default-deny", nsName), metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Adding ingress allow only and verifying traffic is still blocked")
	g.GinkgoWriter.Printf("creating allow-ingress policy in %s\n", nsName)
	_, err = kubeClient.NetworkingV1().NetworkPolicies(nsName).Create(ctx, allowIngressPolicy("allow-ingress", nsName, serverLabels, clientLabels, 8080), metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	expectConnectivity(ctx, kubeClient, nsName, clientLabels, serverIPs, 8080, false)

	g.By("Adding egress allow and verifying traffic is permitted")
	g.GinkgoWriter.Printf("creating allow-egress policy in %s\n", nsName)
	_, err = kubeClient.NetworkingV1().NetworkPolicies(nsName).Create(ctx, allowEgressPolicy("allow-egress", nsName, clientLabels, serverLabels, 8080), metav1.CreateOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	expectConnectivity(ctx, kubeClient, nsName, clientLabels, serverIPs, 8080, true)
}

func testSchedulerNetworkPolicyEnforcement() {
	ctx := context.Background()
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	schedulerOperatorLabels := map[string]string{"app": "openshift-kube-scheduler-operator"}

	g.By("Verifying kube-scheduler-operator NetworkPolicy exists")
	_, err = kubeClient.NetworkingV1().NetworkPolicies(schedulerOperatorNamespace).Get(ctx, "kube-scheduler-operator", metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Creating test pods in openshift-kube-scheduler-operator for allow/deny checks")
	g.GinkgoWriter.Printf("creating scheduler operator server pods in %s\n", schedulerOperatorNamespace)
	allowedServerIPs, cleanupAllowed := createServerPod(ctx, kubeClient, schedulerOperatorNamespace, "np-scheduler-op-allowed", schedulerOperatorLabels, 8443)
	g.DeferCleanup(cleanupAllowed)
	deniedServerIPs, cleanupDenied := createServerPod(ctx, kubeClient, schedulerOperatorNamespace, "np-scheduler-op-denied", schedulerOperatorLabels, 12345)
	g.DeferCleanup(cleanupDenied)

	g.By("Verifying allowed port 8443 ingress to scheduler operator")
	expectConnectivity(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorLabels, allowedServerIPs, 8443, true)

	g.By("Verifying denied port 12345 (not in NetworkPolicy)")
	expectConnectivity(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorLabels, deniedServerIPs, 12345, false)

	g.By("Verifying denied ports even from same namespace")
	for _, port := range []int32{80, 443, 6443, 9090} {
		expectConnectivity(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorLabels, allowedServerIPs, port, false)
	}

	g.By("Verifying scheduler operator egress to DNS")
	dnsSvc, err := kubeClient.CoreV1().Services("openshift-dns").Get(ctx, "dns-default", metav1.GetOptions{})
	o.Expect(err).NotTo(o.HaveOccurred())
	dnsIPs := serviceClusterIPs(dnsSvc)
	g.GinkgoWriter.Printf("expecting allow from %s to DNS %v:53\n", schedulerOperatorNamespace, dnsIPs)
	expectConnectivity(ctx, kubeClient, schedulerOperatorNamespace, schedulerOperatorLabels, dnsIPs, 53, true)

	g.By("Verifying scheduler pods egress (installer/pruner policy)")
	installerLabels := map[string]string{"app": "installer"}
	expectConnectivity(ctx, kubeClient, schedulerNamespace, installerLabels, dnsIPs, 53, true)
}

func testCrossNamespaceIngressEnforcement() {
	ctx := context.Background()
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Creating test server pods in kube-scheduler-operator namespace")
	schedulerOpLabels := map[string]string{"app": "openshift-kube-scheduler-operator"}
	schedulerOpIPs, cleanupSchedulerOp := createServerPod(ctx, kubeClient, schedulerOperatorNamespace, "np-scheduler-op-xns", schedulerOpLabels, 8443)
	g.DeferCleanup(cleanupSchedulerOp)

	g.By("Testing cross-namespace ingress: monitoring -> kube-scheduler-operator:8443")
	g.GinkgoWriter.Printf("expecting allow from openshift-monitoring to %v:8443\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-monitoring", map[string]string{"app.kubernetes.io/name": "prometheus"}, schedulerOpIPs, 8443, true)

	g.By("Testing cross-namespace ingress: any pod from monitoring can access metrics")
	g.GinkgoWriter.Printf("expecting allow from openshift-monitoring with any labels to %v:8443\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-monitoring", map[string]string{"app": "any-label"}, schedulerOpIPs, 8443, true)
}

func testMetricsOpenButOtherPortsBlocked() {
	ctx := context.Background()
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Creating test server pod in kube-scheduler-operator namespace")
	schedulerOpLabels := map[string]string{"app": "openshift-kube-scheduler-operator"}
	schedulerOpIPs, cleanupSchedulerOp := createServerPod(ctx, kubeClient, schedulerOperatorNamespace, "np-scheduler-op-unauth", schedulerOpLabels, 8443)
	g.DeferCleanup(cleanupSchedulerOp)

	g.By("Testing metrics port 8443 is open: default namespace -> kube-scheduler-operator:8443")
	g.GinkgoWriter.Printf("expecting allow from default to %v:8443 (metrics open to all)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "default", map[string]string{"test": "client"}, schedulerOpIPs, 8443, true)

	g.By("Testing metrics port 8443 from openshift-etcd with custom app label: should be denied")
	g.GinkgoWriter.Printf("expecting deny from openshift-etcd with custom label to %v:8443 (custom app labels not allowed in etcd namespace)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-etcd", map[string]string{"test": "client"}, schedulerOpIPs, 8443, false)

	g.By("Testing port-based blocking: unauthorized ports are still blocked")
	g.GinkgoWriter.Printf("expecting deny from openshift-monitoring to %v:9999 (unauthorized port)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-monitoring", map[string]string{"app.kubernetes.io/name": "prometheus"}, schedulerOpIPs, 9999, false)

	g.By("Testing multiple unauthorized ports are still blocked by default-deny")
	for _, port := range []int32{80, 443, 8080, 22, 3306, 9090} {
		g.GinkgoWriter.Printf("expecting deny from default to %v:%d (unauthorized port)\n", schedulerOpIPs, port)
		expectConnectivity(ctx, kubeClient, "default", map[string]string{"test": "any-pod"}, schedulerOpIPs, port, false)
	}
}

func testMetricsIngressOpenAccess() {
	ctx := context.Background()
	kubeConfig, err := test.NewClientConfigForTest()
	o.Expect(err).NotTo(o.HaveOccurred())
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	o.Expect(err).NotTo(o.HaveOccurred())

	g.By("Creating test server pod in kube-scheduler-operator namespace with operator labels")
	schedulerOpLabels := map[string]string{"app": "openshift-kube-scheduler-operator"}
	schedulerOpIPs, cleanupSchedulerOp := createServerPod(ctx, kubeClient, schedulerOperatorNamespace, "np-metrics-test", schedulerOpLabels, 8443)
	g.DeferCleanup(cleanupSchedulerOp)

	g.By("Testing metrics policy: monitoring namespace can access metrics -> operator:8443")
	g.GinkgoWriter.Printf("expecting allow from openshift-monitoring to %v:8443\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-monitoring", map[string]string{"app.kubernetes.io/name": "prometheus"}, schedulerOpIPs, 8443, true)

	g.By("Testing metrics policy: etcd namespace with custom app label should be denied")
	g.GinkgoWriter.Printf("expecting deny from openshift-etcd with custom label to %v:8443 (custom app labels not allowed in etcd namespace)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-etcd", map[string]string{"test": "metrics-client"}, schedulerOpIPs, 8443, false)

	g.By("Testing metrics policy: console namespace with custom app label can access metrics")
	g.GinkgoWriter.Printf("expecting allow from openshift-console with custom label to %v:8443 (metrics open to most namespaces)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-console", map[string]string{"custom-app": "test-client"}, schedulerOpIPs, 8443, true)

	g.By("Testing metrics policy: default namespace can access metrics -> operator:8443")
	g.GinkgoWriter.Printf("expecting allow from default namespace to %v:8443\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "default", map[string]string{"test": "client"}, schedulerOpIPs, 8443, true)

	g.By("Testing metrics policy: same namespace can access metrics -> operator:8443")
	g.GinkgoWriter.Printf("expecting allow from openshift-kube-scheduler-operator to %v:8443 (same namespace)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, schedulerOperatorNamespace, map[string]string{"app": "openshift-kube-scheduler-operator"}, schedulerOpIPs, 8443, true)

	g.By("Testing default-deny still blocks unauthorized ports")
	g.GinkgoWriter.Printf("expecting deny from openshift-monitoring to %v:9090 (wrong port, not allowed by any policy)\n", schedulerOpIPs)
	expectConnectivity(ctx, kubeClient, "openshift-monitoring", map[string]string{"app.kubernetes.io/name": "prometheus"}, schedulerOpIPs, 9090, false)
}
