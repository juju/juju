package internal

import (
	"github.com/juju/juju/domain/application/charm"
	domaindeploymentcharm "github.com/juju/juju/domain/deployment/charm"
)

// CharmStorageDefinitionForValidation holds the information required about a
// Charm's Storage Definition for the purpose of validating storage actions
// against the definition.
//
// This type exist as there are many places where storage related operations
// need to be validated against the Charm's Storage Definition for correctness.
// The types representing the Charm's Storage Definition vary so this type acts
// a common representation for the purposes of validation.
type CharmStorageDefinitionForValidation struct {
	// Name is the name of the storage definition defined by the Charm.
	Name string

	// Type is the storage type define by the Charm's storage definition.
	// Expected values will be one of [charm.StorageBlock] or
	// [charm.StorageFilesystem].
	Type charm.StorageType

	// CountMin is the minimum number of storage instances that MUST be attached
	// for this storage definition per unit. If the charm has no opinion this
	// value will be 0.
	CountMin int

	// CountMax is the maximum number of Storage Instances that can be attached
	// of this definition per Unit. If the Charm has no opinion this value will
	// be -1.
	CountMax int

	// MinimumSize is the minimum size a Storage Instance must be to be able to
	// be attached to a unit fulfilling this Storage Definition. The value is
	// expressed MiB.
	MinimumSize uint64
}

// StorageDefinitionForValidationFromCharm converts a charm storage definition
// into the reduced validation form used by application domain logic.
func StorageDefinitionForValidationFromCharm(
	def domaindeploymentcharm.Storage,
) CharmStorageDefinitionForValidation {
	return CharmStorageDefinitionForValidation{
		Name:        def.Name,
		Type:        charm.StorageType(def.Type),
		CountMin:    def.CountMin,
		CountMax:    def.CountMax,
		MinimumSize: def.MinimumSize,
	}
}

// StorageDefinitionsForValidationFromCharm converts the charm storage
// definition map into the validation view indexed by storage name.
func StorageDefinitionsForValidationFromCharm(
	defs map[string]domaindeploymentcharm.Storage,
) map[string]CharmStorageDefinitionForValidation {
	retVal := make(map[string]CharmStorageDefinitionForValidation, len(defs))
	for name, def := range defs {
		retVal[name] = StorageDefinitionForValidationFromCharm(def)
	}
	return retVal
}
