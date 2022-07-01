// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/v3/core/series"
	"github.com/juju/juju/v3/state"
)

// Validator returns a blocker.
type Validator func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error)

// Blocker describes a model upgrade blocker.
type Blocker struct {
	reason string
}

// NewBlocker returns a block.
func NewBlocker(format string, a ...any) *Blocker {
	return &Blocker{reason: fmt.Sprintf(format, a...)}
}

// String returns the Blocker as a string.
func (b Blocker) String() string {
	return fmt.Sprintf("\n- %s", b.reason)
}

func (b Blocker) Error() string {
	return b.reason
}

// ModelUpgradeBlockers holds a list of blockers for upgrading the provided model.
type ModelUpgradeBlockers struct {
	modelName string
	blockers  []Blocker
	next      *ModelUpgradeBlockers
}

// NewModelUpgradeBlockers creates a ModelUpgradeBlockers.
func NewModelUpgradeBlockers(modelName string, blockers ...Blocker) *ModelUpgradeBlockers {
	return &ModelUpgradeBlockers{modelName: modelName, blockers: blockers}
}

// String returns the ModelUpgradeBlockers as a string.
func (e ModelUpgradeBlockers) String() string {
	s := e.string()
	cursor := e.next
	for {
		if cursor == nil {
			return s
		}
		s += fmt.Sprintf("\n%s", cursor.string())
		cursor = cursor.next
	}
}

// Join links the provided ModelUpgradeBlockers as the next node.
func (e *ModelUpgradeBlockers) Join(next *ModelUpgradeBlockers) {
	e.tail().next = next
}

func (e *ModelUpgradeBlockers) tail() *ModelUpgradeBlockers {
	if e.next == nil {
		return e
	}
	tail := e.next
	for {
		if tail.next == nil {
			return tail
		}
		tail = tail.next
	}
}

func (e ModelUpgradeBlockers) string() string {
	if len(e.blockers) == 0 {
		return ""
	}
	errString := fmt.Sprintf("%q:", e.modelName)
	for _, b := range e.blockers {
		errString += b.String()
	}
	return errString
}

// ModelUpgradeCheck sumarizes a list of blockers for upgrading the provided model.
type ModelUpgradeCheck struct {
	modelUUID  string
	pool       StatePool
	state      State
	model      Model
	validators []Validator
}

// NewModelUpgradeCheck returns a ModelUpgradeCheck instance.
func NewModelUpgradeCheck(
	modelUUID string, pool StatePool, state State, model Model,
	validators ...Validator,
) *ModelUpgradeCheck {
	return &ModelUpgradeCheck{
		modelUUID:  modelUUID,
		pool:       pool,
		state:      state,
		model:      model,
		validators: validators,
	}
}

// Validate runs the provided validators and returns blocks.
func (m *ModelUpgradeCheck) Validate() (*ModelUpgradeBlockers, error) {
	var blockers []Blocker
	for _, validator := range m.validators {
		if blocker, err := validator(m.modelUUID, m.pool, m.state, m.model); err != nil {
			return nil, errors.Trace(err)
		} else if blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	if len(blockers) == 0 {
		return nil, nil
	}
	return NewModelUpgradeBlockers(
		fmt.Sprintf("%s/%s", m.model.Owner().Name(), m.model.Name()), blockers...,
	), nil
}

func getCheckUpgradeSeriesLockForModel(force bool) Validator {
	return func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
		locked, err := st.HasUpgradeSeriesLocks()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if locked && !force {
			return NewBlocker("unexpected upgrade series lock found"), nil
		}
		return nil, nil
	}
}

var windowsSeries = []string{
	"win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
	"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81", "win10",
}

func checkNoWinMachinesForModel(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	count, err := st.MachineCountForSeries(windowsSeries...)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot count machines for series %v", windowsSeries)
	}
	if count > 0 {
		return NewBlocker("windows is not supported but the model hosts %d windows machine(s)", count), nil
	}
	return nil, nil
}

func checkNoXenialMachinesForModel(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	xenial := series.Xenial.String()
	count, err := st.MachineCountForSeries(xenial)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot count machines for series %v", xenial)
	}
	if count > 0 {
		return NewBlocker("%s is not supported but the model hosts %d %s machine(s)", xenial, count, xenial), nil
	}
	return nil, nil
}

func getCheckTargetVersionForModel(
	targetVersion version.Number,
	versionChecker func(from, to version.Number) (bool, version.Number, error),
) Validator {
	return func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
		agentVersion, err := model.AgentVersion()
		if err != nil {
			return nil, errors.Trace(err)
		}

		allowed, minVer, err := versionChecker(agentVersion, targetVersion)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if allowed {
			return nil, nil
		}
		return NewBlocker(
			"current model (%q) has to be upgraded to %q at least", agentVersion, minVer,
		), nil
	}
}

func checkModelMigrationModeForControllerUpgrade(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	if mode := model.MigrationMode(); mode != state.MigrationModeNone {
		return NewBlocker("model is under %q mode, upgrade blocked", mode), nil
	}
	return nil, nil
}

func checkMongoStatusForControllerUpgrade(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	replicaStatus, err := st.MongoCurrentStatus()
	if err != nil {
		return nil, errors.Annotate(err, "cannot check replicaset status")
	}

	// Iterate over the replicaset, and record any nodes that aren't either
	// primary or secondary.
	var notes []string
	for _, member := range replicaStatus.Members {
		switch member.State {
		case replicaset.PrimaryState:
			// All good.
		case replicaset.SecondaryState:
			// Also good.
		default:
			msg := fmt.Sprintf("node %d (%s) has state %s", member.Id, member.Address, member.State)
			notes = append(notes, msg)
		}
	}
	if len(notes) > 0 {
		return NewBlocker("unable to upgrade, database %s", strings.Join(notes, ", ")), nil
	}
	return nil, nil
}

func checkMongoVersionForControllerModel(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	v, err := pool.MongoVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !strings.Contains(v, "4.4") {
		// Controllers with mongo version != 4.4 are not able to be upgraded further.
		return NewBlocker(
			`mongo version has to be "4.4" at least, but current version is %q`, v,
		), nil
	}
	return nil, nil
}
