// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// OLMStatusApplyConfiguration represents a declarative configuration of the OLMStatus type for use
// with apply.
type OLMStatusApplyConfiguration struct {
	OperatorStatusApplyConfiguration `json:",inline"`
}

// OLMStatusApplyConfiguration constructs a declarative configuration of the OLMStatus type for use with
// apply.
func OLMStatus() *OLMStatusApplyConfiguration {
	return &OLMStatusApplyConfiguration{}
}

// WithObservedGeneration sets the ObservedGeneration field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ObservedGeneration field is set to the value of the last call.
func (b *OLMStatusApplyConfiguration) WithObservedGeneration(value int64) *OLMStatusApplyConfiguration {
	b.OperatorStatusApplyConfiguration.ObservedGeneration = &value
	return b
}

// WithConditions adds the given value to the Conditions field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Conditions field.
func (b *OLMStatusApplyConfiguration) WithConditions(values ...*OperatorConditionApplyConfiguration) *OLMStatusApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithConditions")
		}
		b.OperatorStatusApplyConfiguration.Conditions = append(b.OperatorStatusApplyConfiguration.Conditions, *values[i])
	}
	return b
}

// WithVersion sets the Version field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Version field is set to the value of the last call.
func (b *OLMStatusApplyConfiguration) WithVersion(value string) *OLMStatusApplyConfiguration {
	b.OperatorStatusApplyConfiguration.Version = &value
	return b
}

// WithReadyReplicas sets the ReadyReplicas field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ReadyReplicas field is set to the value of the last call.
func (b *OLMStatusApplyConfiguration) WithReadyReplicas(value int32) *OLMStatusApplyConfiguration {
	b.OperatorStatusApplyConfiguration.ReadyReplicas = &value
	return b
}

// WithLatestAvailableRevision sets the LatestAvailableRevision field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the LatestAvailableRevision field is set to the value of the last call.
func (b *OLMStatusApplyConfiguration) WithLatestAvailableRevision(value int32) *OLMStatusApplyConfiguration {
	b.OperatorStatusApplyConfiguration.LatestAvailableRevision = &value
	return b
}

// WithGenerations adds the given value to the Generations field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Generations field.
func (b *OLMStatusApplyConfiguration) WithGenerations(values ...*GenerationStatusApplyConfiguration) *OLMStatusApplyConfiguration {
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithGenerations")
		}
		b.OperatorStatusApplyConfiguration.Generations = append(b.OperatorStatusApplyConfiguration.Generations, *values[i])
	}
	return b
}
