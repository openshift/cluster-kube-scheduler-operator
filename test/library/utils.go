package library

import (
	"context"
	"fmt"
	"time"

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
