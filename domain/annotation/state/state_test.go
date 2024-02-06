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
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&stateSuite{})

// TestGetAnnotationsHappyPath asserts the happy path, retrieves annotations from the DB.
func (s *stateSuite) TestGetAnnotationsHappyPath(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.manuallyInsertMachine("my-machine", "123", c)

	s.manuallyInsertAnnotation("machine", "123", "foo", "5", c)
	s.manuallyInsertAnnotation("machine", "123", "bar", "6", c)

	annotations, err := st.GetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindMachine,
		Name: "my-machine",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annotations, gc.HasLen, 2)
}

// TestGetAnnotationsHappyPath asserts the happy path, retrieves annotations from the DB.
func (s *stateSuite) TestGetAnnotationsHappyPathModel(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())

	s.manuallyInsertAnnotation("model", "", "foo", "5", c)
	s.manuallyInsertAnnotation("model", "", "bar", "6", c)

	annotations, err := st.GetAnnotations(context.Background(), annotations.ID{
		Kind: annotations.KindModel,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(annotations, gc.HasLen, 2)
}

// TestGetAnnotationsHappyPath asserts the happy path of the utility uuidQueryForID
func (s *stateSuite) TestUUIDQueryForID(c *gc.C) {
	// machine
	kindQuery, kindQueryParam, _ := uuidQueryForID(annotations.ID{Kind: annotations.KindMachine, Name: "my-machine"})
	c.Check(kindQuery, gc.Equals, `SELECT &M.uuid FROM machine WHERE machine_id = $M.entity_id`)
	c.Check(kindQueryParam, gc.DeepEquals, sqlair.M{"entity_id": "my-machine"})

	// application
	kindQuery, kindQueryParam, _ = uuidQueryForID(annotations.ID{Kind: annotations.KindApplication, Name: "appname"})
	c.Check(kindQuery, gc.Equals, `SELECT &M.uuid FROM application WHERE name = $M.entity_id`)
	c.Check(kindQueryParam, gc.DeepEquals, sqlair.M{"entity_id": "appname"})

	// charm
	kindQuery, kindQueryParam, _ = uuidQueryForID(annotations.ID{Kind: annotations.KindCharm, Name: "charmurl"})
	c.Check(kindQuery, gc.Equals, `SELECT &M.uuid FROM charm WHERE url = $M.entity_id`)
	c.Check(kindQueryParam, gc.DeepEquals, sqlair.M{"entity_id": "charmurl"})
}

// manuallyInsertAnnotation is a test utility that manually adds a row to an annotation table
//
// s.manuallyInsertAnnotations("machine", "uuid123", "keyfoo", "valuebar") will add the row
// (uuid123 keyfoo valuebar) into the annotation_machine table
//
// If the id is model, it'll just ignore the uuid and add the key value pair into the
// annotation_model table
func (s *stateSuite) manuallyInsertAnnotation(id, uuid, key, value string, c *gc.C) {
	if id == "model" {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO annotation_model (key, value)
				VALUES (?, ?)`, key, value)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	} else {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, fmt.Sprintf(`
				INSERT INTO annotation_%[1]s (%[1]s_uuid, key, value)
				VALUES (?, ?, ?)`, id), uuid, key, value)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *stateSuite) manuallyInsertMachine(id, uuid string, c *gc.C) {
	// Manually insert a machine with uuid
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO net_node (uuid)
		VALUES ("node2")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO life (id, value)
		VALUES ("life3", "3")`)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		INSERT INTO machine (uuid, net_node_uuid, machine_id, life_id)
		VALUES (?, "node2", ?, "life3")`, uuid, id)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
