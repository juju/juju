// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type charmStateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&charmStateSuite{})

func (s *charmStateSuite) TestGetCharmIDByRevision(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'foo')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_origin (charm_uuid, revision) VALUES (?, 1)`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := st.GetCharmIDByRevision(context.Background(), "foo", 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.Equals, id)
}

func (s *charmStateSuite) TestSetCharmGetCharmIDByRevision(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "foo",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	charmID, err := st.GetCharmIDByRevision(context.Background(), "foo", 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(charmID, gc.Equals, id)
}

func (s *charmStateSuite) TestGetCharmIDByRevisionWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	_, err := st.GetCharmIDByRevision(context.Background(), "foo", 0)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestIsControllerCharmWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestIsControllerCharmWithControllerCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'juju-controller')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmStateSuite) TestIsControllerCharmWithNoControllerCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *charmStateSuite) TestIsSubordinateCharmWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestIsSubordinateCharmWithSubordinateCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, true)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmStateSuite) TestIsSubordinateCharmWithNoSubordinateCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *charmStateSuite) TestSupportsContainersWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSupportsContainersWithContainers(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_container (charm_uuid, "key") VALUES (?, 'ubuntu@22.04')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_container (charm_uuid, "key") VALUES (?, 'ubuntu@20.04')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmStateSuite) TestSupportsContainersWithNoContainers(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *charmStateSuite) TestIsCharmAvailableWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestIsCharmAvailableWithAvailable(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, true)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmStateSuite) TestIsCharmAvailableWithNotAvailable(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, subordinate) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *charmStateSuite) TestSetCharmAvailableWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := st.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSetCharmAvailable(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name) VALUES (?, 'ubuntu')`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)

	err = st.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	result, err = st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsTrue)
}

