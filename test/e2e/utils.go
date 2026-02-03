package e2e

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	operatorversionedclient "github.com/openshift/client-go/operator/clientset/versioned"
	operatorclientv1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	routeclient "github.com/openshift/client-go/route/clientset/versioned"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

// ensureOperatorIsReady waits for operator before running tests
func ensureOperatorIsReady(t testing.TB) {
	kclient, err := getKubeClient()
	if err != nil {
		t.Fatalf("failed to get kubeclient with error %v", err)
	}
	ctx := context.Background()
	err = waitForOperator(ctx, kclient)
	if err != nil {
		t.Fatalf("failed waiting for operator to start: %v\n", err)
	}
}
