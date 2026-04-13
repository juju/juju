// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/constraints"
	constraintsmigration "github.com/juju/juju/domain/constraints/modelmigration"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, clock clock.Clock, logger logger.Logger) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
	clock   clock.Clock
	logger  logger.Logger
}

// ImportService defines the machine service used to import machines from
// another controller model to this controller.
type ImportService interface {
	// CreateMachine creates the specified machine.
	CreateMachine(
		ctx context.Context,
		hostname string,
		machineName machine.Name,
		nonce *string,
		platform deployment.Platform,
		placement deployment.Placement,
		cons constraints.Constraints,
	) (machine.UUID, error)

	// CreateSubordinateMachine creates the specified subordinate machine.
	CreateSubordinateMachine(
		ctx context.Context,
		hostname string,
		machineName machine.Name,
		parentUUID machine.UUID,
		nonce *string,
		platform deployment.Platform,
		placement deployment.Placement,
		cons constraints.Constraints,
	) (machine.UUID, error)

	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(
		ctx context.Context,
		machineUUID machine.UUID,
		instanceID instance.Id,
		displayName,
		nonce string,
		hardwareCharacteristics *instance.HardwareCharacteristics,
	) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import machines"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewState(scope.ModelDB(), i.clock, i.logger),
		domain.NewStatusHistory(i.logger, i.clock),
		i.clock,
		i.logger,
	)
	return nil
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, m := range model.Machines() {
		osType, channel, err := encodeBaseFromMachine(m)
		if err != nil {
			return errors.Errorf("importing machine %q: %w", m.Id(), err)
		}

		arch, err := encodeArchitectureFromMachine(m.Constraints(), m.Instance())
		if err != nil {
			return errors.Errorf("importing machine %q: %w", m.Id(), err)
		}
		machinePlatform := deployment.Platform{
			Channel:      channel,
			OSType:       osType,
			Architecture: arch,
		}

		placement, err := instance.ParsePlacement(m.Placement())
		if err != nil {
			return errors.Errorf("parsing machine instance placement %q: %w", m.Placement(), err)
		}
		domainPlacement, err := deployment.ParsePlacement(placement)
		if err != nil {
			return errors.Errorf("parsing machine domain placement %q: %w", m.Id(), err)
		}

		cons := constraintsmigration.DecodeConstraints(m.Constraints())

		// We need skeleton machines in dqlite.
		machineUUID, err := i.service.CreateMachine(
			ctx,
			m.Hostname(),
			machine.Name(m.Id()),
			new(m.Nonce()),
			machinePlatform,
			domainPlacement,
			constraints.DecodeConstraints(cons))
		if err != nil {
			return errors.Errorf("importing machine %q: %w", m.Id(), err)
		}

		if err := i.importCloudInstance(ctx, machineUUID, m); err != nil {
			return err
		}

		containers := m.Containers()
		for _, c := range containers {
			placement, err := instance.ParsePlacement(c.Placement())
			if err != nil {
				return errors.Errorf("parsing machine instance placement %q: %w", c.Placement(), err)
			}
			domainPlacement, err := deployment.ParsePlacement(placement)
			if err != nil {
				return errors.Errorf("parsing machine domain placement %q: %w", c.Id(), err)
			}

			cons := constraintsmigration.DecodeConstraints(c.Constraints())

			containerUUID, err := i.service.CreateSubordinateMachine(
				ctx,
				c.Hostname(),
				machine.Name(c.Id()),
				machineUUID,
				new(c.Nonce()),
				machinePlatform,
				domainPlacement,
				constraints.DecodeConstraints(cons))
			if err != nil {
				return errors.Errorf("importing container machine %q: %w", c.Id(), err)
			}

			if err := i.importCloudInstance(ctx, containerUUID, c); err != nil {
				return err
			}
		}

	}
	return nil
}

func (i *importOperation) importCloudInstance(ctx context.Context, machineUUID machine.UUID, m description.Machine) error {
	// Import the machine's cloud instance.
	cloudInstance := m.Instance()
	if cloudInstance == nil {
		return nil
	}

	hardwareCharacteristics := &instance.HardwareCharacteristics{
		Arch:             ptrOrZero(cloudInstance.Architecture()),
		Mem:              ptrOrZero(cloudInstance.Memory()),
		RootDisk:         ptrOrZero(cloudInstance.RootDisk()),
		RootDiskSource:   ptrOrZero(cloudInstance.RootDiskSource()),
		CpuCores:         ptrOrZero(cloudInstance.CpuCores()),
		CpuPower:         ptrOrZero(cloudInstance.CpuPower()),
		AvailabilityZone: ptrOrZero(cloudInstance.AvailabilityZone()),
		VirtType:         ptrOrZero(cloudInstance.VirtType()),
	}
	if tags := cloudInstance.Tags(); len(tags) != 0 {
		hardwareCharacteristics.Tags = &tags
	}
	if err := i.service.SetMachineCloudInstance(
		ctx,
		machineUUID,
		instance.Id(cloudInstance.InstanceId()),
		cloudInstance.DisplayName(),
		m.Nonce(),
		hardwareCharacteristics,
	); err != nil {
		return errors.Errorf("importing machine cloud instance %q: %w", m.Id(), err)
	}

	return nil
}

func encodeBaseFromMachine(m description.Machine) (deployment.OSType, string, error) {
	b, err := base.ParseBaseFromString(m.Base())
	if err != nil {
		return -1, "", err
	}
	osType, err := encodeOSType(b)
	if err != nil {
		return -1, "", err
	}
	return osType, b.Channel.String(), nil
}

func encodeOSType(b base.Base) (deployment.OSType, error) {
	switch b.OS {
	case base.UbuntuOS:
		return deployment.Ubuntu, nil
	default:
		return -1, errors.Errorf("unknown os type %q", b)
	}
}

func encodeArchitectureFromMachine(constraints description.Constraints, instance description.CloudInstance) (deployment.Architecture, error) {
	// Look first at the constraints.
	if constraints != nil {
		arch, err := encodeArchitecture(constraints.Architecture())
		if err != nil {
			return -1, err
		} else if arch >= 0 {
			return arch, nil
		}
	}

	// Next look at the instance information.
	if instance != nil {
		arch, err := encodeArchitecture(instance.Architecture())
		if err != nil {
			return -1, err
		} else if arch >= 0 {
			return arch, nil
		}
	}

	// If there is no constraint or instance architecture, then we can fall
	// back to the default architecture.
	return encodeArchitecture(arch.DefaultArchitecture)
}

func encodeArchitecture(a string) (deployment.Architecture, error) {
	switch a {
	case arch.AMD64:
		return architecture.AMD64, nil
	case arch.ARM64:
		return architecture.ARM64, nil
	case arch.PPC64EL:
		return architecture.PPC64EL, nil
	case arch.S390X:
		return architecture.S390X, nil
	case arch.RISCV64:
		return architecture.RISCV64, nil
	case "":
		return -1, nil
	default:
		return -1, errors.Errorf("unknown architecture %q", a)
	}
}

func ptrOrZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
