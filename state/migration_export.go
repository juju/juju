// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
)

// The following exporter type is being refactored. This is to better model the
// dependencies for creating the exported yaml and to correctly provide us to
// unit tests at the right level of work. Rather than create integration tests
// at the "unit" level.
//
// All exporting migrations have been currently moved to `state/migrations`.
// Each provide their own type that allows them to execute a migration step
// before return if successful or not via an error. The step resembles the
// visitor pattern for good reason, as it allows us to safely model what is
// required at a type level and type safety level. Everything is typed all the
// way down. We can then create mocks for each one independently from other
// migration steps (see examples).
//
// As this is in its infancy, there are intermediary steps. Each export type
// creates its own StateExportMigration. In the future, there will be only
// one and each migration step will add itself to that and Run for completion.
//
// Whilst we're creating these steps, it is expected to create the unit tests
// and supplement all of these tests with existing tests, to ensure that no
// gaps are missing. In the future the integration tests should be replaced with
// the new shell tests to ensure a full end to end test is performed.

// ExportConfig allows certain aspects of the model to be skipped
// during the export. The intent of this is to be able to get a partial
// export to support other API calls, like status.
type ExportConfig struct {
	IgnoreIncompleteModel  bool
	SkipActions            bool
	SkipAnnotations        bool
	SkipCloudImageMetadata bool
	SkipCredentials        bool
	SkipIPAddresses        bool
	SkipSettings           bool
	SkipSSHHostKeys        bool
	SkipLinkLayerDevices   bool
	SkipRelationData       bool
	SkipInstanceData       bool
	SkipSecrets            bool
}

// ExportPartial the current model for the State optionally skipping
// aspects as defined by the ExportConfig.
func (st *State) ExportPartial(cfg ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	return st.exportImpl(cfg, store)
}

// Export the current model for the State.
func (st *State) Export(store objectstore.ObjectStore) (description.Model, error) {
	return st.exportImpl(ExportConfig{}, store)
}

func (st *State) exportImpl(cfg ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	dbModel, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := description.ModelArgs{
		Type:         string(dbModel.Type()),
		Cloud:        dbModel.CloudName(),
		CloudRegion:  dbModel.CloudRegion(),
		Owner:        dbModel.Owner().Id(),
		Config:       make(map[string]interface{}, 0),
		PasswordHash: dbModel.doc.PasswordHash,
	}
	if dbModel.LatestToolsVersion() != semversion.Zero {
		args.LatestToolsVersion = dbModel.LatestToolsVersion().String()
	}

	expModel := description.NewModel(args)

	// We used to export the model credential here but that is now done
	// using the new domain/credential exporter. We still need to set the
	// credential tag details so the exporter knows the credential to export.
	credTag, exists := dbModel.CloudCredentialTag()
	if exists && !cfg.SkipCredentials {
		expModel.SetCloudCredential(description.CloudCredentialArgs{
			Owner: credTag.Owner().Id(),
			Cloud: credTag.Cloud().Id(),
			Name:  credTag.Name(),
		})
	}

	return expModel, nil
}

// ExportStateMigration defines a migration for exporting various entities into
// a destination description model from the source state.
// It accumulates a series of migrations to run at a later time.
// Running the state migration visits all the migrations and exits upon seeing
// the first error from the migration.
type ExportStateMigration struct {
	migrations []func() error
}

// Add adds a migration to execute at a later time
// Return error from the addition will cause the Run to terminate early.
func (m *ExportStateMigration) Add(f func() error) {
	m.migrations = append(m.migrations, f)
}

// Run executes all the migrations required to be run.
func (m *ExportStateMigration) Run() error {
	for _, f := range m.migrations {
		if err := f(); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
