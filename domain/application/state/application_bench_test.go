// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	_ "github.com/mattn/go-sqlite3"

	coreapplication "github.com/juju/juju/core/application"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/txn"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/uuid"
)

func BenchmarkSetApplicationConstraints(b *testing.B) {
	tests := []struct {
		name string
		fn   func(context.Context, *State, coreapplication.UUID, constraints.Constraints) error
	}{{
		name: "StateBasePrepareWarm",
		fn:   setApplicationConstraintsWithStatePrepare,
	}, {
		name: "StaticMustPrepare",
		fn: func(ctx context.Context, st *State, appID coreapplication.UUID, cons constraints.Constraints) error {
			return st.SetApplicationConstraints(ctx, appID, cons)
		},
	}}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			ctx := b.Context()
			st, appID := newApplicationBenchmarkState(b, ctx)
			cons := benchmarkApplicationConstraints()
			if err := test.fn(ctx, st, appID, cons); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if err := test.fn(ctx, st, appID, cons); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newApplicationBenchmarkState(
	b *testing.B,
	ctx context.Context,
) (*State, coreapplication.UUID) {
	b.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		b.Fatal(err)
	}

	runner := &benchmarkTxnRunner{
		db:     sqlair.NewDB(db),
		runner: txn.NewRetryingTxnRunner(),
		dying:  make(chan struct{}),
	}
	changeSet, err := schema.ModelDDL().Ensure(ctx, runner)
	if err != nil {
		b.Fatal(err)
	}
	if changeSet.Post != schema.ModelDDL().Len() {
		b.Fatalf("unexpected schema version %d", changeSet.Post)
	}

	modelUUID, err := coremodel.NewUUID()
	if err != nil {
		b.Fatal(err)
	}
	factory := func(context.Context) (coredatabase.TxnRunner, error) {
		return runner, nil
	}
	setupState := NewState(
		factory,
		modelUUID,
		clock.WallClock,
		internallogger.Noop(),
	)

	appID, _, err := setupState.CreateIAASApplication(
		ctx,
		"benchmark-app",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Platform: deployment.Platform{
					Channel:      "22.04/stable",
					OSType:       deployment.Ubuntu,
					Architecture: architecture.AMD64,
				},
				Channel: &deployment.Channel{
					Track: "latest",
					Risk:  deployment.RiskStable,
				},
				Charm: charm.Charm{
					Metadata: charm.Metadata{
						Name: "benchmark-app",
					},
					Manifest: charm.Manifest{
						Bases: []charm.Base{{
							Name: "ubuntu",
							Channel: charm.Channel{
								Risk: charm.RiskStable,
							},
							Architectures: []string{"amd64"},
						}},
					},
					Source:        charm.CharmHubSource,
					ReferenceName: "benchmark-app",
					Revision:      1,
					Hash:          "benchmark-hash",
					Architecture:  architecture.AMD64,
				},
				CharmDownloadInfo: &charm.DownloadInfo{
					Provenance:         charm.ProvenanceDownload,
					CharmhubIdentifier: "benchmark-charm",
					DownloadURL:        "https://example.invalid/benchmark.charm",
					DownloadSize:       42,
				},
			},
		},
		nil,
	)
	if err != nil {
		b.Fatal(err)
	}

	if err := addBenchmarkSpaces(ctx, runner, "space0", "space1"); err != nil {
		b.Fatal(err)
	}

	return NewState(
		factory,
		modelUUID,
		clock.WallClock,
		internallogger.Noop(),
	), appID
}

type benchmarkTxnRunner struct {
	db     *sqlair.DB
	runner *txn.RetryingTxnRunner
	dying  chan struct{}
}

func (r *benchmarkTxnRunner) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return r.runner.Txn(ctx, r.db, fn)
}

func (r *benchmarkTxnRunner) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return r.runner.StdTxn(ctx, r.db.PlainDB(), fn)
}

func (r *benchmarkTxnRunner) Dying() <-chan struct{} {
	return r.dying
}

func addBenchmarkSpaces(ctx context.Context, runner *benchmarkTxnRunner, names ...string) error {
	return runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for i, name := range names {
			if _, err := tx.ExecContext(
				ctx,
				"INSERT INTO space (uuid, name) VALUES (?, ?)",
				fmt.Sprintf("benchmark-space-%d", i),
				name,
			); err != nil {
				return errors.Capture(err)
			}
		}
		return nil
	})
}

