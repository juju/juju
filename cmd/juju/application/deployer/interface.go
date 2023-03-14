// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v10"
	charmresource "github.com/juju/charm/v10/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/jujuclient"
	apiparams "github.com/juju/juju/rpc/params"
)

// DeployerFactory contains a method to get a deployer.
type DeployerFactory interface {
	GetDeployer(DeployerConfig, ModelConfigGetter, Resolver) (Deployer, error)
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

// DeployStepAPI represents a API required for deploying using the step
// deployment code.
type DeployStepAPI interface {
	MeteredDeployAPI
}

// DeployStep is an action that needs to be taken during charm deployment.
type DeployStep interface {
	// SetFlags sets flags necessary for the deploy step.
	SetFlags(*gnuflag.FlagSet)

	// RunPre runs before the call is made to add the charm to the environment.
	RunPre(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo) error

	// RunPost runs after the call is made to add the charm to the environment.
	// The error parameter is used to notify the step of a previously occurred error.
	RunPost(DeployStepAPI, *httpbakery.Client, *cmd.Context, DeploymentInfo, error) error
}

type ModelAPI interface {
	ModelUUID() (string, bool)
	ModelGet() (map[string]interface{}, error)
	Sequences() (map[string]int, error)
	GetModelConstraints() (constraints.Value, error)
}

// MeteredDeployAPI represents the methods of the API the deploy
// command needs for metered charms.
type MeteredDeployAPI interface {
	IsMetered(charmURL string) (bool, error)
	SetMetricCredentials(application string, credentials []byte) error
}

// CharmDeployAPI represents the methods of the API the deploy
// command needs for charms.
type CharmDeployAPI interface {
	CharmInfo(string) (*apicharms.CharmInfo, error)
	ListCharmResources(curl *charm.URL, origin commoncharm.Origin) ([]charmresource.Resource, error)
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
var SupportedJujuSeries = series.WorkloadSeries

type DeployerAPI interface {
	// APICallCloser is needed for the DeployResourcesFunc.
	base.APICallCloser

	ApplicationAPI
	store.CharmAdder
	DeployStepAPI
	CharmDeployAPI
	ModelAPI
	OfferAPI

	ListSpaces() ([]apiparams.Space, error)
	Deploy(application.DeployArgs) error
	Status(patterns []string) (*apiparams.FullStatus, error)
	WatchAll() (api.AllWatch, error)
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

// Bundle is a local version of the charm.Bundle interface, for test
// with the Resolver interface.
type Bundle interface {
	// Data returns the contents of the bundle's bundle.yaml file.
	Data() *charm.BundleData // yes
	// ReadMe returns the contents of the bundle's README.md file.
	ReadMe() string
	// ContainsOverlays returns true if the bundle contains any overlays.
	ContainsOverlays() bool
}

// Resolver defines what we need to resolve a charm or bundle and
// read the bundle data.
type Resolver interface {
	GetBundle(*charm.URL, commoncharm.Origin, string) (charm.Bundle, error)
	ResolveBundleURL(*charm.URL, commoncharm.Origin) (*charm.URL, commoncharm.Origin, error)
	ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []string, error)
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
	// ReadCharm reads a given charm from the filesystem.
	ReadCharm(string) (charm.Charm, error)
}

// DeployConfigFlag defines methods required for charm config when deploying a charm.
type DeployConfigFlag interface {
	// AbsoluteFileNames returns the absolute path of any file names specified.
	AbsoluteFileNames(ctx *cmd.Context) ([]string, error)

	// ReadConfigPairs returns just the k=v attributes
	ReadConfigPairs(ctx *cmd.Context) (map[string]interface{}, error)
}
