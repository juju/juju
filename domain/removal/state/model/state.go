// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"encoding/json"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// State provides persistence and retrieval associated with entity removal.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetAllJobs returns all scheduled removal jobs.
func (st *State) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare("SELECT &removalJob.* FROM removal", removalJob{})
	if err != nil {
		return nil, errors.Errorf("preparing select jobs query: %w", err)
	}

	var dbJobs []removalJob
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&dbJobs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running select jobs query: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(dbJobs) == 0 {
		return nil, nil
	}

	jobs := make([]removal.Job, len(dbJobs))
	for i, job := range dbJobs {
		var arg map[string]any
		if job.Arg.Valid && job.Arg.String != "" {
			if err := json.Unmarshal([]byte(job.Arg.String), &arg); err != nil {
				return nil, errors.Errorf("decoding job arg: %w", err)
			}
		}

		jobs[i] = removal.Job{
			UUID:         removal.UUID(job.UUID),
			RemovalType:  removal.JobType(job.RemovalTypeID),
			EntityUUID:   job.EntityUUID,
			Force:        job.Force,
			ScheduledFor: job.ScheduledFor,
			Arg:          arg,
		}
	}

	return jobs, err
}

// DeleteJob ensures that a job with the input
// UUID is not present in the removal table.
func (st *State) DeleteJob(ctx context.Context, jUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	jobUUID := entityUUID{UUID: jUUID}

	stmt, err := st.Prepare("DELETE FROM removal WHERE uuid=$entityUUID.uuid", jobUUID)
	if err != nil {
		return errors.Errorf("preparing job deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, jobUUID).Run(); err != nil {
			return errors.Errorf("deleting removal row: %w", err)
		}
		return nil
	}))
}

// NamespaceForWatchRemovals returns the table name whose UUIDs we
// are watching in order to be notified of new removal jobs.
func (st *State) NamespaceForWatchRemovals() string {
	return "removal"
}

// NamespaceForWatchEntityRemovals returns the table name whose UUIDs we
// are watching in order to be notified of new removal jobs for specific
// entities.
func (st *State) NamespaceForWatchEntityRemovals() (eventsource.NamespaceQuery, map[string]string) {
	return st.initialEntityRemovalQuery(), map[string]string{
		"custom_application_uuid_lifecycle":      "application",
		"custom_machine_uuid_lifecycle":          "machine",
		"custom_model_life_model_uuid_lifecycle": "model",
		"custom_relation_uuid_lifecycle":         "relation",
		"custom_unit_uuid_lifecycle":             "unit",
	}
}

// initialEntityRemovalQuery returns the initial query for watching entity
// removals.
func (st *State) initialEntityRemovalQuery() eventsource.NamespaceQuery {
	return func(ctx context.Context, runner database.TxnRunner) ([]string, error) {
		var eUUID entityUUID
		selectUnits, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM unit WHERE life_id > 0`, eUUID)
		if err != nil {
			return nil, errors.Errorf("preparing select units query: %w", err)
		}
		selectApplications, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM application WHERE life_id > 0`, eUUID)
		if err != nil {
			return nil, errors.Errorf("preparing select applications query: %w", err)
		}
		selectRelations, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM relation WHERE life_id > 0`, eUUID)
		if err != nil {
			return nil, errors.Errorf("preparing select relations query: %w", err)
		}
		selectMachines, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM machine WHERE life_id > 0`, eUUID)
		if err != nil {
			return nil, errors.Errorf("preparing select machines query: %w", err)
		}

		var (
			units, apps, relations, machines []entityUUID
			entities                         []string
		)
		err = runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := tx.Query(ctx, selectUnits).GetAll(&units); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("selecting units: %w", err)
			}
			if err := tx.Query(ctx, selectApplications).GetAll(&apps); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("selecting applications: %w", err)
			}
			if err := tx.Query(ctx, selectRelations).GetAll(&relations); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("selecting relations: %w", err)
			}
			if err := tx.Query(ctx, selectMachines).GetAll(&machines); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("selecting machines: %w", err)
			}

			for _, u := range units {
				entities = append(entities, "unit:"+u.UUID)
			}
			for _, a := range apps {
				entities = append(entities, "application:"+a.UUID)
			}
			for _, r := range relations {
				entities = append(entities, "relation:"+r.UUID)
			}
			for _, m := range machines {
				entities = append(entities, "machine:"+m.UUID)
			}
			return nil
		})
		if err != nil {
			return nil, errors.Errorf("running initial entity removal query: %w", err)
		}
		return entities, nil
	}
}
