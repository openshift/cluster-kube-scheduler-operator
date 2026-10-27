package library

import (
	"fmt"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// NewClientConfigForTest returns a config configured to connect to the api server
func NewClientConfigForTest() (*rest.Config, error) {
	loader := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, &clientcmd.ConfigOverrides{ClusterInfo: api.Cluster{InsecureSkipTLSVerify: true}})
	config, err := clientConfig.ClientConfig()
	if err == nil {
		fmt.Printf("Found configuration for host %v.\n", config.Host)
	}
	return config, err
}

// NewConfigClient returns a config client for accessing OpenShift ClusterOperator resources
func NewConfigClient() (configv1client.ConfigV1Interface, error) {
	config, err := NewClientConfigForTest()
	if err != nil {
		return nil, err
	}
	return configv1client.NewForConfig(config)
}
