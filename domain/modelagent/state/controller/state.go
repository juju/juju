package controller

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
)

// State is the means by which the model agent accesses the controller's state.
type State struct {
	*domain.StateBase
}

// NewState returns a new [State] object.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

func (s State) GetControllerAgentVersionsByArchitecture(
	ctx context.Context,
	architectures []agentbinary.Architecture,
) ([]semversion.Number, error) {
	return []semversion.Number{}, nil
}
