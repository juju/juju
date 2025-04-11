// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"

	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/cloud/state"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	credstate "github.com/juju/juju/domain/credential/state"
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

	getPreferredSimpleStreams PreferredSimpleStreamsFunc
	agentBinaryFilter         AgentBinaryFinder
}

// PreferredSimpleStreamsFunc is a function that returns the preferred streams
// for the given version and stream.
type PreferredSimpleStreamsFunc func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string

// AgentBinaryFinder is a function that filters agent binaries based on the
// given parameters. It returns a list of agent binaries that match the filter
// criteria.
type AgentBinaryFinder func(
	ctx context.Context,
	ss envtools.SimplestreamsFetcher,
	env environs.BootstrapEnviron,
	majorVersion,
	minorVersion int,
	streams []string,
	filter coretools.Filter,
) (coretools.List, error)

// ProviderWithSecretToken is a subset of caas broker.
type ProviderWithSecretToken interface {
	GetSecretToken(ctx context.Context, name string) (string, error)
}

// ProviderForAgentBinaryFinder is a subset of cloud provider.
type ProviderForAgentBinaryFinder interface {
	environs.BootstrapEnviron
}

// NewStubService returns a new StubService.
func NewStubService(
	modelUUID coremodel.UUID,
	controllerFactory database.TxnRunnerFactory,
	modelFactory database.TxnRunnerFactory,
	providerWithSecretToken providertracker.ProviderGetter[ProviderWithSecretToken],
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	getPreferredSimpleStreams PreferredSimpleStreamsFunc,
	agentBinaryFilter AgentBinaryFinder,
) *StubService {
	return &StubService{
		modelUUID:                    modelUUID,
		controllerState:              domain.NewStateBase(controllerFactory),
		modelState:                   domain.NewStateBase(modelFactory),
		providerWithSecretToken:      providerWithSecretToken,
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		getPreferredSimpleStreams:    getPreferredSimpleStreams,
		agentBinaryFilter:            agentBinaryFilter,
	}
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

// EnvironAgentBinariesFinderFunc is a function that can be used to find agent binaries
// from the simplestreams data sources.
type EnvironAgentBinariesFinderFunc func(
	ctx context.Context,
	major,
	minor int,
	version semversion.Number,
	requestedStream string,
	filter coretools.Filter,
) (coretools.List, error)

// GetEnvironAgentBinariesFinder returns a function that can be used to find
// agent binaries from the simplestreams data sources.
func (s *StubService) GetEnvironAgentBinariesFinder() EnvironAgentBinariesFinderFunc {
	return func(
		ctx context.Context,
		major,
		minor int,
		version semversion.Number,
		requestedStream string,
		filter coretools.Filter,
	) (coretools.List, error) {
		provider, err := s.providerForAgentBinaryFinder(ctx)
		if errors.Is(err, coreerrors.NotSupported) {
			return nil, errors.Errorf("getting provider for agent binary finder %w", coreerrors.NotSupported)
		}
		if err != nil {
			return nil, errors.Capture(err)
		}
		cfg := provider.Config()
		if requestedStream == "" {
			requestedStream = cfg.AgentStream()
		}

		streams := s.getPreferredSimpleStreams(&version, cfg.Development(), requestedStream)
		ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
		return s.agentBinaryFilter(ctx, ssFetcher, provider, major, minor, streams, filter)
	}
}
