package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned"
	operatorversionedclient "github.com/openshift/client-go/operator/clientset/versioned"
	operatorclientv1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"reflect"
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

func waitForOperator(kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		d, err := kclient.AppsV1().Deployments("openshift-kube-scheduler-operator").Get("openshift-kube-scheduler-operator", metav1.GetOptions{})
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

func createSchedulerConfigMap(kclient *k8sclient.Clientset) (*corev1.ConfigMap, error) {
	schedulerConfigMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "policy-configmap",
		},
		Data: map[string]string{
			"policy.cfg": "{\n\"kind\" : \"Policy\",\n\"apiVersion\" : \"v1\",\n\"predicates\" : [\n\t{\"name\" : \"PodFitsHostPorts\"},\n\t{\"name\" : \"PodFitsResources\"},\n\t{\"name\": \"NoDiskConflict\"},\n\t{\"name\" : \"NoVolumeZoneConflict\"},\n\t{\"name\": \"MatchNodeSelector\"},\n\t{\"name\" : \"HostName\"}\n\t],\n\"priorities\" : [\n\t{\"name\" : \"LeastRequestedPriority\", \"weight\" : 1},\n\t{\"name\" : \"BalancedResourceAllocation\", \"weight\" : 1},\n\t{\"name\" : \"ServiceSpreadingPriority\", \"weight\" : 5},\n\t{\"name\" : \"EqualPriority\", \"weight\" : 1}\n\t]\n}\n",
		},
	}
	cm, err := kclient.CoreV1().ConfigMaps("openshift-config").Create(schedulerConfigMap)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func waitForConfigMapUpdate(kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		configMap, err := kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get("config", metav1.GetOptions{})
		if err != nil {
			klog.Infof("error waiting for config map to exist: %v\n", err)
			return false, nil
		}
		klog.Infof("Found configmap")
		// We should have algorithmSource as key in our map which shows we have configured policy.
		if data, ok := configMap.Data["config.yaml"]; !ok {
			klog.Info("Still waiting for configmap created with my policy")
			return false, nil
		} else {
			if strings.Contains(data, "policy-configmap") {
				klog.Info("Configmap created with required policy name")
				return true, nil
			} else {
				klog.Info("Still waiting for configmap to be created with my policy-configmap")
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

	if _, err = createSchedulerConfigMap(kclient); err != nil {
		t.Fatalf("failed creating configmap with error %v\n", err)
	}

	// Get scheduler CR
	schedulerCR, err := configClient.ConfigV1().Schedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)

	}
	// Update scheduler CR to the name of the config map we just created.
	schedulerCR.Spec.Policy.Name = "policy-configmap"
	if _, err = configClient.ConfigV1().Schedulers().Update(schedulerCR); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}
	// Get the latest configmap running from openshift-kube-scheduler namespace, the configmap config should be
	// updated with latest changes
	if err := waitForConfigMapUpdate(kclient); err != nil {
		t.Fatal("Timed out waiting for configmap to be updated")
	}

	// Reset scheduler CR
	schedulerCR, err = configClient.ConfigV1().Schedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)
	}
	schedulerCR.Spec = configv1.SchedulerSpec{}
	if _, err = configClient.ConfigV1().Schedulers().Update(schedulerCR); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}

	// Delete the config map that was created
	err = kclient.CoreV1().ConfigMaps("openshift-config").Delete("policy-configmap", nil)
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

	cm, err := createSchedulerConfigMap(kclient)
	if err != nil {
		t.Fatalf("failed creating configmap with error %v\n", err)
	}
	// Get scheduler CR
	schedulerCR, err := configClient.ConfigV1().Schedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)

	}

	// Update scheduler CR to the name of the config map we just created.
	schedulerCR.Spec.Policy.Name = "policy-configmap"
	if _, err = configClient.ConfigV1().Schedulers().Update(schedulerCR); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}
	// Get the latest configmap running from openshift-kube-scheduler namespace, the configmap config should be
	// updated with latest changes
	if err := waitForConfigMapUpdate(kclient); err != nil {
		t.Fatal("Timed out waiting for configmap to be updated")
	}

	// Update the configmap data in openshift-config namespace.
	// Once the update in openshift-configmap namespace is successful
	// the policy configmap in openshift-kube-scheduler namespace
	// should have been updated
	cm.Data = map[string]string{
		"policy.cfg": "{\n\"kind\" : \"Policy\",\n\"apiVersion\" : \"v1\",\n\"predicates\" : [\n\t{\"name\" : \"PodFitsHostPorts\"},\n\t{\"name\" : \"PodFitsResources\"},\n\t{\"name\": \"NoDiskConflict\"},\n\t{\"name\" : \"NoVolumeZoneConflict\"},\n\t{\"name\": \"MatchNodeSelector\"},\n\t{\"name\" : \"HostName\"}\n\t],\n\"priorities\" : [\n\t{\"name\" : \"LeastRequestedPriority\", \"weight\" : 1},\n\t{\"name\" : \"ServiceSpreadingPriority\", \"weight\" : 5},\n\t{\"name\" : \"EqualPriority\", \"weight\" : 1}\n\t]\n}\n",
	}
	if _, err = kclient.CoreV1().ConfigMaps("openshift-config").Update(cm); err != nil {
		t.Fatalf("Configmap updating failed with error %v", err)
	}

	cm, err = kclient.CoreV1().ConfigMaps("openshift-config").Get(cm.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Configmap updating failed with error %v", err)
	}
	policyConfigMap, err := waitForPolicyConfigMapCreation(kclient, cm)
	if err != nil {
		t.Fatalf("Configmap mismatch: CM generated in openshift-kube-scheduler namespace is %v and configmap in openshift-config namespace is %v", policyConfigMap.Data["policy.cfg"], cm.Data["policy.cfg"])
	}

	// Reset scheduler CR
	schedulerCR, err = configClient.ConfigV1().Schedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while listing scheduler with error %v\n", err)
	}
	schedulerCR.Spec = configv1.SchedulerSpec{}
	if _, err = configClient.ConfigV1().Schedulers().Update(schedulerCR); err != nil {
		t.Fatalf("Error while updating scheduler CR with error %v", err)
	}

	// Delete the config map that was created
	err = kclient.CoreV1().ConfigMaps("openshift-config").Delete("policy-configmap", nil)
	if err != nil {
		t.Fatalf("error waiting for config map to exist: %v\n", err)
	}
	// We expect the configmap in openshift-kube-scheduler namespace to be deleted as well
	if err := waitForPolicyConfigMapDeletion(kclient); err != nil {
		t.Fatal("Expected policy configmap to be delete in openshift-kube-scheduler namespace but it still exists")
	}
}

