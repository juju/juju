// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetCharmIDByRevision(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsControllerCharmWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsControllerCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsControllerCharmWithControllerCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsControllerCharmWithNoControllerCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsSubordinateCharmWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsSubordinateCharm(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsSubordinateCharmWithSubordinateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsSubordinateCharmWithNoSubordinateCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestSupportsContainersWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.SupportsContainers(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestSupportsContainersWithContainers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestSupportsContainersWithNoContainers(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsCharmAvailableWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.IsCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestIsCharmAvailableWithAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestIsCharmAvailableWithNotAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestSetCharmAvailableWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	err := st.SetCharmAvailable(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestSetCharmAvailable(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestReserveCharmRevisionWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.ReserveCharmRevision(context.Background(), id, 1)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestReserveCharmRevision(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

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

func (s *stateSuite) TestGetCharmMetadataWithNoCharm(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)

	_, err := st.GetCharmMetadata(context.Background(), id)
	c.Assert(err, jc.ErrorIs, charmerrors.NotFound)
}

func (s *stateSuite) TestGetCharmMetadata(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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

func (s *stateSuite) TestGetCharmMetadataWithTagsAndCategories(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_category (charm_uuid, value) VALUES (?, 'data'), (?, 'kubernetes')`, uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_tag (charm_uuid, value) VALUES (?, 'foo'), (?, 'bar')`, uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		Tags:           []string{"bar", "foo"},
		Categories:     []string{"data", "kubernetes"},
	})
}

func (s *stateSuite) TestGetCharmMetadataWithTerms(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_term (charm_uuid, value) VALUES (?, 'alpha'), (?, 'beta')`, uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		Terms:          []string{"alpha", "beta"},
	})
}

func (s *stateSuite) TestGetCharmMetadataWithRelation(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that relations are correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_relation (charm_uuid, kind_id, key, name, role_id) 
VALUES 
    (?, 0, 'foo', 'baz', 0),
    (?, 0, 'fred', 'bar', 0),
    (?, 1, 'foo', 'baz', 1),
    (?, 2, 'foo', 'baz', 2);`,
			uuid, uuid, uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		Provides: map[string]charm.Relation{
			"foo": {
				Key:  "foo",
				Name: "baz",
				Role: charm.RoleProvider,
			},
			"fred": {
				Key:  "fred",
				Name: "bar",
				Role: charm.RoleProvider,
			},
		},
		Requires: map[string]charm.Relation{
			"foo": {
				Key:  "foo",
				Name: "baz",
				Role: charm.RoleRequirer,
			},
		},
		Peers: map[string]charm.Relation{
			"foo": {
				Key:  "foo",
				Name: "baz",
				Role: charm.RolePeer,
			},
		},
	})
}

func (s *stateSuite) TestGetCharmMetadataWithExtraBindings(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_extra_binding (charm_uuid, key, name) 
VALUES 
    (?, 'foo', 'bar'),
    (?, 'fred', 'baz');`,
			uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		ExtraBindings: map[string]charm.ExtraBinding{
			"foo": {
				Name: "bar",
			},
			"fred": {
				Name: "baz",
			},
		},
	})
}

func (s *stateSuite) TestGetCharmMetadataWithStorageWithNoProperties(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithStorageWithProperties(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage_property (
    charm_uuid,
    charm_storage_key,
    value
) VALUES
    (?, 'foo', 'alpha'),
    (?, 'foo', 'beta');`,
			uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
				Properties:  []string{"alpha", "beta"},
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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithDevices(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithPayloadClasses(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithResources(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithContainersWithNoMounts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

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
	})
}

func (s *stateSuite) TestGetCharmMetadataWithContainersWithMounts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	id := charmtesting.GenCharmID(c)
	uuid := id.String()

	// Ensure that storage with no properties is correctly extracted.

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, name, description, summary, subordinate, min_juju_version, run_as_id, assumes) 
VALUES (?, 'ubuntu', 'description', 'summary', true, '4.0.0', 1, 'null')`, uuid)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `INSERT INTO charm_state (charm_uuid, available) VALUES (?, false)`, uuid)
		c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_container_mount (
    charm_uuid,
    "index",
    charm_container_key,
    storage,
    location
) VALUES
	(?, 0, 'foo', 'block', '/tmp'),
	(?, 1, 'foo', 'block', '/dev/nvme0n1'),
	(?, 0, 'fred', 'file', '/var/log');`,
			uuid, uuid, uuid)
		c.Assert(err, jc.ErrorIsNil)
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
	})
}

func ptr[T any](v T) *T {
	return &v
}
