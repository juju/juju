// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/charmrepo/v5"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/annotations"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/api/modelconfig"
	jujucharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/resource/resourceadapters"
)

// NewDeployCommandForTest returns a command to deploy applications intended to be used only in tests.
func NewDeployCommandForTest(fakeApi *fakeDeployAPI) modelcmd.ModelCommand {
	deployCmd := &DeployCommand{
		NewAPIRoot: func() (DeployAPI, error) {
			return fakeApi, nil
		},
		DeployResources: func(
			applicationID string,
			chID jujucharmstore.CharmID,
			csMac *macaroon.Macaroon,
			filesAndRevisions map[string]string,
			resources map[string]charmresource.Meta,
			conn base.APICallCloser,
		) (ids map[string]string, err error) {
			return nil, nil
		},
		NewCharmRepo: func() (*charmStoreAdaptor, error) {
			return fakeApi.charmStoreAdaptor, nil
		},
		clock: jujuclock.WallClock,
	}
	if fakeApi == nil {
		deployCmd.NewAPIRoot = func() (DeployAPI, error) {
			apiRoot, err := deployCmd.ModelCommandBase.NewAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			mURL, err := deployCmd.getMeteringAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}

			return &deployAPIAdapter{
				Connection:        apiRoot,
				apiClient:         &apiClient{Client: apiRoot.Client()},
				charmsClient:      &charmsClient{Client: apicharms.NewClient(apiRoot)},
				applicationClient: &applicationClient{Client: application.NewClient(apiRoot)},
				modelConfigClient: &modelConfigClient{Client: modelconfig.NewClient(apiRoot)},
				annotationsClient: &annotationsClient{Client: annotations.NewClient(apiRoot)},
				plansClient:       &plansClient{planURL: mURL},
			}, nil
		}
		deployCmd.NewCharmRepo = func() (*charmStoreAdaptor, error) {
			controllerAPIRoot, err := deployCmd.NewControllerAPIRoot()
			if err != nil {
				return nil, errors.Trace(err)
			}
			bakeryClient, err := deployCmd.BakeryClient()
			if err != nil {
				return nil, errors.Trace(err)
			}
			csURL, err := getCharmStoreAPIURL(controllerAPIRoot)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cstoreClient := newCharmStoreClient(bakeryClient, csURL).WithChannel(deployCmd.Channel)
			return &charmStoreAdaptor{
				macaroonGetter:     cstoreClient,
				charmrepoForDeploy: charmrepo.NewCharmStoreFromClient(cstoreClient),
			}, nil
		}
	}
	return modelcmd.Wrap(deployCmd)
}

