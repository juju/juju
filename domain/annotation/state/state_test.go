// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/annotation"
	annotationerrors "github.com/juju/juju/domain/annotation/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/charm"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestGetAnnotations(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureMachine(c, "my-machine", "123")

	s.ensureAnnotation(c, "machine", "123", "foo", "5")
	s.ensureAnnotation(c, "machine", "123", "bar", "6")

	annotations, err := st.GetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindMachine,
		Name: "my-machine",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.HasLen, 2)
}

func (s *stateSuite) TestGetCharmAnnotations(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureCharm(c, "local:mycharmurl-5", "mystorage", "123")
	s.ensureAnnotation(c, "charm", "123", "foo", "5")
	s.ensureAnnotation(c, "charm", "123", "bar", "6")

	annotations, err := st.GetCharmAnnotations(context.Background(), annotation.GetCharmArgs{
		Source:   "local",
		Name:     "mycharmurl",
		Revision: 5,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.DeepEquals, map[string]string{"foo": "5", "bar": "6"})
}

func (s *stateSuite) TestGetAnnotationsModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureAnnotation(c, "model", "", "foo", "5")
	s.ensureAnnotation(c, "model", "", "bar", "6")

	annotations, err := st.GetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindModel,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.HasLen, 2)
}

func (s *stateSuite) TestSetAnnotations(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add a machine into the TABLE machine
	s.ensureMachine(c, "my-machine", "123")

	id := annotations.ID{
		Kind: annotations.KindMachine,
		Name: "my-machine",
	}

	err := st.SetAnnotations(context.Background(), id, map[string]string{"bar": "6", "foo": "15"})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.DeepEquals, map[string]string{"bar": "6", "foo": "15"})

	err = st.SetAnnotations(context.Background(), id, map[string]string{"bar": "6", "baz": "7"})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations, err = st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.DeepEquals, map[string]string{"bar": "6", "baz": "7"})
}

func (s *stateSuite) TestSetCharmAnnotations(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureCharm(c, "local:mycharmurl-5", "mystorage", "123")

	args := annotation.GetCharmArgs{
		Source:   "local",
		Name:     "mycharmurl",
		Revision: 5,
	}

	// Set annotations bar:6 and foo:15
	err := st.SetCharmAnnotations(context.Background(), args, map[string]string{"bar": "6", "foo": "15"})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations, err := st.GetCharmAnnotations(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.DeepEquals, map[string]string{"bar": "6", "foo": "15"})

	// Set annotations bar:6 and foo:15
	err = st.SetCharmAnnotations(context.Background(), args, map[string]string{"bar": "6", "baz": "7"})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations, err = st.GetCharmAnnotations(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations, gc.DeepEquals, map[string]string{"bar": "6", "baz": "7"})
}

// TestSetAnnotationsUpdateMachine asserts the happy path, updates some
// annotations in the DB for a Machine ID.
func (s *stateSuite) TestSetAnnotationsUpdateMachine(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureMachine(c, "my-machine", "123")
	s.ensureAnnotation(c, "machine", "123", "foo", "5")

	testAnnotationUpdate(c, st, annotations.ID{
		Kind: annotations.KindMachine,
		Name: "my-machine",
	})
}

// TestSetAnnotationsUpdateApplication asserts the happy path, updates some
// annotations in the DB for an Application ID.
func (s *stateSuite) TestSetAnnotationsUpdateApplication(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureApplication(c, "myapp", "123")
	s.ensureAnnotation(c, "application", "123", "foo", "5")

	testAnnotationUpdate(c, st, annotations.ID{
		Kind: annotations.KindApplication,
		Name: "myapp",
	})
}

// TestSetAnnotationsUpdateUnit asserts the happy path, updates some annotations
// in the DB for a Unit ID.
func (s *stateSuite) TestSetAnnotationsUpdateUnit(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureUnit(c, "unit3", "123")
	s.ensureAnnotation(c, "unit", "123", "foo", "5")

	testAnnotationUpdate(c, st, annotations.ID{
		Kind: annotations.KindUnit,
		Name: "unit3",
	})
}

