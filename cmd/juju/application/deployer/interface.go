// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v4"
	"github.com/juju/names/v5"

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
	ModelGet() (map[string]interface{}, error)
	Sequences() (map[string]int, error)
	GetModelConstraints() (constraints.Value, error)
}

// CharmDeployAPI represents the methods of the API the deploy
// command needs for charms.
type CharmDeployAPI interface {
	ModelConfigGetter
	CharmInfo(string) (*apicharms.CharmInfo, error)
}

// OfferAPI represents the methods of the API the deploy command needs
// for creating offers.
type OfferAPI interface {
	Offer(modelUUID, application string, endpoints []string, owner, offerName, descr string) ([]apiparams.ErrorResult, error)
	GrantOffer(user, access string, offerURLs ...string) error
}

// ConsumeDetails represents methods needed to consume an offer.
type ConsumeDetails interface {
	GetConsumeDetails(url string) (apiparams.ConsumeOfferDetails, error)
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

	ListSpaces() ([]apiparams.Space, error)
	Deploy(application.DeployArgs) error
	Status(*client.StatusArgs) (*apiparams.FullStatus, error)
	ListCharmResources(curl string, origin commoncharm.Origin) ([]charmresource.Resource, error)
}

type ApplicationAPI interface {
	AddMachines(machineParams []apiparams.AddMachineParams) ([]apiparams.AddMachinesResult, error)
	AddRelation(endpoints, viaCIDRs []string) (*apiparams.AddRelationResults, error)
	AddUnits(application.AddUnitsParams) ([]string, error)
	Expose(application string, exposedEndpoints map[string]apiparams.ExposedEndpoint) error

	GetAnnotations(tags []string) ([]apiparams.AnnotationsGetResult, error)
	SetAnnotation(annotations map[string]map[string]string) ([]apiparams.ErrorResult, error)

	GetCharmURLOrigin(string, string) (*charm.URL, commoncharm.Origin, error)
	SetCharm(string, application.SetCharmConfig) error

	GetConfig(branchName string, appNames ...string) ([]map[string]interface{}, error)
	SetConfig(branchName string, application, configYAML string, config map[string]string) error

	GetConstraints(appNames ...string) ([]constraints.Value, error)
	SetConstraints(application string, constraints constraints.Value) error

	ScaleApplication(application.ScaleApplicationParams) (apiparams.ScaleApplicationResult, error)
	Consume(arg crossmodel.ConsumeApplicationArgs) (string, error)

	ApplicationsInfo([]names.ApplicationTag) ([]apiparams.ApplicationInfoResult, error)

	DeployFromRepository(arg application.DeployFromRepositoryArg) (application.DeployInfo, []application.PendingResourceUpload, []error)
}

// Resolver defines what we need to resolve a charm or bundle and
// read the bundle data.
type Resolver interface {
	GetBundle(context.Context, *charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
	ResolveBundleURL(*charm.URL, commoncharm.Origin) (*charm.URL, commoncharm.Origin, error)
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error)
}

type ModelConfigGetter interface {
	ModelGet() (map[string]interface{}, error)
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
	ModelDetails() (string, *jujuclient.ModelDetails, error)

	// ModelType returns the type of the model.
	ModelType() (model.ModelType, error)

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