func waitForPolicyConfigMapCreation(kclient *k8sclient.Clientset, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	var policyCM *corev1.ConfigMap
	err := wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		var err error
		policyCM, err = kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get("policy-configmap", metav1.GetOptions{})
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

func waitForPolicyConfigMapDeletion(kclient *k8sclient.Clientset) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		_, err := kclient.CoreV1().ConfigMaps("openshift-kube-scheduler").Get("policy-configmap", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
}

func waitForOperatorDegradedStateToBeRemoved(opClient operatorclientv1.OperatorV1Interface, status string) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		schedulerConfig, err := opClient.KubeSchedulers().Get("cluster", metav1.GetOptions{})
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

// TestStaleConditionRemoval ensures that KSO returns to normal condition even if it had some degraded conditions
// in the past
func TestStaleConditionRemoval(t *testing.T) {
	operatorConfigClient, err := getOpenShiftOperatorConfigClient()
	if err != nil {
		t.Fatalf("failed to get openshift operator client with error %v", err)
	}
	opClient := operatorConfigClient.OperatorV1()
	operatorConfig, err := opClient.KubeSchedulers().Get("cluster", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get scheduler operator config with error %v", err)
	}
	// Assuming the operator is not in degraded state.
	v1helpers.SetOperatorCondition(&operatorConfig.Status.Conditions, operatorv1.OperatorCondition{
		Type:    "Degraded",
		Status:  operatorv1.ConditionTrue,
		Reason:  "Synchronization error",
		Message: "Forced Error",
	})
	if _, err := opClient.KubeSchedulers().UpdateStatus(operatorConfig); err != nil {
		t.Fatalf("failed to update scheduler operator status with error %v", err)
	}
	if err := waitForOperatorDegradedStateToBeRemoved(opClient, "Degraded"); err != nil {
		t.Fatalf("Timed out waiting for scheduler operator status to be updated")
	}
}
