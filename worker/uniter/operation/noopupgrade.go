// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/uniter/hook"
)

type noOpUpgrade struct {
	Operation

	charmURL *charm.URL
}

// String is part of the Operation interface.
func (op *noOpUpgrade) String() string {
	return fmt.Sprintf("no-op upgrade operation to %v", op.charmURL.String())
}

// Commit is part of the Operation interface.
func (op *noOpUpgrade) Commit(state State) (*State, error) {
	change := stateChange{
		Kind: RunHook,
		Step: Queued,
		Hook: &hook.Info{Kind: hooks.UpgradeCharm},
	}
	newState := change.apply(state)
	return newState, nil
}
