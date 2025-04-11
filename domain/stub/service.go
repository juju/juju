// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"

	"github.com/canonical/sqlair"

	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/cloud/state"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	credstate "github.com/juju/juju/domain/credential/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// StubService is a special service that collects temporary methods required for
// wiring together domains which not completely implemented or wired up.
//
// Given the temporary nature of this service, we have not implemented the full
// service/state layer indirection. Instead, the service directly uses a transaction
// runner.
//
// Deprecated: All methods here should be thrown away as soon as we're done with
// then.
type StubService struct {
	modelUUID       coremodel.UUID
	modelState      *domain.StateBase
	controllerState *domain.StateBase

	providerWithSecretToken      providertracker.ProviderGetter[ProviderWithSecretToken]
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder]

	agentBinaryFilter AgentBinaryFilter
}

// ProviderWithSecretToken is a subset of caas broker.
type ProviderWithSecretToken interface {
	GetSecretToken(ctx context.Context, name string) (string, error)
}

// ProviderForAgentBinaryFinder is a subset of cloud provider.
type ProviderForAgentBinaryFinder interface {
	environs.BootstrapEnviron
}

// AgentBinaryFilter is a function that filters agent binaries based on the
// given parameters. It returns a list of agent binaries that match the filter
// criteria.
type AgentBinaryFilter func(
	ctx context.Context, ss envtools.SimplestreamsFetcher, env environs.BootstrapEnviron,
	majorVersion, minorVersion int, streams []string, filter coretools.Filter,
) (_ coretools.List, err error)

// NewStubService returns a new StubService.
func NewStubService(
	modelUUID coremodel.UUID,
	controllerFactory database.TxnRunnerFactory,
	modelFactory database.TxnRunnerFactory,
	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken],
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	agentBinaryFilter AgentBinaryFilter,
) *StubService {
	return &StubService{
		modelUUID:                    modelUUID,
		controllerState:              domain.NewStateBase(controllerFactory),
		modelState:                   domain.NewStateBase(modelFactory),
		providerWithSecretToken:      providerWithSecretToken,
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		agentBinaryFilter:            agentBinaryFilter,
	}
}

// AssignUnitsToMachines assigns the given units to the given machines but setting
// unit net node to the machine net node.
//
// Deprecated: AssignUnitsToMachines will become redundant once the machine and
// application domains have become fully implemented.
func (s *StubService) AssignUnitsToMachines(ctx context.Context, groupedUnitsByMachine map[string][]unit.Name) error {
	db, err := s.modelState.DB()
	if err != nil {
		return errors.Capture(err)
	}

	getNetNodeQuery, err := s.modelState.Prepare(`
SELECT &netNodeUUID.*
FROM machine
WHERE name = $machine.name
`, netNodeUUID{}, machine{})
	if err != nil {
		return errors.Errorf("preparing machine query: %v", err)
	}

	verifyUnitsExistQuery, err := s.modelState.Prepare(`
SELECT COUNT(*) AS &count.count
FROM unit
WHERE name IN ($units[:])
`, count{}, units{})
	if err != nil {
		return errors.Errorf("preparing verify units exist query: %v", err)
	}

	setUnitsNetNodeQuery, err := s.modelState.Prepare(`
UPDATE unit
SET net_node_uuid = $netNodeUUID.net_node_uuid
WHERE name IN ($units[:])
`, netNodeUUID{}, units{})
	if err != nil {
		return errors.Errorf("preparing set units query: %v", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for machine, units := range encodeGroupedUnitsByMachine(groupedUnitsByMachine) {
			var netNodeUUID netNodeUUID
			err = tx.Query(ctx, getNetNodeQuery, machine).Get(&netNodeUUID)
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("%w: %v", machineerrors.MachineNotFound, machine.MachineName)
			}
			if err != nil {
				return errors.Errorf("getting machine net node: %v", err)
			}

			var count count
			err = tx.Query(ctx, verifyUnitsExistQuery, units).Get(&count)
			if err != nil {
				return errors.Errorf("verifying units exist: %v", err)
			}
			if count.Count != len(units) {
				return errors.Errorf("not all units found %q", units).
					Add(applicationerrors.UnitNotFound)
			}

			err = tx.Query(ctx, setUnitsNetNodeQuery, netNodeUUID, units).Run()
			if err != nil {
				return errors.Errorf("setting unit: %v", err)
			}
		}
		return nil
	})

	if err != nil {
		return errors.Errorf("assigning units to machines: %w", err)
	}
	return err
}