func benchmarkApplicationConstraints() constraints.Constraints {
	return constraints.Constraints{
		Arch:             new("amd64"),
		CpuCores:         new(uint64(2)),
		CpuPower:         new(uint64(42)),
		Mem:              new(uint64(8)),
		RootDisk:         new(uint64(256)),
		RootDiskSource:   new("root-disk-source"),
		InstanceRole:     new("instance-role"),
		InstanceType:     new("instance-type"),
		Container:        new(instance.LXD),
		VirtType:         new("virt-type"),
		AllocatePublicIP: new(true),
		ImageID:          new("image-id"),
		Spaces: new([]constraints.SpaceConstraint{
			{SpaceName: "space0", Exclude: false},
			{SpaceName: "space1", Exclude: true},
		}),
		Tags:  new([]string{"tag0", "tag1"}),
		Zones: new([]string{"zone0", "zone1"}),
	}
}

func setApplicationConstraintsWithStatePrepare(
	ctx context.Context,
	st *State,
	appID coreapplication.UUID,
	cons constraints.Constraints,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := checkApplicationNotDeadWithStatePrepare(ctx, st, tx, appID); err != nil {
			return errors.Capture(err)
		}

		return st.setApplicationConstraintsWithStatePrepare(ctx, tx, appID.String(), cons)
	})
}

func checkApplicationNotDeadWithStatePrepare(
	ctx context.Context,
	st *State,
	tx *sqlair.TX,
	appUUID coreapplication.UUID,
) error {
	return checkApplicationLifeWithStatePrepare(ctx, st, tx, appUUID, domainlife.Dying)
}

func checkApplicationLifeWithStatePrepare(
	ctx context.Context,
	st *State,
	tx *sqlair.TX,
	appUUID coreapplication.UUID,
	allowed domainlife.Life,
) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	ident := entityUUID{UUID: appUUID.String()}
	query := `
SELECT &life.*
FROM application AS a
JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid = $entityUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.UUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.UUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		if allowed < result.LifeID {
			return applicationerrors.ApplicationIsDead
		}
	case domainlife.Dying:
		if allowed < result.LifeID {
			return applicationerrors.ApplicationNotAlive
		}
	default:
		return nil
	}
	return nil
}

func (st *State) setApplicationConstraintsWithStatePrepare(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID string,
	cons constraints.Constraints,
) error {

	cUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Capture(err)
	}
	cUUIDStr := cUUID.String()

	selectConstraintUUIDQuery := `
SELECT &constraintUUID.*
FROM   application_constraint
WHERE  application_uuid = $applicationUUID.application_uuid
`
	selectConstraintUUIDStmt, err := st.Prepare(selectConstraintUUIDQuery, constraintUUID{}, applicationUUID{})
	if err != nil {
		return errors.Errorf("preparing select application constraint uuid query: %w", err)
	}

	// Check that spaces provided as constraints do exist in the space table.
	selectSpaceQuery := `SELECT &spaceUUID.uuid FROM space WHERE name = $spaceName.name`
	selectSpaceStmt, err := st.Prepare(selectSpaceQuery, spaceUUID{}, spaceName{})
	if err != nil {
		return errors.Errorf("preparing select space query: %w", err)
	}

	// Cleanup all previous tags, spaces and zones from their join tables.
	deleteConstraintTagsQuery := `DELETE FROM constraint_tag WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintTagsStmt, err := st.Prepare(deleteConstraintTagsQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint tags query: %w", err)
	}
	deleteConstraintSpacesQuery := `DELETE FROM constraint_space WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintSpacesStmt, err := st.Prepare(deleteConstraintSpacesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint spaces query: %w", err)
	}
	deleteConstraintZonesQuery := `DELETE FROM constraint_zone WHERE constraint_uuid = $constraintUUID.constraint_uuid`
	deleteConstraintZonesStmt, err := st.Prepare(deleteConstraintZonesQuery, constraintUUID{})
	if err != nil {
		return errors.Errorf("preparing delete constraint zones query: %w", err)
	}

	selectContainerTypeIDQuery := `SELECT &containerTypeID.id FROM container_type WHERE value = $containerTypeVal.value`
	selectContainerTypeIDStmt, err := st.Prepare(selectContainerTypeIDQuery, containerTypeID{}, containerTypeVal{})
	if err != nil {
		return errors.Errorf("preparing select container type id query: %w", err)
	}

	insertConstraintsQuery := `
INSERT INTO "constraint"(*)
VALUES ($setConstraint.*)
ON CONFLICT (uuid) DO UPDATE SET
    arch = excluded.arch,
    cpu_cores = excluded.cpu_cores,
    cpu_power = excluded.cpu_power,
    mem = excluded.mem,
    root_disk= excluded.root_disk,
    root_disk_source = excluded.root_disk_source,
    instance_role = excluded.instance_role,
    instance_type = excluded.instance_type,
    container_type_id = excluded.container_type_id,
    virt_type = excluded.virt_type,
    allocate_public_ip = excluded.allocate_public_ip,
    image_id = excluded.image_id,
    ip_family = excluded.ip_family

