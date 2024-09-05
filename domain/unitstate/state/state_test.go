package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID string
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	appState := applicationstate.NewApplicationState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	appArg := application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "app",
			},
		},
	}

	unitArg := application.UpsertUnitArg{UnitName: ptr("app/0")}

	_, err := appState.CreateApplication(context.Background(), "app", appArg, unitArg)
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetUUIDForName(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	var uuid string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		uuid, err = st.GetUnitUUIDForName(ctx, "app/0")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, s.unitUUID)
}

func (s *stateSuite) TestEnsureUnitStateRecord(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.EnsureUnitStateRecord(ctx, s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	var uuid string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)

	// Running again makes no change.
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.EnsureUnitStateRecord(ctx, s.unitUUID)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&uuid)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.unitUUID)
}
