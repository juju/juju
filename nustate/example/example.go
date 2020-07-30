// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package example

import (
	"fmt"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/nustate/operation"
	"github.com/juju/juju/nustate/persistence"
	"github.com/juju/juju/nustate/persistence/transaction"
)

func FacadeExample(st persistence.Store) error {
	portOp, err := operation.MutateMachinePortRanges(st, "machine-123")
	if err != nil {
		return err
	}

	portOp.ForUnit("foo/0").
		Open("", network.MustParsePortRange("42/udp")).
		Open("", network.MustParsePortRange("8080/tcp")).
		Close("", network.MustParsePortRange("1234/tcp"))

	ctx, err := doApply(st, portOp)
	if err == nil {
		fmt.Printf("txn applied in %v after %d attempts", ctx.Attempt, ctx.ElapsedTime)
	}
	return err
}

func doApply(st persistence.Store, txnElems ...transaction.Element) (transaction.Context, error) {
	return st.ApplyTxn(txnElems)
}
