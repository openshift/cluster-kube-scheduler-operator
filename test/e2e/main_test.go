package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestMain(m *testing.M) {
	// e2e test job does not guarantee our operator is up before
	// launching the test, so we need to do so.
	err := waitForOperator()
	if err != nil {
		fmt.Printf("failed waiting for operator to start: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForOperator() error {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	kclient, err := k8sclient.NewForConfig(config)
	if err != nil {
		return err
	}

	err = wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		d, err := kclient.AppsV1().Deployments("openshift-kube-scheduler-operator").Get("openshift-kube-scheduler-operator", metav1.GetOptions{})
		if err != nil {
			fmt.Printf("error waiting for operator deployment to exist: %v\n", err)
			return false, nil
		}
		fmt.Println("found operator deployment")
		if d.Status.Replicas < 1 {
			fmt.Println("operator deployment has no replicas")
			return false, nil
		}
		for _, s := range d.Status.Conditions {
			if s.Type == appsv1.DeploymentAvailable && s.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		fmt.Printf("deployment is not yet available: %#v\n", d)
		return false, nil
	})
	return err
}