func (s *charmStateSuite) TestReserveCharmRevisionWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.ReserveCharmRevision(context.Background(), id, 1)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestReserveCharmRevision(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm (uuid, name, run_as_id) VALUES (?, 'ubuntu', 0)`, id.String())
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, id.String())
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	newID, err := st.ReserveCharmRevision(context.Background(), id, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(newID, gc.Not(gc.DeepEquals), id)

	// Ensure that the new charm is usable, although should not be available.
	result, err := st.IsCharmAvailable(context.Background(), newID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.IsFalse)
}

func (s *charmStateSuite) TestGetCharmMetadataWithNoCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestGetCharmMetadata(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertCharmMetadata(ctx, c, tx, uuid)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metadata, gc.DeepEquals, charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithTagsAndCategories(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that duplicate tags and categories are correctly inserted and
	// extracted.

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_category (charm_uuid, array_index, value)
VALUES (?, 0, 'data'), (?, 1, 'kubernetes'), (?, 2, 'kubernetes')
`, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_tag (charm_uuid, array_index, value)
VALUES (?, 0, 'foo'), (?, 1, 'foo'), (?, 2,'bar')
`, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Tags = []string{"foo", "foo", "bar"}
		expected.Categories = []string{"data", "kubernetes", "kubernetes"}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithTerms(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that duplicate terms are correctly inserted and extracted.

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_term (charm_uuid, array_index, value) 
VALUES (?, 0, 'alpha'), (?, 1, 'beta'), (?, 2, 'beta')
`, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Terms = []string{"alpha", "beta", "beta"}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithRelation(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that relations are correctly extracted.

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_relation (charm_uuid, kind_id, key, name, role_id, scope_id) 
VALUES 
    (?, 0, 'foo', 'baz', 0, 0),
    (?, 0, 'fred', 'bar', 0, 1),
    (?, 1, 'foo', 'baz', 1, 1),
    (?, 2, 'foo', 'baz', 2, 0);`,
			uuid, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Provides = map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			},
			"fred": {
				Key:   "fred",
				Name:  "bar",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeContainer,
			},
		}
		expected.Requires = map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RoleRequirer,
				Scope: charm.ScopeContainer,
			},
		}
		expected.Peers = map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RolePeer,
				Scope: charm.ScopeGlobal,
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithExtraBindings(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_extra_binding (charm_uuid, key, name) 
VALUES 
    (?, 'foo', 'bar'),
    (?, 'fred', 'baz');`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.ExtraBindings = map[string]charm.ExtraBinding{
			"foo": {
				Name: "bar",
			},
			"fred": {
				Name: "baz",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithStorageWithNoProperties(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (
    charm_uuid,
    key,
    name,
    description,
    storage_kind_id,
    shared,
    read_only,
    count_min,
    count_max,
    minimum_size_mib,
    location
) VALUES 
    (?, 'foo', 'bar', 'description 1', 1, true, true, 1, 2, 3, '/tmp'),
    (?, 'fred', 'baz', 'description 2', 0, false, false, 4, 5, 6, '/var/mount');`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Storage = map[string]charm.Storage{
			"foo": {
				Name:        "bar",
				Type:        charm.StorageFilesystem,
				Description: "description 1",
				Shared:      true,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 3,
				Location:    "/tmp",
			},
			"fred": {
				Name:        "baz",
				Type:        charm.StorageBlock,
				Description: "description 2",
				Shared:      false,
				ReadOnly:    false,
				CountMin:    4,
				CountMax:    5,
				MinimumSize: 6,
				Location:    "/var/mount",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithStorageWithProperties(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with properties is correctly extracted.

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (
    charm_uuid,
    key,
    name,
    description,
    storage_kind_id,
    shared,
    read_only,
    count_min,
    count_max,
    minimum_size_mib,
    location
) VALUES 
    (?, 'foo', 'bar', 'description 1', 1, true, true, 1, 2, 3, '/tmp'),
    (?, 'fred', 'baz', 'description 2', 0, false, false, 4, 5, 6, '/var/mount');`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage_property (
    charm_uuid,
    charm_storage_key,
    array_index,
    value
) VALUES
    (?, 'foo', 0, 'alpha'),
    (?, 'foo', 1, 'beta'),
    (?, 'foo', 2, 'beta');`,
			uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Storage = map[string]charm.Storage{
			"foo": {
				Name:        "bar",
				Type:        charm.StorageFilesystem,
				Description: "description 1",
				Shared:      true,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 3,
				Location:    "/tmp",
				Properties:  []string{"alpha", "beta", "beta"},
			},
			"fred": {
				Name:        "baz",
				Type:        charm.StorageBlock,
				Description: "description 2",
				Shared:      false,
				ReadOnly:    false,
				CountMin:    4,
				CountMax:    5,
				MinimumSize: 6,
				Location:    "/var/mount",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithDevices(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_device (
    charm_uuid,
    key,
    name,
    description,
    device_type,
    count_min,
    count_max
) VALUES 
    (?, 'foo', 'bar', 'description 1', 'gpu', 1, 2),
    (?, 'fred', 'baz', 'description 2', 'tpu', 3, 4);`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Devices = map[string]charm.Device{
			"foo": {
				Name:        "bar",
				Type:        charm.DeviceType("gpu"),
				Description: "description 1",
				CountMin:    1,
				CountMax:    2,
			},
			"fred": {
				Name:        "baz",
				Type:        charm.DeviceType("tpu"),
				Description: "description 2",
				CountMin:    3,
				CountMax:    4,
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithPayloadClasses(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_payload (
    charm_uuid,
    key,
    name,
    type
) VALUES 
    (?, 'foo', 'bar', 'docker'),
    (?, 'fred', 'baz', 'kvm');`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.PayloadClasses = map[string]charm.PayloadClass{
			"foo": {
				Name: "bar",
				Type: "docker",
			},
			"fred": {
				Name: "baz",
				Type: "kvm",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithResources(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_resource (
    charm_uuid,
    key,
    name,
    kind_id,
    path,
    description
) VALUES 
    (?, 'foo', 'bar', 0, '/tmp/file.txt', 'description 1'),
    (?, 'fred', 'baz', 1, 'hub.docker.io/jujusolutions', 'description 2');`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Resources = map[string]charm.Resource{
			"foo": {
				Name:        "bar",
				Type:        charm.ResourceTypeFile,
				Path:        "/tmp/file.txt",
				Description: "description 1",
			},
			"fred": {
				Name:        "baz",
				Type:        charm.ResourceTypeContainerImage,
				Path:        "hub.docker.io/jujusolutions",
				Description: "description 2",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithContainersWithNoMounts(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_container (
    charm_uuid,
    key,
    resource,
    uid,
    gid
) VALUES 
    (?, 'foo', 'ubuntu@22.04', 100, 100),
    (?, 'fred', 'ubuntu@20.04', -1, -1);`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Containers = map[string]charm.Container{
			"foo": {
				Resource: "ubuntu@22.04",
				Uid:      ptr(100),
				Gid:      ptr(100),
			},
			"fred": {
				Resource: "ubuntu@20.04",
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestGetCharmMetadataWithContainersWithMounts(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Metadata
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmMetadata(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_container (
    charm_uuid,
    key,
    resource,
    uid,
    gid
) VALUES 
    (?, 'foo', 'ubuntu@22.04', 100, 100),
    (?, 'fred', 'ubuntu@20.04', -1, -1);`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_container_mount (
    charm_uuid,
    array_index,
    charm_container_key,
    storage,
    location
) VALUES
    (?, 0, 'foo', 'block', '/tmp'),
    (?, 1, 'foo', 'block', '/dev/nvme0n1'),
    (?, 0, 'fred', 'file', '/var/log');`,
			uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	metadata, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmMetadata(c, metadata, func() charm.Metadata {
		expected.Containers = map[string]charm.Container{
			"foo": {
				Resource: "ubuntu@22.04",
				Uid:      ptr(100),
				Gid:      ptr(100),
				Mounts: []charm.Mount{
					{
						Storage:  "block",
						Location: "/tmp",
					},
					{
						Storage:  "block",
						Location: "/dev/nvme0n1",
					},
				},
			},
			"fred": {
				Resource: "ubuntu@20.04",
				Mounts: []charm.Mount{
					{
						Storage:  "file",
						Location: "/var/log",
					},
				},
			},
		}
		return expected
	})
}

func (s *charmStateSuite) TestDeleteCharm(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	err := st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSetCharmTwice(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	_, err = st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmAlreadyExists)
}

func (s *charmStateSuite) TestSetCharmAllowsSameNameButDifferentRevision(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}

	id1, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, charm.SetStateArgs{
		Source:      charm.LocalSource,
		Revision:    1,
		Hash:        "hash",
		ArchivePath: "archive",
		Version:     "deadbeef",
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	id2, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, charm.SetStateArgs{
		Source:      charm.LocalSource,
		Revision:    2,
		Hash:        "hash",
		ArchivePath: "archive",
		Version:     "deadbeef",
	})
	c.Assert(err, jc.ErrorIsNil)

	got, err = st.GetCharmMetadata(context.Background(), id2)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadata(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithTagsAndCategories(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Tags:           []string{"foo", "foo", "bar"},
		Categories:     []string{"data", "kubernetes", "kubernetes"},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithTerms(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Terms:          []string{"foo", "foo", "bar"},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithRelations(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Provides: map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			},
			"fred": {
				Key:   "fred",
				Name:  "bar",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeContainer,
			},
		},
		Requires: map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RoleRequirer,
				Scope: charm.ScopeContainer,
			},
		},
		Peers: map[string]charm.Relation{
			"foo": {
				Key:   "foo",
				Name:  "baz",
				Role:  charm.RolePeer,
				Scope: charm.ScopeGlobal,
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_relation")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithExtraBindings(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		ExtraBindings: map[string]charm.ExtraBinding{
			"foo": {
				Name: "bar",
			},
			"fred": {
				Name: "baz",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_extra_binding")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithStorageWithNoProperties(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Storage: map[string]charm.Storage{
			"foo": {
				Name:        "bar",
				Type:        charm.StorageFilesystem,
				Description: "description 1",
				Shared:      true,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 3,
				Location:    "/tmp",
			},
			"fred": {
				Name:        "baz",
				Type:        charm.StorageBlock,
				Description: "description 2",
				Shared:      false,
				ReadOnly:    false,
				CountMin:    4,
				CountMax:    5,
				MinimumSize: 6,
				Location:    "/var/mount",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_storage")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithStorageWithProperties(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Storage: map[string]charm.Storage{
			"foo": {
				Name:        "bar",
				Type:        charm.StorageFilesystem,
				Description: "description 1",
				Shared:      true,
				ReadOnly:    true,
				CountMin:    1,
				CountMax:    2,
				MinimumSize: 3,
				Location:    "/tmp",
				Properties:  []string{"alpha", "beta", "beta"},
			},
			"fred": {
				Name:        "baz",
				Type:        charm.StorageBlock,
				Description: "description 2",
				Shared:      false,
				ReadOnly:    false,
				CountMin:    4,
				CountMax:    5,
				MinimumSize: 6,
				Location:    "/var/mount",
				Properties:  []string{"foo", "foo", "baz"},
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_storage")
	assertTableEmpty(c, s.TxnRunner(), "charm_storage_property")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithDevices(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Devices: map[string]charm.Device{
			"foo": {
				Name:        "bar",
				Type:        charm.DeviceType("gpu"),
				Description: "description 1",
				CountMin:    1,
				CountMax:    2,
			},
			"fred": {
				Name:        "baz",
				Type:        charm.DeviceType("tpu"),
				Description: "description 2",
				CountMin:    3,
				CountMax:    4,
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_device")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithPayloadClasses(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		PayloadClasses: map[string]charm.PayloadClass{
			"foo": {
				Name: "bar",
				Type: "docker",
			},
			"fred": {
				Name: "baz",
				Type: "kvm",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_payload")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithResources(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Resources: map[string]charm.Resource{
			"foo": {
				Name:        "bar",
				Type:        charm.ResourceTypeFile,
				Path:        "/tmp/file.txt",
				Description: "description 1",
			},
			"fred": {
				Name:        "baz",
				Type:        charm.ResourceTypeContainerImage,
				Path:        "hub.docker.io/jujusolutions",
				Description: "description 2",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_resource")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithContainersWithNoMounts(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Containers: map[string]charm.Container{
			"foo": {
				Resource: "ubuntu@22.04",
				Uid:      ptr(100),
				Gid:      ptr(100),
			},
			"fred": {
				Resource: "ubuntu@20.04",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_container")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmMetadataWithContainersWithMounts(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
		Containers: map[string]charm.Container{
			"foo": {
				Resource: "ubuntu@22.04",
				Uid:      ptr(100),
				Gid:      ptr(100),
				Mounts: []charm.Mount{
					{
						Storage:  "block",
						Location: "/tmp",
					},
					{
						Storage:  "block",
						Location: "/tmp",
					},
					{
						Storage:  "block",
						Location: "/dev/nvme0n1",
					},
				},
			},
			"fred": {
				Resource: "ubuntu@20.04",
				Mounts: []charm.Mount{
					{
						Storage:  "file",
						Location: "/var/log",
					},
				},
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_container")
	assertTableEmpty(c, s.TxnRunner(), "charm_container_mount")
}

func (s *charmStateSuite) TestGetCharmManifest(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	var expected charm.Manifest
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		if expected, err = insertCharmManifest(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_manifest_base (
    charm_uuid,
    array_index,
    nested_array_index,
    os_id,
    track,
    risk,
    branch,
    architecture_id
) VALUES 
    (?, 0, 1, 0, '', 'stable', '', 1),
    (?, 1, 0, 0, '', 'edge', 'foo', 0),
    (?, 0, 0, 0, '', 'stable', '', 0),
    (?, 2, 0, 0, '4.0', 'beta', 'baz', 2);`,
			uuid, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	manifest, err := st.GetCharmManifest(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertCharmManifest(c, manifest, func() charm.Manifest {
		expected.Bases = []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64", "arm64"},
			},
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk:   charm.RiskEdge,
					Branch: "foo",
				},
				Architectures: []string{"amd64"},
			},
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track:  "4.0",
					Risk:   charm.RiskBeta,
					Branch: "baz",
				},
				Architectures: []string{"ppc64el"},
			},
		}
		return expected
	})

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_manifest_base")
}

func (s *charmStateSuite) TestSetCharmThenGetCharmManifest(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64", "arm64"},
			},
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk:   charm.RiskEdge,
					Branch: "foo",
				},
				Architectures: []string{"amd64"},
			},
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Track:  "4.0",
					Risk:   charm.RiskBeta,
					Branch: "baz",
				},
				Architectures: []string{"ppc64el"},
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: charm.Metadata{
			Name: "ubuntu",
		},
		Manifest: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmManifest(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_manifest_base")
}
func (s *charmStateSuite) TestGetCharmManifestCharmNotFound(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmManifest(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestGetCharmLXDProfile(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCharmState(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err := tx.ExecContext(ctx, `
UPDATE charm 
SET lxd_profile = ?
WHERE uuid = ?
`, `{"profile": []}`, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	profile, err := st.GetCharmLXDProfile(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(profile, gc.DeepEquals, []byte(`{"profile": []}`))

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
}

func (s *charmStateSuite) TestGetCharmLXDProfileCharmNotFound(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmLXDProfile(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestGetCharmConfig(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCharmState(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_config (
    charm_uuid,
	"key",
    type_id,
    default_value,
    description
) VALUES 
    (?, 'foo', 0, 'string', 'this is a string'),
    (?, 'bar', 1, '42', 'this is an int'),
	(?, 'baz', 3, 'true', 'this is a bool'),
	(?, 'alpha', 2, '3.42', 'this is a float'),
	(?, 'beta', 2, '3', 'this is also a float'),
	(?, 'shh', 4, 'secret', 'this is a secret');`,
			uuid, uuid, uuid, uuid, uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	config, err := st.GetCharmConfig(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, charm.Config{
		Options: map[string]charm.Option{
			"foo": {
				Type:        charm.OptionString,
				Default:     "string",
				Description: "this is a string",
			},
			"bar": {
				Type:        charm.OptionInt,
				Default:     42,
				Description: "this is an int",
			},
			"baz": {
				Type:        charm.OptionBool,
				Default:     true,
				Description: "this is a bool",
			},
			"alpha": {
				Type:        charm.OptionFloat,
				Default:     3.42,
				Description: "this is a float",
			},
			"beta": {
				Type:        charm.OptionFloat,
				Default:     float64(3),
				Description: "this is also a float",
			},
			"shh": {
				Type:        charm.OptionSecret,
				Default:     "secret",
				Description: "this is a secret",
			},
		},
	})
}

func (s *charmStateSuite) TestSetCharmThenGetCharmConfig(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Config{
		Options: map[string]charm.Option{
			"foo": {
				Type:        charm.OptionString,
				Default:     "string",
				Description: "this is a string",
			},
			"bar": {
				Type:        charm.OptionInt,
				Default:     42,
				Description: "this is an int",
			},
			"baz": {
				Type:        charm.OptionBool,
				Default:     true,
				Description: "this is a bool",
			},
			"alpha": {
				Type:        charm.OptionFloat,
				Default:     3.42,
				Description: "this is a float",
			},
			"beta": {
				Type:        charm.OptionFloat,
				Default:     float64(3),
				Description: "this is also a float",
			},
			"shh": {
				Type:        charm.OptionSecret,
				Default:     "secret",
				Description: "this is a secret",
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: charm.Metadata{
			Name: "ubuntu",
		},
		Config: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmConfig(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_config")
}

func (s *charmStateSuite) TestGetCharmConfigCharmNotFound(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmConfig(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestGetCharmConfigEmpty(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCharmState(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	config, err := st.GetCharmConfig(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, charm.Config{
		Options: map[string]charm.Option{},
	})
}

func (s *charmStateSuite) TestGetCharmActions(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCharmState(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}

		_, err := tx.ExecContext(ctx, `
INSERT INTO charm_action (
    charm_uuid,
	"key",
    description,
    parallel,
    execution_group,
	params
) VALUES 
    (?, 'foo', 'description1', true, 'group1', '{}'),
    (?, 'bar', 'description2', false, 'group2', null);`,
			uuid, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	config, err := st.GetCharmActions(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, charm.Actions{
		Actions: map[string]charm.Action{
			"foo": {
				Description:    "description1",
				Parallel:       true,
				ExecutionGroup: "group1",
				Params:         []byte("{}"),
			},
			"bar": {
				Description:    "description2",
				Parallel:       false,
				ExecutionGroup: "group2",
			},
		},
	})
}

func (s *charmStateSuite) TestSetCharmThenGetCharmActions(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	expected := charm.Actions{
		Actions: map[string]charm.Action{
			"foo": {
				Description:    "description1",
				Parallel:       true,
				ExecutionGroup: "group1",
				Params:         []byte("{}"),
			},
			"bar": {
				Description:    "description2",
				Parallel:       false,
				ExecutionGroup: "group2",
				Params:         make([]byte, 0),
			},
		},
	}

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: charm.Metadata{
			Name: "ubuntu",
		},
		Actions: expected,
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmActions(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, expected)

	err = st.DeleteCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	assertTableEmpty(c, s.TxnRunner(), "charm")
	assertTableEmpty(c, s.TxnRunner(), "charm_action")
}

func (s *charmStateSuite) TestGetCharmActionsCharmNotFound(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmActions(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *charmStateSuite) TestGetCharmActionsEmpty(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		if err := insertCharmState(ctx, c, tx, uuid); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	config, err := st.GetCharmActions(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(config, gc.DeepEquals, charm.Actions{
		Actions: map[string]charm.Action{},
	})
}

func (s *charmStateSuite) TestSetCharmThenGetCharmArchivePath(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id, err := st.SetCharm(context.Background(), charm.Charm{
		Metadata: charm.Metadata{
			Name: "ubuntu",
		},
	}, setStateArgs())
	c.Assert(err, jc.ErrorIsNil)

	got, err := st.GetCharmArchivePath(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(got, gc.DeepEquals, "archive")
}

func (s *charmStateSuite) TestGetCharmArchivePathCharmNotFound(c *gc.C) {
	st := NewCharmState(&commonStateBase{StateBase: domain.NewStateBase(s.TxnRunnerFactory())})

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmArchivePath(context.Background(), id)
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func insertCharmState(ctx context.Context, c *gc.C, tx *sql.Tx, uuid string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func insertCharmMetadata(ctx context.Context, c *gc.C, tx *sql.Tx, uuid string) (charm.Metadata, error) {
	if err := insertCharmState(ctx, c, tx, uuid); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	return charm.Metadata{
		Name:           "ubuntu",
		Summary:        "summary",
		Description:    "description",
		Subordinate:    true,
		RunAs:          charm.RunAsRoot,
		MinJujuVersion: version.MustParse("4.0.0"),
		Assumes:        []byte("null"),
	}, nil
}

func insertCharmManifest(ctx context.Context, c *gc.C, tx *sql.Tx, uuid string) (charm.Manifest, error) {
	if err := insertCharmState(ctx, c, tx, uuid); err != nil {
		return charm.Manifest{}, errors.Trace(err)
	}

	return charm.Manifest{}, nil
}

func assertTableEmpty(c *gc.C, runner coredatabase.TxnRunner, table string) {
	// Ensure that we don't use zero values for the count, as that would
	// pass if the table is empty.
	count := -1
	err := runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

func assertCharmMetadata(c *gc.C, metadata charm.Metadata, expected func() charm.Metadata) {
	c.Check(metadata, gc.DeepEquals, expected())
}

func assertCharmManifest(c *gc.C, manifest charm.Manifest, expected func() charm.Manifest) {
	c.Check(manifest, gc.DeepEquals, expected())
}

func setStateArgs() charm.SetStateArgs {
	return charm.SetStateArgs{
		Source:      charm.LocalSource,
		Revision:    1,
		Hash:        "hash",
		ArchivePath: "archive",
		Version:     "deadbeef",
	}
}
