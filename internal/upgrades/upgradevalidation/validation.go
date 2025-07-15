// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
)

// ValidatorServices is a set of required services to perform upgrade validation.
// Nothing in this list can be unused. If they are no longer required, remove them.
type ValidatorServices struct {
	ModelAgentService ModelAgentService
	MachineService    MachineService
}

// Validator returns a blocker.
type Validator func(ctx context.Context, services ValidatorServices) (*Blocker, error)

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
	modelName  string
	services   ValidatorServices
	validators []Validator
}

// NewModelUpgradeCheck returns a ModelUpgradeCheck instance.
func NewModelUpgradeCheck(
	modelName string,
	services ValidatorServices,
	validators ...Validator,
) *ModelUpgradeCheck {
	return &ModelUpgradeCheck{
		modelName:  modelName,
		services:   services,
		validators: validators,
	}
}

// Validate runs the provided validators and returns blocks.
func (m *ModelUpgradeCheck) Validate(ctx context.Context) (*ModelUpgradeBlockers, error) {
	var blockers []Blocker
	for _, validator := range m.validators {
		if blocker, err := validator(ctx, m.services); err != nil {
			return nil, errors.Trace(err)
		} else if blocker != nil {
			blockers = append(blockers, *blocker)
		}
	}
	if len(blockers) == 0 {
		return nil, nil
	}
	return NewModelUpgradeBlockers(
		m.modelName, blockers...,
	), nil
}

// For testing.
// TODO: unexport it if we don't need to patch it anymore.
var SupportedJujuBases = corebase.WorkloadBases

func checkForDeprecatedUbuntuSeriesForModel(
	ctx context.Context,
	services ValidatorServices,
) (*Blocker, error) {
	supportedBases := SupportedJujuBases()

	// TODO(modelmigrations): this should be one call to machine domain.
	machineNames, err := services.MachineService.AllMachineNames(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get machine names")
	}
	baseCounts := map[corebase.Base]int{}
	for _, machineName := range machineNames {
		base, err := services.MachineService.GetMachineBase(ctx, machineName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		baseCounts[base]++
	}
	totalUnsupported := 0
	for base, count := range baseCounts {
		supported := false
		for _, supportedBase := range supportedBases {
			if supportedBase.IsCompatible(base) {
				supported = true
				break
			}
		}
		if !supported {
			totalUnsupported += count
		}
	}

	if totalUnsupported > 0 {
		return NewBlocker("the model hosts %d ubuntu machine(s) with an unsupported base. The supported bases are: %v",
			totalUnsupported,
			strings.Join(transform.Slice(supportedBases, func(b corebase.Base) string { return b.DisplayString() }), ", "),
		), nil
	}
	return nil, nil
}

func getCheckTargetVersionForControllerModel(
	targetVersion semversion.Number,
) Validator {
	return func(ctx context.Context, services ValidatorServices) (*Blocker, error) {
		agentVersion, err := services.ModelAgentService.GetModelTargetAgentVersion(ctx)
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
	targetVersion semversion.Number,
	versionChecker func(from, to semversion.Number) (bool, semversion.Number, error),
) Validator {
	return func(ctx context.Context, services ValidatorServices) (*Blocker, error) {
		agentVersion, err := services.ModelAgentService.GetModelTargetAgentVersion(ctx)
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

// For testing.
var NewServerFactory = lxd.NewServerFactory

func getCheckForLXDVersion(cloudspec environscloudspec.CloudSpec) Validator {
	return func(ctx context.Context, services ValidatorServices) (*Blocker, error) {
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
			return NewBlocker("%s", err.Error()), nil
		}
		return nil, errors.Trace(err)
	}
}
