// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/bundle"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/storage"
)

type deployBundle struct {
	model ModelCommand

	dryRun bool
	force  bool
	trust  bool

	bundleDataSource  charm.BundleDataSource
	bundleDir         string
	bundleURL         *charm.URL
	bundleOverlayFile []string
	origin            commoncharm.Origin
	modelConstraints  constraints.Value

	// The default schema to use for charms that do not specify one. The
	// value depends on whether we are deploying to a 2.9+ or an older
	// controller.
	defaultCharmSchema charm.Schema

	resolver             Resolver
	newConsumeDetailsAPI func(ctx context.Context, url *crossmodel.OfferURL) (ConsumeDetails, error)
	deployResources      DeployResourcesFunc
	charmReader          CharmReader

	useExistingMachines bool
	bundleMachines      map[string]string
	bundleStorage       map[string]map[string]storage.Directive
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
	resolver Resolver,
) (rErr error) {
	d.resolver = resolver
	var ok bool
	if d.targetModelUUID, ok = deployAPI.ModelUUID(); !ok {
		return errors.New("API connection is controller-only (should never happen)")
	}

	var err error
	if d.targetModelName, _, err = d.model.ModelDetails(ctx); err != nil {
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

	// Compose bundle to be deployed and check its validity.
	bundleData, unmarshalErrors, err := bundle.ComposeAndVerifyBundle(ctx, d.bundleDataSource, d.bundleOverlayFile)
	if err != nil {
		return errors.Annotatef(err, "cannot deploy bundle")
	}
	d.printDryRunUnmarshalErrors(ctx, unmarshalErrors)

	err = d.checkExplicitBase(bundleData)
	if err != nil {
		return errors.Trace(err)
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

	spec, err := d.makeBundleDeploySpec(ctx, deployAPI)
	if err != nil {
		return errors.Trace(err)
	}

	// Deploying bundles does not allow the use force, it's expected that the
	// bundle is correct and therefore the charms are also.
	if err := bundleDeploy(ctx, d.defaultCharmSchema, bundleData, spec); err != nil {
		return errors.Annotate(err, "cannot deploy bundle")
	}
	return nil
}

// checkExplicitBase returns an error if the image-id constraint is used and
// there is no series explicitly defined by the user.
func (d *deployBundle) checkExplicitBase(bundleData *charm.BundleData) error {
	for _, applicationSpec := range bundleData.Applications {
		// First we check if the app is deployed "to" a machine that
		// has the image-id constraint
		machineHasImageID := false
		for _, to := range applicationSpec.To {

			// This error can be ignored since the bundle has
			// already been validated at this point, and it's not
			// this method's responsibility to validate it.
			placement, _ := charm.ParsePlacement(to)
			// Only check for explicit series when image-id
			// constraint *if* the placement machine is defined
			// in the bundle. In the case of 'new' it does pass
			// bundle validation but it won't be defined since it's
			// a new machine, so don't perform any check in that
			// case.
			if machine, ok := bundleData.Machines[placement.Machine]; ok && machine != nil {
				machineCons, err := constraints.Parse(machine.Constraints)
				if err != nil {
					return errors.Trace(err)
				}
				if machineCons.HasImageID() {
					machineHasImageID = true
					break
				}
			}
		}
		// Then we check if the constraints declared on the app have
		// image-id
		appCons, err := constraints.Parse(applicationSpec.Constraints)
		if err != nil {
			return errors.Trace(err)
		}
		appHasImageID := appCons.HasImageID()
		// Lastly we check if the model constraints have image-id
		modelHasImageID := d.modelConstraints.HasImageID()
		// We check if series are defined when any of the constraints
		// above have image-id
		if (appHasImageID || modelHasImageID || machineHasImageID) &&
			applicationSpec.Base == "" &&
			bundleData.DefaultBase == "" {
			return errors.Forbiddenf("base must be explicitly provided for %q when image-id constraint is used", applicationSpec.Charm)
		}
	}
	return nil
}

func (d *deployBundle) printDryRunUnmarshalErrors(ctx *cmd.Context, unmarshalErrors []error) {
	if !d.dryRun {
		return
	}
	// During a dry run, print any unmarshalling errors from the
	// bundles and overlays
	var msg string
	for _, err := range unmarshalErrors {
		if err == nil {
			continue
		}
		msg = fmt.Sprintf("%s\n %s\n", msg, err)
	}
	if msg == "" {
		return
	}
	ctx.Warningf("These fields%swill be ignored during deployment\n", msg)
}

func (d *deployBundle) makeBundleDeploySpec(ctx *cmd.Context, apiRoot DeployerAPI) (bundleDeploySpec, error) {
	// set the consumer details API factory method on the spec, so it makes it
	// possible to communicate with other controllers, that are found within
	// the local cache.
	// If no controller is found within the local cache, an error will be raised
	// which should ask the user to login.
	getConsumeDetails := func(url *crossmodel.OfferURL) (ConsumeDetails, error) {
		// Ensure that we have a url source when querying the controller.
		if url.Source == "" {
			url.Source = d.controllerName
		}
		return d.newConsumeDetailsAPI(ctx, url)
	}

	knownSpaces, err := apiRoot.ListSpaces(ctx)
	if err != nil && !errors.Is(err, errors.NotSupported) {
		return bundleDeploySpec{}, errors.Trace(err)
	}

	knownSpaceNames := set.NewStrings()
	for _, space := range knownSpaces {
		knownSpaceNames.Add(space.Name)
	}

	modelType, err := d.model.ModelType(ctx)
	if err != nil {
		return bundleDeploySpec{}, errors.Trace(err)
	}
	return bundleDeploySpec{
		ctx:                  ctx,
		filesystem:           d.model.Filesystem(),
		modelType:            modelType,
		dryRun:               d.dryRun,
		force:                d.force,
		trust:                d.trust,
		bundleDataSource:     d.bundleDataSource,
		bundleDir:            d.bundleDir,
		bundleURL:            d.bundleURL,
		bundleOverlayFile:    d.bundleOverlayFile,
		origin:               d.origin,
		modelConstraints:     d.modelConstraints,
		deployAPI:            apiRoot,
		bundleResolver:       d.resolver,
		getConsumeDetailsAPI: getConsumeDetails,
		deployResources:      d.deployResources,
		charmReader:          d.charmReader,
		useExistingMachines:  d.useExistingMachines,
		bundleMachines:       d.bundleMachines,
		bundleStorage:        d.bundleStorage,
		bundleDevices:        d.bundleDevices,
		targetModelName:      d.targetModelName,
		targetModelUUID:      d.targetModelUUID,
		controllerName:       d.controllerName,
		accountUser:          d.accountUser,
		knownSpaceNames:      knownSpaceNames,
	}, nil
}

type localBundle struct {
	deployBundle
}

// String returns a string description of the deployer.
func (d *localBundle) String() string {
	str := fmt.Sprintf("deploy local bundle from: %s", d.bundleDir)
	if isEmptyOrigin(d.origin, commoncharm.OriginLocal) {
		return str
	}
	var channel string
	if ch := d.origin.CharmChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s", str, channel)
}

// PrepareAndDeploy deploys a local bundle, no further preparation is needed.
func (d *localBundle) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver) error {
	return d.deploy(ctx, deployAPI, resolver)
}

type repositoryBundle struct {
	deployBundle
}

// String returns a string description of the deployer.
func (d *repositoryBundle) String() string {
	str := fmt.Sprintf("deploy bundle: %s", d.bundleURL.String())
	if isEmptyOrigin(d.origin, commoncharm.OriginCharmHub) {
		return str
	}
	var revision string
	if d.origin.Revision != nil && *d.origin.Revision != -1 {
		revision = fmt.Sprintf(" with revision %d", *d.origin.Revision)
	}
	var channel string
	if ch := d.origin.CharmChannel().String(); ch != "" {
		channel = fmt.Sprintf(" from channel %s", ch)
	}
	return fmt.Sprintf("%s%s%s", str, revision, channel)
}

// PrepareAndDeploy deploys a local bundle, no further preparation is needed.
func (d *repositoryBundle) PrepareAndDeploy(ctx *cmd.Context, deployAPI DeployerAPI, resolver Resolver) error {
	var revision string
	if d.bundleURL.Revision != -1 {
		revision = fmt.Sprintf(", revision %d", d.bundleURL.Revision)
	}
	ctx.Infof("Located bundle %q in %s%s", d.bundleURL.Name, d.origin.Source, revision)
	return d.deploy(ctx, deployAPI, resolver)
}
