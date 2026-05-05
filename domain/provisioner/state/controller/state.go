// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/errors"
)

// State provides direct database access to the controller
// database for provisioning info retrieval.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new controller state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// controllerConfigRow is a key-value pair from the v_controller_config view.
// GetControllerConfig retrieves controller configuration from the
// controller database, including controller-uuid, ca-cert, and api-port.
func (st *State) GetControllerConfig(ctx context.Context) (map[string]any, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &controllerConfigRow.*
FROM v_controller_config
`, controllerConfigRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []controllerConfigRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting controller config: %w", err)
	}

	result := make(map[string]any, len(rows))
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}

// GetCloudEndpoint retrieves the cloud endpoint for a given cloud name and
// region. If the region has a specific endpoint it is returned, otherwise
// the cloud-level endpoint is returned.
func (st *State) GetCloudEndpoint(ctx context.Context, cloudName, regionName string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var endpoint string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// If a region is specified, try the region endpoint first.
		if regionName != "" {
			regionStmt, err := st.Prepare(`
SELECT cr.endpoint AS &cloudEndpointRow.endpoint
FROM cloud_region AS cr
JOIN cloud AS c ON cr.cloud_uuid = c.uuid
WHERE c.name = $cloudNameParam.name
AND cr.name = $cloudRegionNameParam.region_name
`, cloudEndpointRow{}, cloudNameParam{}, cloudRegionNameParam{})
			if err != nil {
				return errors.Capture(err)
			}

			var row cloudEndpointRow
			err = tx.Query(ctx, regionStmt, cloudNameParam{Name: cloudName}, cloudRegionNameParam{RegionName: regionName}).Get(&row)
			if err == nil && row.Endpoint != "" {
				endpoint = row.Endpoint
				return nil
			}
			if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying cloud region endpoint: %w", err)
			}
		}

		// Fall back to the cloud-level endpoint.
		cloudStmt, err := st.Prepare(`
SELECT c.endpoint AS &cloudEndpointRow.endpoint
FROM cloud AS c
WHERE c.name = $cloudNameParam.name
`, cloudEndpointRow{}, cloudNameParam{})
		if err != nil {
			return errors.Capture(err)
		}

		var row cloudEndpointRow
		err = tx.Query(ctx, cloudStmt, cloudNameParam{Name: cloudName}).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying cloud endpoint: %w", err)
		}
		endpoint = row.Endpoint
		return nil
	})
	if err != nil {
		return "", errors.Errorf("getting cloud endpoint: %w", err)
	}
	return endpoint, nil
}

// GetCachedImageMetadata retrieves cached image metadata from the controller
// database matching the given version, architecture, region, and stream.
// Empty string parameters are treated as wildcards (not filtered on).
func (st *State) GetCachedImageMetadata(ctx context.Context, version, arch, region, stream string) ([]provisioner.CloudImageMetadata, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT cim.image_id AS &imageMetadataRow.image_id,
       cim.stream AS &imageMetadataRow.stream,
       cim.region AS &imageMetadataRow.region,
       cim.version AS &imageMetadataRow.version,
       a.name AS &imageMetadataRow.arch,
       cim.virt_type AS &imageMetadataRow.virt_type,
       cim.root_storage_type AS &imageMetadataRow.root_storage_type,
       cim.root_storage_size AS &imageMetadataRow.root_storage_size,
       cim.source AS &imageMetadataRow.source,
       cim.priority AS &imageMetadataRow.priority
FROM cloud_image_metadata AS cim
JOIN architecture AS a ON cim.architecture_id = a.id
WHERE ($imageMetadataFlags.has_version = 0 OR cim.version = $imageMetadataFilter.version)
AND ($imageMetadataFlags.has_arch = 0 OR a.name = $imageMetadataFilter.arch)
AND ($imageMetadataFlags.has_region = 0 OR cim.region = $imageMetadataFilter.region)
AND ($imageMetadataFlags.has_stream = 0 OR cim.stream = $imageMetadataFilter.stream)
`, imageMetadataRow{}, imageMetadataFilter{}, imageMetadataFlags{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	flags := imageMetadataFlags{}
	filter := imageMetadataFilter{Version: version, Arch: arch, Region: region, Stream: stream}
	if version != "" {
		flags.HasVersion = 1
	}
	if arch != "" {
		flags.HasArch = 1
	}
	if region != "" {
		flags.HasRegion = 1
	}
	if stream != "" {
		flags.HasStream = 1
	}

	var rows []imageMetadataRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, filter, flags).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting cached image metadata: %w", err)
	}

	result := make([]provisioner.CloudImageMetadata, len(rows))
	for i, row := range rows {
		var rootStorageSize *uint64
		if row.RootStorageSize != nil {
			v := uint64(*row.RootStorageSize)
			rootStorageSize = &v
		}
		var priority int
		if row.Priority != nil {
			priority = *row.Priority
		}
		result[i] = provisioner.CloudImageMetadata{
			ImageID:         row.ImageID,
			Stream:          row.Stream,
			Region:          row.Region,
			Version:         row.Version,
			Arch:            row.Arch,
			VirtType:        row.VirtType,
			RootStorageType: row.RootStorageType,
			RootStorageSize: rootStorageSize,
			Source:          row.Source,
			Priority:        priority,
		}
	}
	return result, nil
}