// CloudSpec returns the cloud spec for the model.
func (s *StubService) CloudSpec(ctx context.Context) (cloudspec.CloudSpec, error) {
	modelSt := modelstate.ModelState{StateBase: s.modelState}
	cloudSt := state.State{StateBase: s.controllerState}
	credSt := credstate.State{StateBase: s.controllerState}

	cloudName, cloudRegion, credKey, err := modelSt.GetModelCloudRegionAndCredential(ctx, s.modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		err = coreerrors.NotFound
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	cld, err := cloudSt.Cloud(ctx, cloudName)
	if errors.Is(err, clouderrors.NotFound) {
		err = coreerrors.NotFound
	}
	if err != nil {
		return cloudspec.CloudSpec{}, errors.Capture(err)
	}

	cred, credErr := credSt.CloudCredential(ctx, credKey)
	if !errors.Is(credErr, credentialerrors.NotFound) && credErr != nil {
		return cloudspec.CloudSpec{}, errors.Capture(credErr)
	}

	var cloudCred *jujucloud.Credential
	if credErr == nil {
		c := jujucloud.NewCredential(jujucloud.AuthType(cred.AuthType), cred.Attributes)
		cloudCred = &c
	}
	return cloudspec.MakeCloudSpec(*cld, cloudRegion, cloudCred)
}

// GetExecSecretToken returns a token that can be used to run exec operations
// on the provider cloud.
func (s *StubService) GetExecSecretToken(ctx context.Context) (string, error) {
	provider, err := s.providerWithSecretToken(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return "", errors.Errorf("getting secret token %w", coreerrors.NotSupported)
	}
	if err != nil {
		return "", errors.Capture(err)
	}

	return provider.GetSecretToken(ctx, k8sprovider.ExecRBACResourceName)
}

type FindAgentBinariesArg struct {
	// MajorVersion will be used to match the major version if non-zero.
	MajorVersion int
	// MinorVersion will be used to match the minor version if non-zero.
	MinorVersion int
	// Number is the version of the agent binary.
	Number semversion.Number
	// Arch is the architecture of the agent binary.
	Arch string
	// OSType will be used to match tools by os type if non-empty.
	OSType string
	// AgentStream will be used to set agent stream to search
	AgentStream string
}

// FindAgentBinariesFromSimpleStreams finds agent binaries from the simplestreams
// data sources.
func (s *StubService) FindAgentBinariesFromSimpleStreams(
	ctx context.Context,
	arg FindAgentBinariesArg,
) (coretools.List, error) {
	provider, err := s.providerForAgentBinaryFinder(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf("getting provider for agent binary finder %w", coreerrors.NotSupported)
	}
	if err != nil {
		return nil, errors.Capture(err)
	}
	cfg := provider.Config()
	requestedStream := cfg.AgentStream()
	if arg.AgentStream != "" {
		requestedStream = arg.AgentStream
	}

	streams := envtools.PreferredStreams(&arg.Number, cfg.Development(), requestedStream)
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	majorVersion := arg.Number.Major
	minorVersion := arg.Number.Minor
	if arg.Number == semversion.Zero {
		majorVersion = arg.MajorVersion
		minorVersion = arg.MinorVersion
	}

	filter := coretools.Filter{
		Number: arg.Number,
		Arch:   arg.Arch,
		OSType: arg.OSType,
	}
	simplestreamsList, err := s.agentBinaryFilter(
		ctx, ss, provider,
		majorVersion, minorVersion,
		streams, filter,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return simplestreamsList, nil
}
