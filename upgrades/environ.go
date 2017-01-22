// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	jujuversion "github.com/juju/juju/version"
)

// newEnvironUpgradeOpsIterator returns an opsIterator that yields
// operations for upgrading all environs managed by the controller.
//
// These operations are run by the DatabaseMaster target only.
func newEnvironUpgradeOpsIterator(from version.Number, context Context) (*opsIterator, error) {
	st := context.State()
	models, err := st.AllModels()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var envUpgradeOps []environs.UpgradeOperation
	for _, model := range models {
		env, err := environs.GetEnviron(environConfigGetter{model}, context.NewEnviron)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if env, ok := env.(environs.Upgrader); ok {
			envUpgradeOps = append(envUpgradeOps, env.UpgradeOperations()...)
		}
	}
	ops := make([]Operation, len(envUpgradeOps))
	for i, envUpgradeOp := range envUpgradeOps {
		ops[i] = newEnvironUpgradeOperation(envUpgradeOp)
	}
	return newOpsIterator(from, jujuversion.Current, ops), nil
}

type environConfigGetter struct {
	m Model
}

func (e environConfigGetter) ModelConfig() (*config.Config, error) {
	return e.m.Config()
}

func (e environConfigGetter) CloudSpec(names.ModelTag) (environs.CloudSpec, error) {
	return e.m.CloudSpec()
}

func newEnvironUpgradeOperation(op environs.UpgradeOperation) Operation {
	steps := make([]Step, len(op.Steps))
	for i, step := range op.Steps {
		steps[i] = newEnvironUpgradeStep(step)
	}
	return upgradeToVersion{op.TargetVersion, steps}
}

func newEnvironUpgradeStep(step environs.UpgradeStep) Step {
	return &upgradeStep{
		step.Description(),
		[]Target{DatabaseMaster},
		func(Context) error {
			return step.Run()
		},
	}
}
