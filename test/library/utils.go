package library

import (
	"context"
	"fmt"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// WaitForOperator waits for the kube-scheduler-operator deployment to be available
func WaitForOperator(ctx context.Context, kclient kubernetes.Interface) error {
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

// WaitForClusterOperatorStable waits for the kube-scheduler ClusterOperator to be non-progressing
func WaitForClusterOperatorStable(ctx context.Context, configClient configv1client.ConfigV1Interface) error {
	return wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		co, err := configClient.ClusterOperators().Get(ctx, "kube-scheduler", metav1.GetOptions{})
		if err != nil {
			klog.Infof("error getting clusteroperator: %v\n", err)
			return false, nil
		}

		for _, condition := range co.Status.Conditions {
			if condition.Type == configv1.OperatorProgressing {
				if condition.Status == configv1.ConditionTrue {
					klog.Infof("cluster operator still progressing: %s\n", condition.Message)
					return false, nil
				}
				return true, nil
			}
		}
		return false, fmt.Errorf("progressing condition not found")
	})
}
