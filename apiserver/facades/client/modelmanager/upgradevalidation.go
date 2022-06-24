// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/replicaset/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/series"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

// Validator returns a blocker.
type Validator func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error)

// Blocker describes a model upgrade blocker.
type Blocker struct {
	reason string
}

func NewBlocker(format string, a ...any) *Blocker {
	return &Blocker{reason: fmt.Sprintf(format, a...)}
}

func (b Blocker) String() string {
	return fmt.Sprintf("\n\t%s", b.reason)
}

func (b Blocker) Error() string {
	return b.reason
}

type ModelUpgradeBlockers struct {
	modelName string
	blockers  []Blocker
	next      *ModelUpgradeBlockers
}

// NewModelUpgradeBlockers creates a ModelUpgradeBlockers.
func NewModelUpgradeBlockers(modelName string, blockers ...Blocker) *ModelUpgradeBlockers {
	return &ModelUpgradeBlockers{modelName: modelName, blockers: blockers}
}

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

func (e *ModelUpgradeBlockers) Append(next *ModelUpgradeBlockers) {
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
	errString := fmt.Sprintf("model %q:", e.modelName)
	for _, b := range e.blockers {
		errString += b.String()
	}
	return errString
}

type ModelUpgradeCheck struct {
	modelUUID  string
	pool       StatePool
	state      State
	validators []Validator
}

// NewModelUpgradeCheck returns a ModelUpgradeCheck instance.
func NewModelUpgradeCheck(modelUUID string, pool StatePool, state State, validators ...Validator) *ModelUpgradeCheck {
	return &ModelUpgradeCheck{
		modelUUID:  modelUUID,
		pool:       pool,
		state:      state,
		validators: validators,
	}
}

// Validate runs the provided validators and returns blocks.
func (m *ModelUpgradeCheck) Validate() (*ModelUpgradeBlockers, error) {
	model, err := m.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var blockers []Blocker
	for _, validator := range m.validators {
		if blocker, err := validator(m.modelUUID, m.pool, m.state, model); err != nil {
			return nil, errors.Trace(err)
		} else if blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	if len(blockers) == 0 {
		return nil, nil
	}
	return NewModelUpgradeBlockers(
		fmt.Sprintf("%s/%s", model.Owner().Name(), model.Name()), blockers...,
	), nil
}

func validateControllerModel(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	return nil, nil
}

func validateModelForControllerUpgrade(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	return nil, nil
}

func validateModelForModelUpgrade(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	return nil, nil
}

func checkNoWinMachinesForModel(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
	var winSeries []string
	for _, v := range series.WindowsVersions() {
		winSeries = append(winSeries, v)
		sort.Strings(winSeries)
	}

	winMachineCount, err := st.MachineCountForSeries(winSeries...)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot count machines for series %v", winSeries)
	}
	if winMachineCount > 0 {
		return NewBlocker("model hosts %d windows machine(s)", winMachineCount), nil
	}
	return nil, nil
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

func getCheckTargetVersionForModel(targetVersion version.Number) Validator {
	return func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
		agentVersion, err := model.AgentVersion()
		if err != nil {
			return nil, errors.Trace(err)
		}

		// TODO: move the upgrades/model.go
		allowed, minVer, err := upgrades.UpgradeAllowed(agentVersion, targetVersion)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if allowed {
			return nil, nil
		}
		return NewBlocker(
			"upgrade current version %q to at least %q before upgrading to %q",
			agentVersion, minVer, targetVersion,
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
	replicaStatus, err := st.MongoSession().CurrentStatus()
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
	mongoVersion, err := mongo.NewVersion(v)
	if err != nil {
		return nil, errors.Trace(err)
	}
	mongoVersion.StorageEngine = ""
	mongoMinVersion := mongo.Version{Major: 4, Minor: 4}
	if mongoVersion.NewerThan(mongoMinVersion) < 0 {
		// Controllers with mongo version < 4.4 are not able to be upgraded further.
		return NewBlocker(
			"mongo version has to be %q at least, but current version is %q",
			mongoMinVersion, mongoVersion,
		), nil
	}
	return nil, nil
}