// TestSetAnnotationsUpdateStorage asserts the happy path, updates some
// annotations in the DB for a Storage ID.
func (s *stateSuite) TestSetAnnotationsUpdateStorage(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureCharm(c, "mystorage", "mystorage", "123")
	s.ensureStorage(c, "mystorage", "456", "123")
	s.ensureAnnotation(c, "storage_instance", "456", "foo", "5")

	testAnnotationUpdate(c, st, annotations.ID{
		Kind: annotations.KindStorage,
		Name: "mystorage/0",
	})
}

// TestSetAnnotationsUpdateModel asserts the happy path, updates some
// annotations in the DB for a Model ID.
func (s *stateSuite) TestSetAnnotationsUpdateModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureAnnotation(c, "model", "", "foo", "5")

	testAnnotationUpdate(c, st, annotations.ID{
		Kind: annotations.KindModel,
		Name: "",
	})
}

// testAnnotationUpdate checks if the given ID has a {foo:5} annotation
// already attached to it (so ensureAnnotation needs to be called with the ID
// before this), then updates the annotations with
// {bar:6, foo:15} and validates that it's actually updated.
func testAnnotationUpdate(c *gc.C, st *State, id annotations.ID) {
	// Check that we only have the foo:5
	annotations1, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations1, gc.DeepEquals, map[string]string{"foo": "5"})

	// Add bar:6 and update foo:15
	err = st.SetAnnotations(context.Background(), id, map[string]string{"bar": "6", "foo": "15"})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations2, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations2, gc.DeepEquals, map[string]string{"bar": "6", "foo": "15"})
}

// TestSetAnnotationsUnset asserts the happy path, unsets some annotations in
// the DB for an ID.
func (s *stateSuite) TestSetAnnotationsUnset(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	// Add a machine into the TABLE machine and an annotation (to be updated)
	s.ensureMachine(c, "my-machine", "123")
	s.ensureAnnotation(c, "machine", "123", "foo", "5")

	id := annotations.ID{
		Kind: annotations.KindMachine,
		Name: "my-machine",
	}

	// Check that we only have the foo:5
	annotations1, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(annotations1, gc.DeepEquals, map[string]string{"foo": "5"})

	// Unset foo
	err = st.SetAnnotations(context.Background(), id, map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations2, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annotations2, gc.HasLen, 0)
}

// TestSetAnnotationsUnsetModel asserts the happy path, unsets some annotations
// in the DB for a model ID.
func (s *stateSuite) TestSetAnnotationsUnsetModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.ensureAnnotation(c, "model", "", "foo", "5")

	id := annotations.ID{
		Kind: annotations.KindModel,
	}

	// Unset foo
	err := st.SetAnnotations(context.Background(), id, map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	// Check the final annotation set
	annotations2, err := st.GetAnnotations(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annotations2, gc.HasLen, 0)
}

