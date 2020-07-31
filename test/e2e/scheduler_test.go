package e2e

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	operatorversionedclient "github.com/openshift/client-go/operator/clientset/versioned"
	operatorclientv1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	routeclient "github.com/openshift/client-go/route/clientset/versioned"
	"github.com/openshift/library-go/test/library/metrics"

	"github.com/prometheus/common/model"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/storage/names"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

const (
	customPolicyCMName = "custom-policy-configmap"
)

func getKubeClient() (*k8sclient.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	kclient, err := k8sclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return kclient, nil
}

func getOpenShiftConfigClient() (*configv1client.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	// Get openshift api config client.
	configClient, err := configv1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return configClient, nil
}

func getOpenShiftOperatorConfigClient() (*operatorversionedclient.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	operatorClient, err := operatorversionedclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return operatorClient, nil
}

func getOpenShiftRouteClient() (*routeclient.Clientset, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	routeClient, err := routeclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return routeClient, nil
}

func waitForOperator(ctx context.Context, kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		d, err := kclient.AppsV1().Deployments("openshift-kube-scheduler-operator").Get(ctx, "openshift-kube-scheduler-operator", metav1.GetOptions{})
		if err != nil {
			klog.Infof("error waiting for operator deployment to exist: %v\n", err)
			return false, nil
		}
		fmt.Println("found operator deployment")
		if d.Status.Replicas < 1 {
			klog.Infof("operator deployment has no replicas")
			return false, nil
		}
		for _, s := range d.Status.Conditions {
			if s.Type == appsv1.DeploymentAvailable && s.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		klog.Infof("deployment is not yet available: %#v\n", d)
		return false, nil
	})

}

func createSchedulerConfigMap(ctx context.Context, kclient *k8sclient.Clientset) (*corev1.ConfigMap, error) {
	schedulerConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: customPolicyCMName,
		},
		Data: map[string]string{
			"policy.cfg": "{\n\"kind\" : \"Policy\",\n\"apiVersion\" : \"v1\",\n\"predicates\" : [\n\t{\"name\" : \"PodFitsHostPorts\"},\n\t{\"name\" : \"PodFitsResources\"},\n\t{\"name\": \"NoDiskConflict\"},\n\t{\"name\" : \"NoVolumeZoneConflict\"},\n\t{\"name\": \"MatchNodeSelector\"},\n\t{\"name\" : \"HostName\"}\n\t],\n\"priorities\" : [\n\t{\"name\" : \"LeastRequestedPriority\", \"weight\" : 1},\n\t{\"name\" : \"BalancedResourceAllocation\", \"weight\" : 1},\n\t{\"name\" : \"ServiceSpreadingPriority\", \"weight\" : 5},\n\t{\"name\" : \"EqualPriority\", \"weight\" : 1}\n\t]\n}\n",
		},
	}
	cm, err := kclient.CoreV1().ConfigMaps("openshift-config").Create(ctx, schedulerConfigMap, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func waitForConfigMapUpdate(ctx context.Context, kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		configMap, err := kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get(ctx, "policy-configmap", metav1.GetOptions{})
		if err != nil {
			klog.Infof("error waiting for config map to exist: %v\n", err)
			return false, nil
		}
		klog.Infof("Found configmap")
		// We should have algorithmSource as key in our map which shows we have configured policy.
		if data, ok := configMap.Data["policy.cfg"]; !ok {
			klog.Info("Still waiting for configmap created with my policy")
			return false, nil
		} else {
			if strings.Contains(data, "predicates") {
				klog.Info("Configmap created")
				return true, nil
			} else {
				klog.Info("Still waiting for configmap to be created")
			}
		}
		klog.Infof("configmap %v is not yet available", configMap.Name)
		return false, nil
	})
}

func TestConfigMapCreation(t *testing.T) {
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

func TestPolicyConfigMapUpdate(t *testing.T) {
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

func TestMetricsAccessible(t *testing.T) {
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
	var response model.Value
	err = wait.PollImmediate(time.Second*1, time.Second*30, func() (bool, error) {
		response, _, err = prometheusClient.Query(ctx, `scheduler_scheduling_duration_seconds_sum`, time.Now())
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

	// do something with result, eventually this will be tested b4 and after rotation to ensure values differ
	t.Logf("result from prometheus query `scheduler_scheduling_duration_seconds_sum`: %v", response)
}

func waitForPolicyConfigMapCreation(ctx context.Context, kclient *k8sclient.Clientset, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	var policyCM *corev1.ConfigMap
	err := wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		var err error
		policyCM, err = kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get(ctx, "policy-configmap", metav1.GetOptions{})
		if err != nil {
			klog.Infof("Policy configmap not yet created in openshift-kube-scheduler namespace with error %v", err)
			return false, nil
		}
		if policyCM != nil && policyCM.Data != nil && reflect.DeepEqual(policyCM.Data, cm.Data) {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return policyCM, nil
}

func waitForPolicyConfigMapDeletion(ctx context.Context, kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get(ctx, "policy-configmap", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
}

func waitForOperatorDegradedStateToBeRemoved(ctx context.Context, opClient operatorclientv1.OperatorV1Interface, status string) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		schedulerConfig, err := opClient.KubeSchedulers().Get(ctx, "cluster", metav1.GetOptions{})
		if err != nil {
			klog.Infof("Scheduler operator CR cluster doesn't exist: %v\n", err)
			return false, nil
		}
		klog.Info("Scheduler operator CR cluster exists. Let's proceed to check it's status")
		for _, condition := range schedulerConfig.Status.Conditions {
			if condition.Type == status {
				return false, nil
			}
		}
		return true, nil
	})
}
