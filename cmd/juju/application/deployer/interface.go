// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	apiparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/jujuclient"
)

// DeployStepAPI represents a API required for deploying using the step
// deployment code.
type DeployStepAPI interface {
	MeteredDeployAPI
}

// DeployStep is an action that needs to be taken during charm deployment.
type DeployStep interface {
	// SetFlags sets flags necessary for the deploy step.
	SetFlags(*gnuflag.FlagSet)

	// SetPlanURL sets the plan URL prefix.
	SetPlanURL(planURL string)

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
}

// OfferAPI represents the methods of the API the deploy command needs
// for creating offers.
type OfferAPI interface {
	Offer(modelUUID, application string, endpoints []string, offerName, descr string) ([]apiparams.ErrorResult, error)
	GrantOffer(user, access string, offerURLs ...string) error
}

// ConsumeDetails
type ConsumeDetails interface {
	GetConsumeDetails(url string) (apiparams.ConsumeOfferDetails, error)
	Close() error
}

var supportedJujuSeries = series.WorkloadSeries

type DeployerAPI interface {
	// Needed for BestFacadeVersion and for the DeployResourcesFunc.
	base.APICallCloser

	ApplicationAPI
	store.CharmAdder
	DeployStepAPI
	CharmDeployAPI
	ModelAPI
	OfferAPI

	Deploy(application.DeployArgs) error
	Status(patterns []string) (*apiparams.FullStatus, error)
	WatchAll() (api.AllWatch, error)
}

type ApplicationAPI interface {
	AddMachines(machineParams []apiparams.AddMachineParams) ([]apiparams.AddMachinesResult, error)
	AddRelation(endpoints, viaCIDRs []string) (*apiparams.AddRelationResults, error)
	AddUnits(application.AddUnitsParams) ([]string, error)
	Expose(application string) error
	GetAnnotations(tags []string) ([]apiparams.AnnotationsGetResult, error)
	GetConfig(branchName string, appNames ...string) ([]map[string]interface{}, error)
	GetConstraints(appNames ...string) ([]constraints.Value, error)
	SetAnnotation(annotations map[string]map[string]string) ([]apiparams.ErrorResult, error)
	SetCharm(string, application.SetCharmConfig) error
	SetConstraints(application string, constraints constraints.Value) error
	Update(apiparams.ApplicationUpdate) error
	ScaleApplication(application.ScaleApplicationParams) (apiparams.ScaleApplicationResult, error)
	Consume(arg crossmodel.ConsumeApplicationArgs) (string, error)
}

// BundleResolver defines what we need from a charm store to resolve a
// bundle and read the bundle data.
type BundleResolver interface {
	store.URLResolver
	GetBundle(*charm.URL, string) (charm.Bundle, error)
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
