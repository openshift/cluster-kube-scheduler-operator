package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openshift/library-go/test/library/metrics"
	"github.com/prometheus/common/model"

	g "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
)

var _ = g.Describe("[sig-scheduling] kube scheduler operator", func() {
	g.It("[Operator][Parallel] should expose metrics endpoints accessible via prometheus", func() {
		testMetricsAccessible(g.GinkgoTB())
	})
})

func testMetricsAccessible(t testing.TB) {
	kClient, err := getKubeClient()
	if err != nil {
		t.Fatalf("failed to get kubeclient with error %v", err)
	}
	routeClient, err := getOpenShiftRouteClient()
	if err != nil {
		t.Fatalf("failed to get routeclient with error %v", err)
	}
	ctx := context.Background()
	name := names.SimpleNameGenerator.GenerateName("ksotest-metrics-")
	_, err = kClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("could not create test namespace: %v", err)
	}
	defer kClient.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})

	// Now verify kso metrics are accessible
	prometheusClient, err := metrics.NewPrometheusClient(ctx, kClient, routeClient)
	if err != nil {
		t.Fatalf("error creating route client for prometheus: %v", err)
	}
	for _, metric := range []string{
		"scheduler_schedule_attempts_total", // returned by /metrics
		"kube_pod_resource_request",         // returned by /metrics/resources
	} {
		var response model.Value
		err = wait.PollImmediate(time.Second*1, time.Minute*3, func() (bool, error) {
			response, _, err = prometheusClient.Query(ctx, metric, time.Now())
			if err != nil {
				return false, fmt.Errorf("error querying prometheus: %v", err)
			}
			if len(response.String()) == 0 {
				return false, nil
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("prometheus returned unexpected results: %v", err)
		}
		t.Logf("result from prometheus query `%s`: %v", metric, response)
	}
}
