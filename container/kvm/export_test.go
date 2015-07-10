package kvm

// This file exports internal package implementations so that tests
// can utilize them to mock behavior.

var (
	KVMPath = &kvmPath

	// Used to export the parameters used to call Start on the KVM Container
	TestStartParams = &startParams
)

func NewEmptyKvmContainer() *kvmContainer {
	return &kvmContainer{}
}
