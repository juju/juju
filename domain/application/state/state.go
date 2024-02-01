// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// UpsertApplication creates or updates the specified application,
// also adding any units if specified.
// TODO - this just creates a minimal row for now.
func (s *State) UpsertApplication(ctx context.Context, name string, units ...application.AddUnitParams) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	appNameParam := sqlair.M{"name": name}
	query := `SELECT &M.uuid FROM application WHERE name = $M.name`
	queryStmt, err := sqlair.Prepare(query, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	createApplication := `
INSERT INTO application (uuid, name, life_id)
VALUES ($M.application_uuid, $M.name, $M.life_id)
`
	createApplicationStmt, err := sqlair.Prepare(createApplication, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	upsertUnitFunc, err := upsertUnitFuncGetter()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, queryStmt, appNameParam).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying application %q", name)
			}
		}
		if err != nil {
			applicationUUID, err := utils.NewUUID()
			if err != nil {
				return errors.Trace(err)
			}
			createParams := sqlair.M{
				"application_uuid": applicationUUID.String(),
				"name":             name,
				"life_id":          life.Alive,
			}
			if err := tx.Query(ctx, createApplicationStmt, createParams).Run(); err != nil {
				return errors.Annotatef(err, "creating row for application %q", name)
			}
		}

		if len(units) == 0 {
			return nil
		}

		for _, u := range units {
			if err := upsertUnitFunc(ctx, tx, name, u); err != nil {
				return fmt.Errorf("adding unit for application %q: %w", name, err)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "upserting application %q", name)
}

// DeleteApplication deletes the specified application.
func (s *State) DeleteApplication(ctx context.Context, name string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	appNameParam := sqlair.M{"name": name}

	queryApplication := `SELECT &M.uuid FROM application WHERE name = $M.name`
	queryApplicationStmt, err := sqlair.Prepare(queryApplication, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	queryUnits := `SELECT count(*) AS &M.count FROM unit WHERE application_uuid = $M.application_uuid`
	queryUnitsStmt, err := sqlair.Prepare(queryUnits, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteApplication := `DELETE FROM application WHERE name = $M.name`
	deleteApplicationStmt, err := sqlair.Prepare(deleteApplication, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, queryApplicationStmt, appNameParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for application %q", name)
		}
		// Application already deleted is a no op.
		if len(result) == 0 {
			return nil
		}
		applicationUUID := result["uuid"].(string)

		// Check that there are no units.
		result = sqlair.M{}
		err := tx.Query(ctx, queryUnitsStmt, sqlair.M{"application_uuid": applicationUUID}).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying units for application %q", name)
			}
		}
		numUnits, _ := result["count"].(int64)
		if numUnits > 0 {
			return fmt.Errorf("cannot delete application %q as it still has %d unit(s)%w", name, numUnits, errors.Hide(applicationerrors.HasUnits))
		}

		if err := tx.Query(ctx, deleteApplicationStmt, appNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting application %q", name)
		}
		return nil
	})
	return errors.Annotatef(err, "deleting application %q", name)
}

// AddUnits adds the specified units to the application.
// TODO - this just creates a minimal row for now.
func (s *State) AddUnits(ctx context.Context, applicationName string, args ...application.AddUnitParams) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	upsertUnitFunc, err := upsertUnitFuncGetter()
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, arg := range args {
			if err := upsertUnitFunc(ctx, tx, applicationName, arg); err != nil {
				return fmt.Errorf("adding unit for application %q: %w", applicationName, err)
			}
		}
		return nil
	})
	return errors.Annotatef(err, "adding units for application %q", applicationName)
}

// upsertUnitFunc is a function which adds a unit in the specified transaction.
type upsertUnitFunc func(ctx context.Context, tx *sqlair.TX, appName string, params application.AddUnitParams) error

// upsertUnitFuncGetter returns a function which can be called as many times
// as needed to add units, ensuring that statement preparation is only done once.
// TODO - this just creates a minimal row for now.
func upsertUnitFuncGetter() (upsertUnitFunc, error) {
	query := `SELECT &M.uuid FROM unit WHERE unit_id = $M.name`
	queryStmt, err := sqlair.Prepare(query, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	queryApplication := `SELECT &M.uuid FROM application WHERE name = $M.name`
	queryApplicationStmt, err := sqlair.Prepare(queryApplication, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	createUnit := `
INSERT INTO unit (uuid, net_node_uuid, unit_id, life_id, application_uuid)
VALUES ($M.unit_uuid, $M.net_node_uuid, $M.unit_id, $M.life_id, $M.application_uuid)
`
	createUnitStmt, err := sqlair.Prepare(createUnit, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := sqlair.Prepare(createNode, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return func(ctx context.Context, tx *sqlair.TX, applicationName string, args application.AddUnitParams) error {
		// TODO - we are mirroring what's in mongo, hence the unit name is known.
		// In future we'll need to use a sequence to get a new unit id.
		if args.UnitName == nil {
			return fmt.Errorf("must pass unit name when adding a new unit for application %q", applicationName)
		}
		unitName := *args.UnitName

		result := sqlair.M{}
		err := tx.Query(ctx, queryApplicationStmt, sqlair.M{"name": applicationName}).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying applicaion for unit %q", unitName)
			}
		}
		if len(result) == 0 {
			return fmt.Errorf("application %q not found%w", applicationName, errors.Hide(applicationerrors.NotFound))
		}
		applicationUUID := result["uuid"].(string)

		result = sqlair.M{}
		err = tx.Query(ctx, queryStmt, sqlair.M{"name": unitName}).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying unit %q", unitName)
			}
		}
		// For now, we just care if the minimal unit row already exists.
		if err == nil {
			return nil
		}
		nodeUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		unitUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		createParams := sqlair.M{
			"unit_uuid":        unitUUID.String(),
			"net_node_uuid":    nodeUUID.String(),
			"unit_id":          unitName,
			"life_id":          life.Alive,
			"application_uuid": applicationUUID,
		}
		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for unit %q", unitName)
		}
		if err := tx.Query(ctx, createUnitStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating unit row for unit %q", unitName)
		}
		return nil
	}, nil

}
