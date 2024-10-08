// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	configv1 "github.com/openshift/api/config/v1"
)

// OpenStackPlatformStatusApplyConfiguration represents a declarative configuration of the OpenStackPlatformStatus type for use
// with apply.
type OpenStackPlatformStatusApplyConfiguration struct {
	APIServerInternalIP  *string                                          `json:"apiServerInternalIP,omitempty"`
	APIServerInternalIPs []string                                         `json:"apiServerInternalIPs,omitempty"`
	CloudName            *string                                          `json:"cloudName,omitempty"`
	IngressIP            *string                                          `json:"ingressIP,omitempty"`
	IngressIPs           []string                                         `json:"ingressIPs,omitempty"`
	NodeDNSIP            *string                                          `json:"nodeDNSIP,omitempty"`
	LoadBalancer         *OpenStackPlatformLoadBalancerApplyConfiguration `json:"loadBalancer,omitempty"`
	MachineNetworks      []configv1.CIDR                                  `json:"machineNetworks,omitempty"`
}

// OpenStackPlatformStatusApplyConfiguration constructs a declarative configuration of the OpenStackPlatformStatus type for use with
// apply.
func OpenStackPlatformStatus() *OpenStackPlatformStatusApplyConfiguration {
	return &OpenStackPlatformStatusApplyConfiguration{}
}

// WithAPIServerInternalIP sets the APIServerInternalIP field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the APIServerInternalIP field is set to the value of the last call.
func (b *OpenStackPlatformStatusApplyConfiguration) WithAPIServerInternalIP(value string) *OpenStackPlatformStatusApplyConfiguration {
	b.APIServerInternalIP = &value
	return b
}

// WithAPIServerInternalIPs adds the given value to the APIServerInternalIPs field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the APIServerInternalIPs field.
func (b *OpenStackPlatformStatusApplyConfiguration) WithAPIServerInternalIPs(values ...string) *OpenStackPlatformStatusApplyConfiguration {
	for i := range values {
		b.APIServerInternalIPs = append(b.APIServerInternalIPs, values[i])
	}
	return b
}

// WithCloudName sets the CloudName field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CloudName field is set to the value of the last call.
func (b *OpenStackPlatformStatusApplyConfiguration) WithCloudName(value string) *OpenStackPlatformStatusApplyConfiguration {
	b.CloudName = &value
	return b
}

// WithIngressIP sets the IngressIP field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the IngressIP field is set to the value of the last call.
func (b *OpenStackPlatformStatusApplyConfiguration) WithIngressIP(value string) *OpenStackPlatformStatusApplyConfiguration {
	b.IngressIP = &value
	return b
}

// WithIngressIPs adds the given value to the IngressIPs field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the IngressIPs field.
func (b *OpenStackPlatformStatusApplyConfiguration) WithIngressIPs(values ...string) *OpenStackPlatformStatusApplyConfiguration {
	for i := range values {
		b.IngressIPs = append(b.IngressIPs, values[i])
	}
	return b
}

// WithNodeDNSIP sets the NodeDNSIP field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the NodeDNSIP field is set to the value of the last call.
func (b *OpenStackPlatformStatusApplyConfiguration) WithNodeDNSIP(value string) *OpenStackPlatformStatusApplyConfiguration {
	b.NodeDNSIP = &value
	return b
}

// WithLoadBalancer sets the LoadBalancer field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LoadBalancer field is set to the value of the last call.
func (b *OpenStackPlatformStatusApplyConfiguration) WithLoadBalancer(value *OpenStackPlatformLoadBalancerApplyConfiguration) *OpenStackPlatformStatusApplyConfiguration {
	b.LoadBalancer = value
	return b
}

// WithMachineNetworks adds the given value to the MachineNetworks field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the MachineNetworks field.
func (b *OpenStackPlatformStatusApplyConfiguration) WithMachineNetworks(values ...configv1.CIDR) *OpenStackPlatformStatusApplyConfiguration {
	for i := range values {
		b.MachineNetworks = append(b.MachineNetworks, values[i])
	}
	return b
}
