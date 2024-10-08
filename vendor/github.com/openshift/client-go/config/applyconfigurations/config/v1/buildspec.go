// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// BuildSpecApplyConfiguration represents a declarative configuration of the BuildSpec type for use
// with apply.
type BuildSpecApplyConfiguration struct {
	AdditionalTrustedCA *ConfigMapNameReferenceApplyConfiguration `json:"additionalTrustedCA,omitempty"`
	BuildDefaults       *BuildDefaultsApplyConfiguration          `json:"buildDefaults,omitempty"`
	BuildOverrides      *BuildOverridesApplyConfiguration         `json:"buildOverrides,omitempty"`
}

// BuildSpecApplyConfiguration constructs a declarative configuration of the BuildSpec type for use with
// apply.
func BuildSpec() *BuildSpecApplyConfiguration {
	return &BuildSpecApplyConfiguration{}
}

// WithAdditionalTrustedCA sets the AdditionalTrustedCA field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the AdditionalTrustedCA field is set to the value of the last call.
func (b *BuildSpecApplyConfiguration) WithAdditionalTrustedCA(value *ConfigMapNameReferenceApplyConfiguration) *BuildSpecApplyConfiguration {
	b.AdditionalTrustedCA = value
	return b
}

// WithBuildDefaults sets the BuildDefaults field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the BuildDefaults field is set to the value of the last call.
func (b *BuildSpecApplyConfiguration) WithBuildDefaults(value *BuildDefaultsApplyConfiguration) *BuildSpecApplyConfiguration {
	b.BuildDefaults = value
	return b
}

// WithBuildOverrides sets the BuildOverrides field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the BuildOverrides field is set to the value of the last call.
func (b *BuildSpecApplyConfiguration) WithBuildOverrides(value *BuildOverridesApplyConfiguration) *BuildSpecApplyConfiguration {
	b.BuildOverrides = value
	return b
}
