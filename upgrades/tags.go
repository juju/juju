// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
)

func addInstanceTags(env environs.Environ, machines []*state.Machine) error {
	cfg := env.Config()
	tagger, ok := env.(environs.InstanceTagger)
	if !ok {
		logger.Debugf("environment type %q does not support instance tagging", cfg.Type())
		return nil
	}

	// Tag each top-level, provisioned machine.
	logger.Infof("adding tags to existing machine instances")
	for _, m := range machines {
		if names.IsContainerMachine(m.Id()) {
			continue
		}
		instId, err := m.InstanceId()
		if errors.IsNotProvisioned(err) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "getting instance ID for machine %v", m.Id())
		}

		stateMachineJobs := m.Jobs()
		paramsMachineJobs := make([]multiwatcher.MachineJob, len(stateMachineJobs))
		for i, job := range stateMachineJobs {
			paramsMachineJobs[i] = job.ToParams()
		}

		tags := instancecfg.InstanceTags(cfg, paramsMachineJobs)
		logger.Infof("tagging instance %v: %v", instId, tags)
		if err := tagger.TagInstance(instId, tags); err != nil {
			return errors.Annotatef(err, "tagging instance %v for machine %v", instId, m.Id())
		}
	}

	return nil
}
