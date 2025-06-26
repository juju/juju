// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/client"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/jujuclient"
	apiparams "github.com/juju/juju/rpc/params"
)

// DeployerFactory contains a method to get a deployer.
type DeployerFactory interface {
	GetDeployer(context.Context, DeployerConfig, CharmDeployAPI, Resolver) (Deployer, error)
}

// Deployer defines the functionality of a deployer returned by the
// factory.
type Deployer interface {
	// PrepareAndDeploy finishes preparing to deploy a charm or bundle,
	// then deploys it.  This is done as one step to accommodate the
	// call being wrapped by block.ProcessBlockedError.
	PrepareAndDeploy(*cmd.Context, DeployerAPI, Resolver) error

	// String returns a string description of the deployer.
	String() string
}

type ModelAPI interface {
	ModelUUID() (string, bool)
	ModelGet(ctx context.Context) (map[string]interface{}, error)
	Sequences(ctx context.Context) (map[string]int, error)
	GetModelConstraints(ctx context.Context) (constraints.Value, error)
}

// CharmDeployAPI represents the methods of the API the deploy
// command needs for charms.
type CharmDeployAPI interface {
	ModelConfigGetter
	CharmInfo(context.Context, string) (*apicharms.CharmInfo, error)
}

// OfferAPI represents the methods of the API the deploy command needs
// for creating offers.
type OfferAPI interface {
	Offer(ctx context.Context, modelUUID, application string, endpoints []string, qualifier, offerName, descr string) ([]apiparams.ErrorResult, error)
	GrantOffer(ctx context.Context, user, access string, offerURLs ...string) error
}

// ConsumeDetails represents methods needed to consume an offer.
type ConsumeDetails interface {
	GetConsumeDetails(ctx context.Context, url string) (apiparams.ConsumeOfferDetails, error)
	Close() error
}

// For testing.
// TODO: unexport it if we don't need to patch it anymore.
var SupportedJujuBases = corebase.WorkloadBases

type DeployerAPI interface {
	// APICallCloser is needed for the DeployResourcesFunc.
	base.APICallCloser

	ApplicationAPI
	store.CharmAdder
	CharmDeployAPI
	ModelAPI
	OfferAPI

	ListSpaces(ctx context.Context) ([]apiparams.Space, error)
	Deploy(context.Context, application.DeployArgs) error
	Status(context.Context, *client.StatusArgs) (*apiparams.FullStatus, error)
	ListCharmResources(ctx context.Context, curl string, origin commoncharm.Origin) ([]charmresource.Resource, error)
}

type ApplicationAPI interface {
	AddMachines(ctx context.Context, machineParams []apiparams.AddMachineParams) ([]apiparams.AddMachinesResult, error)
	AddRelation(ctx context.Context, endpoints, viaCIDRs []string) (*apiparams.AddRelationResults, error)
	AddUnits(context.Context, application.AddUnitsParams) ([]string, error)
	Expose(ctx context.Context, application string, exposedEndpoints map[string]apiparams.ExposedEndpoint) error

	GetAnnotations(ctx context.Context, tags []string) ([]apiparams.AnnotationsGetResult, error)
	SetAnnotation(ctx context.Context, annotations map[string]map[string]string) ([]apiparams.ErrorResult, error)

	GetCharmURLOrigin(context.Context, string) (*charm.URL, commoncharm.Origin, error)
	SetCharm(context.Context, application.SetCharmConfig) error

	GetConfig(ctx context.Context, appNames ...string) ([]map[string]interface{}, error)
	SetConfig(ctx context.Context, application, configYAML string, config map[string]string) error

	GetConstraints(ctx context.Context, appNames ...string) ([]constraints.Value, error)
	SetConstraints(ctx context.Context, application string, constraints constraints.Value) error

	ScaleApplication(context.Context, application.ScaleApplicationParams) (apiparams.ScaleApplicationResult, error)
	Consume(ctx context.Context, arg crossmodel.ConsumeApplicationArgs) (string, error)

	ApplicationsInfo(context.Context, []names.ApplicationTag) ([]apiparams.ApplicationInfoResult, error)

	DeployFromRepository(ctx context.Context, arg application.DeployFromRepositoryArg) (application.DeployInfo, []application.PendingResourceUpload, []error)
}

// Resolver defines what we need to resolve a charm or bundle and
// read the bundle data.
type Resolver interface {
	GetBundle(context.Context, *charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
	ResolveBundleURL(context.Context, *charm.URL, commoncharm.Origin) (*charm.URL, commoncharm.Origin, error)
	ResolveCharm(ctx context.Context, url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error)
}

type ModelConfigGetter interface {
	ModelGet(ctx context.Context) (map[string]interface{}, error)
}

type ModelCommand interface {
	// BakeryClient returns a macaroon bakery client that
	// uses the same HTTP client returned by HTTPClient.
	BakeryClient() (*httpbakery.Client, error)

	// ControllerName returns the name of the controller that contains
	// the model returned by ModelName().
	ControllerName() (string, error)

	// CurrentAccountDetails returns details of the account associated with
	// the current controller.
	CurrentAccountDetails() (*jujuclient.AccountDetails, error)

	// ModelDetails returns details from the file store for the model indicated by
	// the currently set controller name and model identifier.
	ModelDetails(ctx context.Context) (string, *jujuclient.ModelDetails, error)

	// ModelType returns the type of the model.
	ModelType(ctx context.Context) (model.ModelType, error)

	// Filesystem returns an instance that provides access to
	// the filesystem, either delegating to calling os functions
	// or functions which always return an error.
	Filesystem() modelcmd.Filesystem
}

// CharmReader aims to read a charm from the filesystem.
type CharmReader interface {
	// NewCharmAtPath returns the charm represented by this path,
	// and a URL that describes it.
	NewCharmAtPath(string) (charm.Charm, *charm.URL, error)
}

// DeployConfigFlag defines methods required for charm config when deploying a charm.
type DeployConfigFlag interface {
	// AbsoluteFileNames returns the absolute path of any file names specified.
	AbsoluteFileNames(ctx *cmd.Context) ([]string, error)

	// ReadConfigPairs returns just the k=v attributes
	ReadConfigPairs(ctx *cmd.Context) (map[string]interface{}, error)
}
