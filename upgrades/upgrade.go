// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.upgrade")

// Step defines an idempotent operation that is run to perform
// a specific upgrade step.
type Step interface {
	// Description is a human readable description of what the upgrade step does.
	Description() string

	// Targets returns the target machine types for which the upgrade step is applicable.
	Targets() []Target

	// Run executes the upgrade business logic.
	Run(context Context) error
}

// Operation defines what steps to perform to upgrade to a target version.
type Operation interface {
	// The Juju version for which this operation is applicable.
	// Upgrade operations designed for versions of Juju earlier
	// than we are upgrading from are not run since such steps would
	// already have been used to get to the version we are running now.
	TargetVersion() version.Number

	// Steps to perform during an upgrade.
	Steps() []Step
}

// Target defines the type of machine for which a particular upgrade
// step can be run.
type Target string

const (
	// AllMachines applies to any machine.
	AllMachines = Target("allMachines")

	// HostMachine is a machine on which units are deployed.
	HostMachine = Target("hostMachine")

	// StateServer is a machine participating in a Juju state server cluster.
	StateServer = Target("stateServer")
)

// upgradeToVersion encapsulates the steps which need to be run to
// upgrade any prior version of Juju to targetVersion.
type upgradeToVersion struct {
	targetVersion version.Number
	steps         []Step
}

// Steps is defined on the Operation interface.
func (u upgradeToVersion) Steps() []Step {
	return u.steps
}

// TargetVersion is defined on the Operation interface.
func (u upgradeToVersion) TargetVersion() version.Number {
	return u.targetVersion
}

// Context is used give the upgrade steps attributes needed
// to do their job.
type Context interface {
	// APIState returns an API connection to state.
	APIState() *api.State

	// State returns a connection to state. This will be non-nil
	// only in the context of a state server.
	State() *state.State

	// AgentConfig returns the agent config for the machine that is being
	// upgraded.
	AgentConfig() agent.Config
}

// upgradeContext is a default Context implementation.
type upgradeContext struct {
	// Work in progress........
	// Exactly what a context needs is to be determined as the
	// implementation evolves.
	api         *api.State
	st          *state.State
	agentConfig agent.Config
}

// APIState is defined on the Context interface.
func (c *upgradeContext) APIState() *api.State {
	return c.api
}

// State is defined on the Context interface.
func (c *upgradeContext) State() *state.State {
	return c.st
}

// AgentConfig is defined on the Context interface.
func (c *upgradeContext) AgentConfig() agent.Config {
	return c.agentConfig
}

// NewContext returns a new upgrade context.
func NewContext(agentConfig agent.Config, api *api.State, st *state.State) Context {
	return &upgradeContext{
		api:         api,
		st:          st,
		agentConfig: agentConfig,
	}
}

// upgradeError records a description of the step being performed and the error.
type upgradeError struct {
	description string
	err         error
}

func (e *upgradeError) Error() string {
	return fmt.Sprintf("%s: %v", e.description, e.err)
}

// PerformUpgrade runs the business logic needed to upgrade the current "from" version to this
// version of Juju on the "target" type of machine.
func PerformUpgrade(from version.Number, target Target, context Context) error {
	// If from is not known, it is 1.16.
	if from == version.Zero {
		from = version.MustParse("1.16.0")
	}
	for _, upgradeOps := range upgradeOperations() {
		targetVersion := upgradeOps.TargetVersion()
		// Do not run steps for versions of Juju earlier or same as we are upgrading from.
		if targetVersion.Compare(from) <= 0 {
			continue
		}
		// Do not run steps for versions of Juju later than we are upgrading to.
		if targetVersion.Compare(version.Current.Number) > 0 {
			continue
		}
		if err := runUpgradeSteps(context, target, upgradeOps); err != nil {
			return err
		}
	}
	return nil
}

// validTarget returns true if target is in step.Targets().
func validTarget(target Target, step Step) bool {
	for _, opTarget := range step.Targets() {
		if opTarget == AllMachines || target == opTarget {
			return true
		}
	}
	return len(step.Targets()) == 0
}

// runUpgradeSteps runs all the upgrade steps relevant to target.
// As soon as any error is encountered, the operation is aborted since
// subsequent steps may required successful completion of earlier ones.
// The steps must be idempotent so that the entire upgrade operation can
// be retried.
func runUpgradeSteps(context Context, target Target, upgradeOp Operation) *upgradeError {
	for _, step := range upgradeOp.Steps() {
		if !validTarget(target, step) {
			continue
		}
		logger.Infof("Running upgrade step: %v", step.Description())
		if err := step.Run(context); err != nil {
			logger.Errorf("upgrade step %q failed: %v", step.Description(), err)
			return &upgradeError{
				description: step.Description(),
				err:         err,
			}
		}
	}
	logger.Infof("All upgrade steps completed successfully")
	return nil
}

type upgradeStep struct {
	description string
	targets     []Target
	run         func(Context) error
}

// Description is defined on the Step interface.
func (step *upgradeStep) Description() string {
	return step.description
}

// Targets is defined on the Step interface.
func (step *upgradeStep) Targets() []Target {
	return step.targets
}

// Run is defined on the Step interface.
func (step *upgradeStep) Run(context Context) error {
	return step.run(context)
}