func NewUpgradeCharmCommandForTest(
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	deployResources resourceadapters.DeployResourcesFunc,
	resolveCharm ResolveCharmFunc,
	newCharmStore NewCharmStoreFunc,
	newCharmAdder NewCharmAdderFunc,
	newCharmClient func(base.APICallCloser) CharmClient,
	newCharmUpgradeClient func(base.APICallCloser) CharmAPIClient,
	newResourceLister func(base.APICallCloser) (ResourceLister, error),
	charmStoreURLGetter func(base.APICallCloser) (string, error),
	newSpacesClient func(base.APICallCloser) SpacesAPI,
) cmd.Command {
	cmd := &upgradeCharmCommand{
		DeployResources:       deployResources,
		ResolveCharm:          resolveCharm,
		NewCharmAdder:         newCharmAdder,
		NewCharmClient:        newCharmClient,
		NewCharmUpgradeClient: newCharmUpgradeClient,
		NewResourceLister:     newResourceLister,
		CharmStoreURLGetter:   charmStoreURLGetter,
		NewSpacesClient:       newSpacesClient,
		NewCharmStore:         newCharmStore,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpen(apiOpen)
	return modelcmd.Wrap(cmd)
}

func NewUpgradeCharmCommandForStateTest(
	newCharmStore NewCharmStoreFunc,
	newCharmAdder NewCharmAdderFunc,
	newCharmClient func(base.APICallCloser) CharmClient,
	deployResources resourceadapters.DeployResourcesFunc,
	newCharmAPIClient func(conn base.APICallCloser) CharmAPIClient,
) cmd.Command {
	cmd := newUpgradeCharmCommand()
	cmd.NewCharmStore = newCharmStore
	cmd.NewCharmAdder = newCharmAdder
	cmd.NewCharmClient = newCharmClient
	if newCharmAPIClient != nil {
		cmd.NewCharmUpgradeClient = newCharmAPIClient
	}
	cmd.DeployResources = deployResources
	return modelcmd.Wrap(cmd)
}

func NewBindCommandForTest(
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	newApplicationClient func(base.APICallCloser) ApplicationBindClient,
	newSpacesClient func(base.APICallCloser) SpacesAPI,
) cmd.Command {
	cmd := &bindCommand{
		NewApplicationClient: newApplicationClient,
		NewSpacesClient:      newSpacesClient,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpen(apiOpen)
	return modelcmd.Wrap(cmd)
}

// NewResolvedCommandForTest returns a ResolvedCommand with the api provided as specified.
func NewResolvedCommandForTest(applicationResolveAPI applicationResolveAPI, clientAPI clientAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &resolvedCommand{applicationResolveAPI: applicationResolveAPI, clientAPI: clientAPI}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api applicationAddUnitAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &addUnitCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified as well as overrides the refresh function.
func NewAddUnitCommandForTestWithRefresh(api applicationAddUnitAPI, store jujuclient.ClientStore, refreshFunc func(jujuclient.ClientStore, string) error) modelcmd.ModelCommand {
	cmd := &addUnitCommand{api: api}
	cmd.SetClientStore(store)
	cmd.SetModelRefresh(refreshFunc)
	return modelcmd.Wrap(cmd)
}

// NewRemoveUnitCommandForTest returns a RemoveUnitCommand with the api provided as specified.
func NewRemoveUnitCommandForTest(api RemoveApplicationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeUnitCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

type removeAPIFunc func() (RemoveApplicationAPI, int, error)

// NewRemoveApplicationCommandForTest returns a RemoveApplicationCommand.
func NewRemoveApplicationCommandForTest(f removeAPIFunc, store jujuclient.ClientStore) modelcmd.ModelCommand {
	c := &removeApplicationCommand{}
	c.newAPIFunc = f
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

// NewAddRelationCommandForTest returns an AddRelationCommand with the api provided as specified.
func NewAddRelationCommandForTest(addAPI applicationAddRelationAPI, consumeAPI applicationConsumeDetailsAPI) modelcmd.ModelCommand {
	cmd := &addRelationCommand{addRelationAPI: addAPI, consumeDetailsAPI: consumeAPI}
	return modelcmd.Wrap(cmd)
}

// NewRemoveRelationCommandForTest returns an RemoveRelationCommand with the api provided as specified.
func NewRemoveRelationCommandForTest(api ApplicationDestroyRelationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeRelationCommand{newAPIFunc: func() (ApplicationDestroyRelationAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewConsumeCommandForTest returns a ConsumeCommand with the specified api.
func NewConsumeCommandForTest(
	store jujuclient.ClientStore,
	sourceAPI applicationConsumeDetailsAPI,
	targetAPI applicationConsumeAPI,
) cmd.Command {
	c := &consumeCommand{sourceAPI: sourceAPI, targetAPI: targetAPI}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

// NewSetSeriesCommandForTest returns a SetSeriesCommand with the specified api.
func NewSetSeriesCommandForTest(
	seriesAPI setSeriesAPI,
	store jujuclient.ClientStore,
) modelcmd.ModelCommand {
	cmd := &setSeriesCommand{
		setSeriesClient: seriesAPI,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewSuspendRelationCommandForTest returns a SuspendRelationCommand with the api provided as specified.
func NewSuspendRelationCommandForTest(api SetRelationSuspendedAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &suspendRelationCommand{newAPIFunc: func() (SetRelationSuspendedAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewResumeRelationCommandForTest returns a ResumeRelationCommand with the api provided as specified.
func NewResumeRelationCommandForTest(api SetRelationSuspendedAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &resumeRelationCommand{newAPIFunc: func() (SetRelationSuspendedAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewRemoveSaasCommandForTest returns a RemoveSaasCommand with the api provided as specified.
func NewRemoveSaasCommandForTest(api RemoveSaasAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeSaasCommand{newAPIFunc: func() (RemoveSaasAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewScaleCommandForTest returns a ScaleCommand with the api provided as specified.
func NewScaleCommandForTest(api scaleApplicationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &scaleApplicationCommand{newAPIFunc: func() (scaleApplicationAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewBundleDiffCommandForTest(api base.APICallCloser, charmStore BundleResolver, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &bundleDiffCommand{
		_apiRoot:    api,
		_charmStore: charmStore,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewShowCommandForTest(api ApplicationsInfoAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showApplicationCommand{newAPIFunc: func() (ApplicationsInfoAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// RepoSuiteBaseSuite allows the patching of the supported juju suite for
// each test.
type RepoSuiteBaseSuite struct {
	jujutesting.RepoSuite
}

func (s *RepoSuiteBaseSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.PatchValue(&supportedJujuSeries, func(time.Time, string, string) (set.Strings, error) {
		return defaultSupportedJujuSeries, nil
	})
}