`
	insertConstraintsStmt, err := st.Prepare(insertConstraintsQuery, setConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert constraints query: %w", err)
	}

	insertConstraintTagsQuery := `INSERT INTO constraint_tag(*) VALUES ($setConstraintTag.*)`
	insertConstraintTagsStmt, err := st.Prepare(insertConstraintTagsQuery, setConstraintTag{})
	if err != nil {
		return errors.Errorf("preparing insert constraint tags query: %w", err)
	}

	insertConstraintSpacesQuery := `INSERT INTO constraint_space(*) VALUES ($setConstraintSpace.*)`
	insertConstraintSpacesStmt, err := st.Prepare(insertConstraintSpacesQuery, setConstraintSpace{})
	if err != nil {
		return errors.Capture(err)
	}

	insertConstraintZonesQuery := `INSERT INTO constraint_zone(*) VALUES ($setConstraintZone.*)`
	insertConstraintZonesStmt, err := st.Prepare(insertConstraintZonesQuery, setConstraintZone{})
	if err != nil {
		return errors.Capture(err)
	}

	insertAppConstraintsQuery := `
INSERT INTO application_constraint(*)
VALUES ($setApplicationConstraint.*)
ON CONFLICT (application_uuid) DO NOTHING
`
	insertAppConstraintsStmt, err := st.Prepare(insertAppConstraintsQuery, setApplicationConstraint{})
	if err != nil {
		return errors.Errorf("preparing insert application constraints query: %w", err)
	}

	var containerTypeID containerTypeID
	if cons.Container != nil {
		err = tx.Query(ctx, selectContainerTypeIDStmt, containerTypeVal{Value: string(*cons.Container)}).Get(&containerTypeID)
		if errors.Is(err, sqlair.ErrNoRows) {
			st.logger.Warningf(ctx, "cannot set constraints, container type %q does not exist", *cons.Container)
			return applicationerrors.InvalidApplicationConstraints
		}
		if err != nil {
			return errors.Capture(err)
		}
	}

	// First check if the constraint already exists, in that case
	// we need to update it, unsetting the nil values.
	var retrievedConstraintUUID constraintUUID
	err = tx.Query(ctx, selectConstraintUUIDStmt, applicationUUID{ApplicationUUID: appUUID}).Get(&retrievedConstraintUUID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	} else if err == nil {
		cUUIDStr = retrievedConstraintUUID.ConstraintUUID
	}

	// Cleanup tags, spaces and zones from their join tables.
	if err := tx.Query(ctx, deleteConstraintTagsStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteConstraintSpacesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteConstraintZonesStmt, constraintUUID{ConstraintUUID: cUUIDStr}).Run(); err != nil {
		return errors.Capture(err)
	}

	constraints := encodeConstraints(cUUIDStr, cons, containerTypeID.ID)

	if err := tx.Query(ctx, insertConstraintsStmt, constraints).Run(); err != nil {
		return errors.Capture(err)
	}

	if cons.Tags != nil {
		for _, tag := range *cons.Tags {
			constraintTag := setConstraintTag{ConstraintUUID: cUUIDStr, Tag: tag}
			if err := tx.Query(ctx, insertConstraintTagsStmt, constraintTag).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	if cons.Spaces != nil {
		for _, space := range *cons.Spaces {
			// Make sure the space actually exists.
			var spaceUUID spaceUUID
			err := tx.Query(ctx, selectSpaceStmt, spaceName{Name: space.SpaceName}).Get(&spaceUUID)
			if errors.Is(err, sqlair.ErrNoRows) {
				st.logger.Warningf(ctx, "cannot set constraints, space %q does not exist", space)
				return applicationerrors.InvalidApplicationConstraints
			}
			if err != nil {
				return errors.Capture(err)
			}

			constraintSpace := setConstraintSpace{ConstraintUUID: cUUIDStr, Space: space.SpaceName, Exclude: space.Exclude}
			if err := tx.Query(ctx, insertConstraintSpacesStmt, constraintSpace).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	if cons.Zones != nil {
		for _, zone := range *cons.Zones {
			constraintZone := setConstraintZone{ConstraintUUID: cUUIDStr, Zone: zone}
			if err := tx.Query(ctx, insertConstraintZonesStmt, constraintZone).Run(); err != nil {
				return errors.Capture(err)
			}
		}
	}

	return errors.Capture(
		tx.Query(ctx, insertAppConstraintsStmt, setApplicationConstraint{
			ApplicationUUID: appUUID,
			ConstraintUUID:  cUUIDStr,
		}).Run(),
	)
}
