// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"strings"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/storage"
)

type deployBundle struct {
	model ModelCommand
	steps []DeployStep

	dryRun bool
	force  bool
	trust  bool

	bundleDataSource  charm.BundleDataSource
	bundleDir         string
	bundleURL         *charm.URL
	bundleOverlayFile []string
	channel           csparams.Channel

	bundleResolver       BundleResolver
	authorizer           store.MacaroonGetter
	newConsumeDetailsAPI func(url *charm.OfferURL) (ConsumeDetails, error)
	deployResources      resourceadapters.DeployResourcesFunc

	useExistingMachines bool
	bundleMachines      map[string]string
	bundleStorage       map[string]map[string]storage.Constraints
	bundleDevices       map[string]map[string]devices.Constraints

	targetModelName string
	targetModelUUID string
	controllerName  string
	accountUser     string
}

// deploy is the business logic of deploying a bundle after
// it's been prepared.
func (d *deployBundle) deploy(
	ctx *cmd.Context,
	deployAPI DeployerAPI,
	cstore *store.CharmStoreAdaptor,
) (rErr error) {
	d.bundleResolver = cstore
	d.authorizer = cstore.MacaroonGetter
	bakeryClient, err := d.model.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}

	var ok bool
	if d.targetModelUUID, ok = deployAPI.ModelUUID(); !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	if d.targetModelName, _, err = d.model.ModelDetails(); err != nil {
		return errors.Annotatef(err, "could not retrieve model name")
	}

	controllerName, err := d.model.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	d.controllerName = controllerName
	accountDetails, err := d.model.CurrentAccountDetails()
	if err != nil {
		return errors.Trace(err)
	}
	d.accountUser = accountDetails.User

	// Compose bundle to be deployed and check its validity before running
	// any pre/post checks.
	var bundleData *charm.BundleData
	if bundleData, err = bundle.ComposeAndVerifyBundle(d.bundleDataSource, d.bundleOverlayFile); err != nil {
		return errors.Annotatef(err, "cannot deploy bundle")
	}
	d.bundleDir = d.bundleDataSource.BasePath()

	// Short-circuit trust checks if the operator specifies '--force'
	if !d.trust {
		if tl := appsRequiringTrust(bundleData.Applications); len(tl) != 0 && !d.force {
			return errors.Errorf(`Bundle cannot be deployed without trusting applications with your cloud credentials.
Please repeat the deploy command with the --trust argument if you consent to trust the following application(s):
  - %s`, strings.Join(tl, "\n  - "))
		}
	}

	for application, applicationSpec := range bundleData.Applications {
		if applicationSpec.Plan != "" {
			for _, step := range d.steps {
				s := step
				charmURL, err := charm.ParseURL(applicationSpec.Charm)
				if err != nil {
					return errors.Trace(err)
				}

				deployInfo := DeploymentInfo{
					CharmID:         charmstore.CharmID{URL: charmURL},
					ApplicationName: application,
					ApplicationPlan: applicationSpec.Plan,
					ModelUUID:       d.targetModelUUID,
					Force:           d.force,
				}

				err = s.RunPre(deployAPI, bakeryClient, ctx, deployInfo)
				if err != nil {
					return errors.Trace(err)
				}

				defer func() {
					err = errors.Trace(s.RunPost(deployAPI, bakeryClient, ctx, deployInfo, rErr))
					if err != nil {
						rErr = err
					}
				}()
			}
		}
	}
	spec := d.makeBundleDeploySpec(ctx, deployAPI)

	// TODO(ericsnow) Do something with the CS macaroons that were returned?
	// Deploying bundles does not allow the use force, it's expected that the
	// bundle is correct and therefore the charms are also.
	if _, err := bundleDeploy(bundleData, spec); err != nil {
		return errors.Annotate(err, "cannot deploy bundle")
	}
	return nil
}

func (d *deployBundle) makeBundleDeploySpec(ctx *cmd.Context, apiRoot DeployerAPI) bundleDeploySpec {
	// set the consumer details API factory method on the spec, so it makes it
	// possible to communicate with other controllers, that are found within
	// the local cache.
	// If no controller is found within the local cache, an error will be raised
	// which should ask the user to login.
	getConsumeDetails := func(url *charm.OfferURL) (ConsumeDetails, error) {
		// Ensure that we have a url source when querying the controller.
		if url.Source == "" {
			url.Source = d.controllerName
		}
		return d.newConsumeDetailsAPI(url)
	}

	return bundleDeploySpec{
		ctx:                  ctx,
		filesystem:           d.model.Filesystem(),
		dryRun:               d.dryRun,
		force:                d.force,
		trust:                d.trust,
		bundleDataSource:     d.bundleDataSource,
		bundleDir:            d.bundleDir,
		bundleURL:            d.bundleURL,
		bundleOverlayFile:    d.bundleOverlayFile,
		channel:              d.channel,
		deployAPI:            apiRoot,
		bundleResolver:       d.bundleResolver,
		authorizer:           d.authorizer,
		getConsumeDetailsAPI: getConsumeDetails,
		deployResources:      d.deployResources,
		useExistingMachines:  d.useExistingMachines,
		bundleMachines:       d.bundleMachines,
		bundleStorage:        d.bundleStorage,
		bundleDevices:        d.bundleDevices,
		targetModelName:      d.targetModelName,
		targetModelUUID:      d.targetModelUUID,
		controllerName:       d.controllerName,
		accountUser:          d.accountUser,
	}
}

type localBundle struct {
	deployBundle
}

// PrepareAndDeploy deploys a local bundle, no further preparation is needed.
func (d *localBundle) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, cstore *store.CharmStoreAdaptor) error {
	return d.deploy(ctx, deployAPI, cstore)
}

type charmstoreBundle struct {
	deployBundle
}

// PrepareAndDeploy deploys a local bundle, no further preparation is needed.
func (d *charmstoreBundle) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, cstore *store.CharmStoreAdaptor) error {
	ctx.Infof("Located bundle %q", d.bundleURL)
	return d.deploy(ctx, deployAPI, cstore)
}
