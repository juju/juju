// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"regexp"
	"strings"

	"github.com/juju/description/v12"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/machine"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/domain/sequence/service"
	"github.com/juju/juju/domain/sequence/state"
)

const (
	// legacyOperationSequenceName is the sequence name for operations in Juju 3.
	legacyOperationSequenceName = "task"

	// legacyStorageSequenceName is the sequence name for storage in Juju 3.
	legacyStorageSequenceName = "stores"

	// legacyApplicationPrefix is the prefix for application sequences in Juju 3.
	// In Juju 3, application sequences are named "application-{appName}".
	legacyApplicationPrefix = "application-"

	// legacyContainerSuffix is the suffix for container sequences in Juju 3.
	// In Juju 3, container sequences are named "machine{parentId}{containerType}Container".
	legacyContainerSuffix = "Container"

	// storageSequenceNamespace is the sequence namespace for storage in Juju 4.
	storageSequenceNamespace = "storage"
)

// legacyContainerRegex matches container sequence names from Juju 3.
// Format: "machine{parentId}{containerType}Container"
// Examples: "machine0lxdContainer", "machine1/lxd/0kvmContainer"
// The parentId can contain slashes for nested containers.
var legacyContainerRegex = regexp.MustCompile(`^machine(.+?)(lxd|kvm)Container$`)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(
	coordinator Coordinator,
) {
	coordinator.Add(&importOperation{})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
}

// ImportService defines the sequence service used to import sequences
// from another controller model to this controller.
type ImportService interface {
	// ImportSequences imports the sequences from the given map. This is used to
	// import the sequences from the database.
	ImportSequences(ctx context.Context, seqs map[string]uint64) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import sequences"
}

// Setup creates the service that is used to import sequences.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the import, adding the sequence to the model. This also includes
// the machines and any units that are associated with the sequence.
//
// Juju 3.x stores the "next value to return" in the sequence counter, while
// Juju 4.x stores the "last value returned". To maintain correct behavior
// after migration, we subtract 1 from all imported sequence values.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	seqs := model.Sequences()
	if len(seqs) == 0 {
		return nil
	}

	s := make(map[string]uint64, len(seqs))
	for k, v := range seqs {
		k = convertLegacySequenceName(k)
		// Juju 3.x stores the next value to return, while Juju 4.x stores
		// the last value returned. Subtract 1 to convert between semantics.
		// Skip sequences with value <= 0 as they were never used.
		if v <= 0 {
			continue
		}
		s[k] = uint64(v) - 1
	}

	return i.service.ImportSequences(ctx, s)
}

// convertLegacySequenceName converts a Juju 3.x sequence name to the
// corresponding Juju 4.x sequence namespace format.
func convertLegacySequenceName(name string) string {
	switch {
	case name == legacyOperationSequenceName:
		// "task" -> "operation"
		return operation.OperationSequenceNamespace.String()

	case name == legacyStorageSequenceName:
		// "stores" -> "storage"
		return storageSequenceNamespace

	case strings.HasPrefix(name, legacyApplicationPrefix):
		// "application-myapp" -> "application_myapp"
		appName := strings.TrimPrefix(name, legacyApplicationPrefix)
		return sequence.MakePrefixNamespace(application.ApplicationSequenceNamespace, appName).String()

	case strings.HasSuffix(name, legacyContainerSuffix) && strings.HasPrefix(name, "machine"):
		// "machine0lxdContainer" -> "machine_container_0"
		// "machine1/lxd/0kvmContainer" -> "machine_container_1/lxd/0"
		if converted := convertContainerSequenceName(name); converted != "" {
			return converted
		}
	}

	return name
}

// convertContainerSequenceName converts a Juju 3.x container sequence name
// to the Juju 4.x format.
// Input format: "machine{parentId}{containerType}Container"
// Output format: "machine_container_{parentId}"
func convertContainerSequenceName(name string) string {
	matches := legacyContainerRegex.FindStringSubmatch(name)
	if matches == nil {
		return ""
	}
	parentID := matches[1]
	return sequence.MakePrefixNamespace(machine.ContainerSequenceNamespace, parentID).String()
}
