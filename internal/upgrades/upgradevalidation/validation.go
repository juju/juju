// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/replicaset/v3"
	"github.com/juju/version/v2"

	corebase "github.com/juju/juju/core/base"
	corelogger "github.com/juju/juju/core/logger"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
	"github.com/juju/juju/state"
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

// For testing.
// TODO: unexport it if we don't need to patch it anymore.
var SupportedJujuBases = corebase.WorkloadBases

func checkForDeprecatedUbuntuSeriesForModel(
	_ string, _ StatePool, st State, _ Model,
) (*Blocker, error) {
	supportedBases, err := SupportedJujuBases(time.Now(), corebase.Base{}, "")
	if err != nil {
		return nil, errors.Annotate(err, "cannot ready supported bases")
	}

	stateBases := transform.Slice(supportedBases, func(b corebase.Base) state.Base {
		return state.Base{OS: b.OS, Channel: b.Channel.String()}
	})
	baseCountMap, err := st.MachineCountForBase(stateBases...)
	if err != nil {
		return nil, errors.Annotate(err, "cannot count deprecated ubuntu machines")
	}
	allSupportedCount := 0
	for _, v := range baseCountMap {
		allSupportedCount += v
	}

	allMachinesCount, err := st.AllMachinesCount()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all machines count")
	}

	if totalUnsupported := allMachinesCount - allSupportedCount; totalUnsupported > 0 {
		return NewBlocker("the model hosts %d ubuntu machine(s) with an unsupported base. The supported bases are: %v",
			totalUnsupported,
			strings.Join(transform.Slice(supportedBases, func(b corebase.Base) string { return b.DisplayString() }), ", "),
		), nil
	}
	return nil, nil
}

func getCheckTargetVersionForControllerModel(
	targetVersion version.Number,
) Validator {
	return func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
		agentVersion, err := model.AgentVersion()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if targetVersion.Major == agentVersion.Major &&
			targetVersion.Minor == agentVersion.Minor {
			return nil, nil
		}

		return NewBlocker(
			"upgrading a controller to a newer major.minor version %d.%d not supported", targetVersion.Major, targetVersion.Minor,
		), nil
	}
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

func checkMongoVersionForControllerModel(_ string, pool StatePool, _ State, _ Model) (*Blocker, error) {
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

// For testing.
var NewServerFactory = lxd.NewServerFactory

func getCheckForLXDVersion(cloudspec environscloudspec.CloudSpec) Validator {
	return func(modelUUID string, pool StatePool, st State, model Model) (*Blocker, error) {
		if !lxdnames.IsDefaultCloud(cloudspec.Type) {
			return nil, nil
		}
		server, err := NewServerFactory(lxd.NewHTTPClientFunc(func() *http.Client {
			return jujuhttp.NewClient(
				jujuhttp.WithLogger(logger.Child("http", corelogger.HTTP)),
			).Client()
		})).RemoteServer(lxd.CloudSpec{CloudSpec: cloudspec})
		if err != nil {
			return nil, errors.Trace(err)
		}
		err = lxd.ValidateAPIVersion(server.ServerVersion())
		if errors.Is(err, errors.NotSupported) {
			return NewBlocker(err.Error()), nil
		}
		return nil, errors.Trace(err)
	}
}