// TestUUIDQueryForID asserts the happy path of the utility uuidQueryForID
func (s *stateSuite) TestUUIDQueryForIDMachine(c *gc.C) {
	kindQuery, kindQueryParam, err := uuidQueryForID(annotations.ID{Kind: annotations.KindMachine, Name: "my-machine"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(kindQuery, gc.Equals, `SELECT &annotationUUID.uuid FROM machine WHERE name = $M.entity_name`)
	c.Check(kindQueryParam, gc.DeepEquals, sqlair.M{"entity_name": "my-machine"})
}

func (s *stateSuite) TestUUIDQueryForIDApplication(c *gc.C) {
	kindQuery, kindQueryParam, err := uuidQueryForID(annotations.ID{Kind: annotations.KindApplication, Name: "appname"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(kindQuery, gc.Equals, `SELECT &annotationUUID.uuid FROM application WHERE name = $M.entity_name`)
	c.Check(kindQueryParam, gc.DeepEquals, sqlair.M{"entity_name": "appname"})
}

// TestKindNameFromID asserts the mapping of annotation.Kind -> actual table
// names
// Keeping these explicit here should ensure we quickly detect any changes in
// the future.
func (s *stateSuite) TestAnnotationTableNameFromID(c *gc.C) {
	t1, err := annotationTableNameFromID(annotations.ID{Kind: annotations.KindMachine, Name: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(t1, gc.Equals, "annotation_machine")

	t2, err := annotationTableNameFromID(annotations.ID{Kind: annotations.KindUnit, Name: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(t2, gc.Equals, "annotation_unit")

	t3, err := annotationTableNameFromID(annotations.ID{Kind: annotations.KindApplication, Name: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(t3, gc.Equals, "annotation_application")

	t4, err := annotationTableNameFromID(annotations.ID{Kind: annotations.KindStorage, Name: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(t4, gc.Equals, "annotation_storage_instance")

	t6, err := annotationTableNameFromID(annotations.ID{Kind: annotations.KindModel, Name: "foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(t6, gc.Equals, "annotation_model")

	_, err = annotationTableNameFromID(annotations.ID{Kind: 12, Name: "foo"})
	c.Assert(err, jc.ErrorIs, annotationerrors.UnknownKind)

}

// ensureAnnotation is a test utility that manually adds a row to an annotation
// table.
//
// s.manuallyInsertAnnotations("machine", "uuid123", "keyfoo", "valuebar")
// will add the row (uuid123 keyfoo valuebar) into the annotation_machine table
//
// If the id is model, it'll just ignore the uuid and add the key value pair
// into the annotation_model table.
func (s *stateSuite) ensureAnnotation(c *gc.C, id, uuid, key, value string) {
	if id == "model" {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO annotation_model (key, value)
VALUES (?, ?)
				`, key, value)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	} else {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO annotation_%[1]s (uuid, key, value)
VALUES (?, ?, ?)
				`, id), uuid, key, value)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

// ensureMachine manually inserts a row into the machine table.
func (s *stateSuite) ensureMachine(c *gc.C, id, uuid string) {
	s.ensureNetNode(c, "node2")
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES (?, "node2", ?, "0")`, uuid, id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureApplication manually inserts a row into the application table.
func (s *stateSuite) ensureApplication(c *gc.C, name, uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)`, uuid, name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')`, uuid)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, uuid, uuid, name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureUnit manually inserts a row into the unit table.
func (s *stateSuite) ensureUnit(c *gc.C, unitName, uuid string) {
	s.ensureApplication(c, "myapp", "234")
	s.ensureCharm(c, "local:mycharmurl-5", "mystorage", "345")
	s.ensureNetNode(c, "456")

	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, ?)
`, uuid, unitName, "234", "345", "456", "0")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureCharm manually inserts a row into the charm table.
func (s *stateSuite) ensureCharm(c *gc.C, url, storageName, uuid string) {
	parts, err := charm.ParseURL(url)
	c.Assert(err, jc.ErrorIsNil)

	source := 0
	if charm.CharmHub.Matches(parts.Schema) {
		source = 1
	}

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, ?, ?, ?, 0)`, uuid, source, parts.Name, parts.Revision)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, ?)
		`, uuid, parts.Name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, ?, ?, ?)
		`, uuid, storageName, 0, 0, 1)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureStorage inserts a row into the storage_instance table
func (s *stateSuite) ensureStorage(c *gc.C, name, uuid, charmUUID string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance (uuid, storage_id, storage_type, requested_size_mib, charm_uuid, storage_name, life_id)
VALUES (?, ?, ?, ?, ?, ?, ?)
		`, uuid, name+"/0", "loop", 100, charmUUID, name, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// ensureNetNode inserts a row into the net_node table, mostly used as a foreign
// key for entries in other tables (e.g. machine)
func (s *stateSuite) ensureNetNode(c *gc.C, uuid string) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, uuid)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
