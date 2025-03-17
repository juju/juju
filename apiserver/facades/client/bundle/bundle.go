// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/yaml.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreapplication "github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/version"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/config"
	bundlechanges "github.com/juju/juju/internal/bundle/changes"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetCharm returns the charm by name, source and revision. Calling this method
	// will return all the data associated with the charm. It is not expected to
	// call this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
	GetCharm(ctx context.Context, locator applicationcharm.CharmLocator) (charm.Charm, applicationcharm.CharmLocator, bool, error)
}

// ModelMigrationFactory defines an interface for getting a model migrator.
type ModelMigrationFactory interface {
	// ModelExporter returns a model exporter for the current model.
	ModelExporter(context.Context, model.UUID, facade.LegacyStateExporter) (facade.ModelExporter, error)
}

// ObjectStoreFactory defines an interface for accessing the object store.
type ObjectStoreFactory interface {
	// ObjectStore returns the object store for the current model.
	ObjectStore() objectstore.ObjectStore
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// APIv8 provides the Bundle API facade for version 8. It drops IncludeSeries
// from ExportBundle params, and drops series entirely from ExportBundle output
type APIv8 struct {
	*BundleAPI
}

// BundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type BundleAPI struct {

	// Access control.
	authorizer facade.Authorizer
	modelTag   names.ModelTag

	// Services.
	applicationService ApplicationService
	networkService     NetworkService

	exportModel exportModelFunc

	logger corelogger.Logger
}

// exportModelFunc is a function type used to export a model's description
// within a given context. It returns a Model and an error.
type exportModelFunc func(context.Context) (description.Model, error)

// NewFacade provides the required signature for facade registration.
func newFacade(ctx facade.ModelContext) (*BundleAPI, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	modelTag := names.NewModelTag(st.ModelUUID())

	return NewBundleAPI(
		getModelExporter(ctx, model.UUID(st.ModelUUID()), ctx.ObjectStore(), st),
		authorizer,
		modelTag,
		ctx.DomainServices().Network(),
		ctx.DomainServices().Application(),
		ctx.Logger().Child("bundlechanges"),
	)
}

// NewBundleAPI returns the new Bundle API facade.
func NewBundleAPI(
	exportModel exportModelFunc,
	auth facade.Authorizer,
	tag names.ModelTag,
	networkService NetworkService,
	applicationService ApplicationService,
	logger corelogger.Logger,
) (*BundleAPI, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &BundleAPI{
		exportModel:        exportModel,
		authorizer:         auth,
		modelTag:           tag,
		networkService:     networkService,
		applicationService: applicationService,
		logger:             logger,
	}, nil
}

func (b *BundleAPI) checkCanRead(ctx context.Context) error {
	return b.authorizer.HasPermission(ctx, permission.ReadAccess, b.modelTag)
}

type validators struct {
	verifyConstraints func(string) error
	verifyStorage     func(string) error
	verifyDevices     func(string) error
}

func (b *BundleAPI) doGetBundleChanges(
	ctx context.Context,
	args params.BundleChangesParams,
	vs validators,
) ([]bundlechanges.Change, []error, error) {
	dataSource, _ := charm.StreamBundleDataSource(strings.NewReader(args.BundleDataYAML), args.BundleURL)
	data, err := charm.ReadAndMergeBundleData(dataSource)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot read bundle YAML")
	}
	if err := data.Verify(vs.verifyConstraints, vs.verifyStorage, vs.verifyDevices); err != nil {
		if verificationError, ok := err.(*charm.VerificationError); ok {
			validationErrors := make([]error, len(verificationError.Errors))
			for i, e := range verificationError.Errors {
				validationErrors[i] = e
			}
			return nil, validationErrors, nil
		}
		// This should never happen as Verify only returns verification errors.
		return nil, nil, errors.Annotate(err, "cannot verify bundle")
	}
	changes, err := bundlechanges.FromData(
		ctx,
		bundlechanges.ChangesConfig{
			Bundle:    data,
			BundleURL: args.BundleURL,
			Logger:    b.logger,
		})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return changes, nil, nil
}

// GetChangesMapArgs returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// V4 GetChangesMapArgs is not supported on anything less than v4
func (b *BundleAPI) GetChangesMapArgs(ctx context.Context, args params.BundleChangesParams) (params.BundleChangesMapArgsResults, error) {
	vs := validators{
		verifyConstraints: func(s string) error {
			_, err := constraints.Parse(s)
			return err
		},
		verifyStorage: func(s string) error {
			_, err := storage.ParseDirective(s)
			return err
		},
		verifyDevices: func(s string) error {
			_, err := devices.ParseConstraints(s)
			return err
		},
	}
	return b.doGetBundleChangesMapArgs(ctx, args, vs, func(changes []bundlechanges.Change, results *params.BundleChangesMapArgsResults) error {
		results.Changes = make([]*params.BundleChangesMapArgs, len(changes))
		results.Errors = make([]string, len(changes))
		for i, c := range changes {
			args, err := c.Args()
			if err != nil {
				results.Errors[i] = err.Error()
				continue
			}
			results.Changes[i] = &params.BundleChangesMapArgs{
				Id:       c.Id(),
				Method:   c.Method(),
				Args:     args,
				Requires: c.Requires(),
			}
		}
		return nil
	})
}

func (b *BundleAPI) doGetBundleChangesMapArgs(
	ctx context.Context,
	args params.BundleChangesParams,
	vs validators,
	postProcess func([]bundlechanges.Change, *params.BundleChangesMapArgsResults) error,
) (params.BundleChangesMapArgsResults, error) {
	var results params.BundleChangesMapArgsResults
	changes, validationErrors, err := b.doGetBundleChanges(ctx, args, vs)
	if err != nil {
		return results, errors.Trace(err)
	}
	if len(validationErrors) > 0 {
		results.Errors = make([]string, len(validationErrors))
		for k, v := range validationErrors {
			results.Errors[k] = v.Error()
		}
		return results, nil
	}
	err = postProcess(changes, &results)
	return results, err
}

// ExportBundle exports the current model configuration as bundle.
func (b *BundleAPI) ExportBundle(ctx context.Context, arg params.ExportBundleParams) (params.StringResult, error) {
	fail := func(failErr error) (params.StringResult, error) {
		return params.StringResult{}, apiservererrors.ServerError(failErr)
	}

	if err := b.checkCanRead(ctx); err != nil {
		return fail(err)
	}

	model, err := b.exportModel(ctx)
	if err != nil {
		return fail(err)
	}

	// Fill it in charm.BundleData data structure.
	bundleData, err := b.fillBundleData(ctx, model, arg.IncludeCharmDefaults)
	if err != nil {
		return fail(err)
	}

	// Split the bundle into a base and overlay bundle and encode as a
	// yaml multi-doc.
	base, overlay, err := charm.ExtractBaseAndOverlayParts(bundleData)
	if err != nil {
		return fail(err)
	}

	// First create a bundle output from the bundle data.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err != nil {
		return fail(err)
	}
	if err = enc.Encode(bundleOutputFromBundleData(base)); err != nil {
		return fail(err)
	}

	// Secondly create an output from the overlay. We do it this way, so we can
	// insert the correct comments for users.
	output := buf.String()
	buf.Reset()
	if err = enc.Encode(overlay); err != nil {
		return fail(err)
	} else if err = enc.Close(); err != nil {
		return fail(err)
	}
	overlayOutput := buf.String()

	// If the overlay part is empty, ignore it; otherwise, inject a
	// comment to let users know that the second document can be extracted
	// out and used as a standalone overlay.
	if !strings.HasPrefix(overlayOutput, "--- {}\n") {
		// strip off the first three dashes and merge the base bundle and the
		// overlay.
		if strings.HasPrefix(overlayOutput, "---") {
			overlayOutput = strings.Replace(overlayOutput, "---", "--- # overlay.yaml", 1)
			output += overlayOutput
		} else {
			return fail(errors.Errorf("expected yaml encoder to delineate multiple documents with \"---\" separator"))
		}
	}

	return params.StringResult{Result: output}, nil
}

// bundleOutput has the same top level keys as the charm.BundleData
// but in a more user oriented output order, with the description first,
// then the distro base, then the apps, machines and releations.
type bundleOutput struct {
	Type         string                            `yaml:"bundle,omitempty"`
	Description  string                            `yaml:"description,omitempty"`
	DefaultBase  string                            `yaml:"default-base,omitempty"`
	Saas         map[string]*charm.SaasSpec        `yaml:"saas,omitempty"`
	Applications map[string]*charm.ApplicationSpec `yaml:"applications,omitempty"`
	Machines     map[string]*charm.MachineSpec     `yaml:"machines,omitempty"`
	Relations    [][]string                        `yaml:"relations,omitempty"`
}

func bundleOutputFromBundleData(bd *charm.BundleData) *bundleOutput {
	return &bundleOutput{
		Type:         bd.Type,
		Description:  bd.Description,
		DefaultBase:  bd.DefaultBase,
		Saas:         bd.Saas,
		Applications: bd.Applications,
		Machines:     bd.Machines,
		Relations:    bd.Relations,
	}
}

func (b *BundleAPI) fillBundleData(ctx context.Context, model description.Model, includeCharmDefaults bool) (*charm.BundleData, error) {
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, errors.Trace(err)
	}
	var defaultBase corebase.Base
	value, ok := cfg.DefaultBase()
	if ok {
		var err error
		defaultBase, err = corebase.ParseBaseFromString(value)
		if err != nil {
			return nil, err
		}
	} else {
		defaultBase = version.DefaultSupportedLTSBase()
	}

	data := &charm.BundleData{}

	isCAAS := model.Type() == description.CAAS
	if isCAAS {
		data.Type = "kubernetes"
	} else {
		data.DefaultBase = defaultBase.String()
	}

	if len(model.Applications()) == 0 {
		return nil, errors.Errorf("nothing to export as there are no applications")
	}

	// Application bundle data.
	var (
		usedBases  set.Strings
		machineIds set.Strings
	)
	data.Applications, machineIds, usedBases, err = b.bundleDataApplications(ctx, model.Applications(), defaultBase, isCAAS, includeCharmDefaults)
	if err != nil {
		return nil, err
	}

	// Machine bundle data.
	var machineBases set.Strings
	data.Machines, machineBases, err = b.bundleDataMachines(model.Machines(), machineIds, defaultBase)
	if err != nil {
		return nil, err
	}
	usedBases = usedBases.Union(machineBases)

	// Remote Application bundle data.
	data.Saas = bundleDataRemoteApplications(model.RemoteApplications())

	// Relation bundle data.
	data.Relations = bundleDataRelations(model.Relations())

	// If there is only one base used, make it the default and remove
	// base from all the apps and machines.
	size := usedBases.Size()
	switch {
	case size == 1:
		used, err := corebase.ParseBaseFromString(usedBases.Values()[0])
		if err != nil {
			return nil, errors.Trace(err)
		}
		if used != defaultBase {
			data.DefaultBase = used.String()
			for _, app := range data.Applications {
				app.Base = ""
			}
			for _, mac := range data.Machines {
				mac.Base = ""
			}
		}
	case size > 1:
		if !usedBases.Contains(defaultBase.String()) {
			data.DefaultBase = ""
		}
	}

	if isCAAS {
		// Kubernetes bundles don't specify bases right now.
		data.DefaultBase = ""
	}

	return data, nil
}

func (b *BundleAPI) bundleDataApplications(
	ctx context.Context,
	apps []description.Application,
	defaultBase corebase.Base,
	isCAAS, includeCharmDefaults bool,
) (map[string]*charm.ApplicationSpec, set.Strings, set.Strings, error) {

	allSpacesInfoLookup, err := b.networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "unable to retrieve all space information")
	}

	applicationData := make(map[string]*charm.ApplicationSpec)
	machineIds := set.NewStrings()
	usedBases := set.NewStrings()

	charmConfigCache := make(map[string]*charm.Config)
	printEndpointBindingSpaceNames := b.printSpaceNamesInEndpointBindings(apps)

	for _, application := range apps {
		if application.CharmOrigin() == nil || application.CharmOrigin().Platform() == "" {
			return nil, nil, nil, errors.Errorf("missing charm origin data for %q", application)
		}
		var newApplication *charm.ApplicationSpec
		p, err := corecharm.ParsePlatformNormalize(application.CharmOrigin().Platform())
		if err != nil {
			return nil, nil, nil, fmt.Errorf("extracting charm origin from application description %w", err)
		}

		appBase, err := corebase.ParseBase(p.OS, p.Channel)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("extracting base from application description %w", err)
		}
		usedBases.Add(appBase.String())

		endpointsWithSpaceNames, err := b.endpointBindings(application.EndpointBindings(), allSpacesInfoLookup, printEndpointBindingSpaceNames)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}

		// For security purposes we do not allow both the expose flag
		// and the exposed endpoints fields to be populated in exported
		// bundles. Otherwise, exporting a bundle from a 2.9 controller
		// (with per-endpoint expose settings) and deploying it to a
		// 2.8 controller would result in all application ports to be
		// made accessible from 0.0.0.0/0.
		exposedEndpoints, err := mapExposedEndpoints(application.ExposedEndpoints(), allSpacesInfoLookup)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		exposedFlag := application.Exposed() && len(exposedEndpoints) == 0

		// We need to correctly handle charmhub urls. The internal
		// representation of a charm url is not the same as a external
		// representation, in that only the application name should be rendered.
		// For charmhub charms, ensure that the revision is listed separately,
		// not in the charm url.
		curl, err := charm.ParseURL(application.CharmURL())
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		var charmURL string
		var revision *int
		switch {
		case charm.CharmHub.Matches(curl.Schema):
			charmURL = curl.Name
			if curl.Revision >= 0 {
				cRev := curl.Revision
				revision = &cRev
			}
		case charm.Local.Matches(curl.Schema):
			charmURL = fmt.Sprintf("local:%s", curl.Name)
			if curl.Revision >= 0 {
				charmURL = fmt.Sprintf("%s-%d", charmURL, curl.Revision)
			}
		default:
			return nil, nil, nil, errors.NotValidf("charm schema %q", curl.Schema)
		}

		var channel string
		if origin := application.CharmOrigin(); origin != nil {
			channel = origin.Channel()
		}
		if channel == "" {
			channel = application.Channel()
		}

		charmCfg := application.CharmConfig()
		if includeCharmDefaults {
			// Augment the user specified config with defaults
			// from the charm config metadata.
			cfgInfo, ok := charmConfigCache[charmURL]
			if !ok {
				source, err := applicationcharm.ParseCharmSchema(charm.Schema(curl.Schema))
				if err != nil {
					return nil, nil, nil, errors.Trace(err)
				}
				ch, _, _, err := b.applicationService.GetCharm(ctx, applicationcharm.CharmLocator{
					Source:   source,
					Name:     curl.Name,
					Revision: curl.Revision,
				})
				if errors.Is(err, applicationerrors.CharmNotFound) {
					return nil, nil, nil, errors.NotFoundf("charm %q", curl)
				} else if err != nil {
					return nil, nil, nil, errors.Trace(err)
				}
				cfgInfo = ch.Config()
				charmConfigCache[charmURL] = cfgInfo
			}
			for name, opt := range cfgInfo.Options {
				if _, ok := charmCfg[name]; ok {
					continue
				}
				charmCfg[name] = opt.Default
			}
		}
		if application.Subordinate() {
			newApplication = &charm.ApplicationSpec{
				Charm:            charmURL,
				Revision:         revision,
				Channel:          channel,
				Expose:           exposedFlag,
				ExposedEndpoints: exposedEndpoints,
				Options:          charmCfg,
				Annotations:      application.Annotations(),
				EndpointBindings: endpointsWithSpaceNames,
			}
		} else {
			var (
				numUnits  int
				scale     int
				placement string
				ut        []string
			)
			if isCAAS {
				placement = application.Placement()
				scale = len(application.Units())
			} else {
				numUnits = len(application.Units())
				for _, unit := range application.Units() {
					machineID := unit.Machine()
					unitMachine := names.NewMachineTag(machineID)
					if names.IsContainerMachine(machineID) {
						machineIds.Add(unitMachine.Parent().Id())
						id := unitMachine.ContainerType() + ":" + unitMachine.Parent().Id()
						ut = append(ut, id)
					} else {
						machineIds.Add(unitMachine.Id())
						ut = append(ut, unitMachine.Id())
					}
				}
			}

			newApplication = &charm.ApplicationSpec{
				Charm:            charmURL,
				Revision:         revision,
				Channel:          channel,
				NumUnits:         numUnits,
				Scale_:           scale,
				Placement_:       placement,
				To:               ut,
				Expose:           exposedFlag,
				ExposedEndpoints: exposedEndpoints,
				Options:          charmCfg,
				Annotations:      application.Annotations(),
				EndpointBindings: endpointsWithSpaceNames,
			}
		}

		newApplication.Resources = applicationDataResources(application.Resources())

		if appBase != defaultBase {
			newApplication.Base = appBase.String()
		}
		if result := b.constraints(application.Constraints()); len(result) != 0 {
			newApplication.Constraints = strings.Join(result, " ")
		}
		if cons := application.StorageDirectives(); len(cons) != 0 {
			newApplication.Storage = make(map[string]string)
			for name, constr := range cons {
				if newApplication.Storage[name], err = storage.ToString(storage.Directive{
					Pool:  constr.Pool(),
					Size:  constr.Size(),
					Count: constr.Count(),
				}); err != nil {
					return nil, nil, nil, errors.NotValidf("storage %q for %q", name, application.Name())
				}
			}
		}

		// If this application has been trusted by the operator, set the
		// Trust field of the ApplicationSpec to true
		if appConfig := application.ApplicationConfig(); appConfig != nil {
			newApplication.RequiresTrust = appConfig[coreapplication.TrustConfigOptionName] == true
		}

		// Populate offer list
		if offerList := application.Offers(); offerList != nil {
			newApplication.Offers = make(map[string]*charm.OfferSpec)
			for _, offer := range offerList {
				endpoints := offer.Endpoints()
				exposedEndpointNames := make([]string, 0, len(endpoints))
				for _, ep := range endpoints {
					exposedEndpointNames = append(exposedEndpointNames, ep)
				}
				sort.Strings(exposedEndpointNames)
				newApplication.Offers[offer.OfferName()] = &charm.OfferSpec{
					Endpoints: exposedEndpointNames,
					ACL:       b.filterOfferACL(offer.ACL()),
				}
			}
		}

		applicationData[application.Name()] = newApplication
	}
	return applicationData, machineIds, usedBases, nil
}

func applicationDataResources(resources []description.Resource) map[string]interface{} {
	var resourceData map[string]interface{}
	for _, res := range resources {
		appRev := res.ApplicationRevision()
		if appRev == nil || appRev.Origin() != resource.OriginStore.String() {
			continue
		}
		if resourceData == nil {
			resourceData = make(map[string]interface{})
		}
		resourceData[res.Name()] = res.ApplicationRevision().Revision()
	}
	return resourceData
}

func (b *BundleAPI) bundleDataMachines(machines []description.Machine, machineIds set.Strings, defaultBase corebase.Base) (map[string]*charm.MachineSpec, set.Strings, error) {
	usedBases := set.NewStrings()
	machineData := make(map[string]*charm.MachineSpec)
	for _, machine := range machines {
		if !machineIds.Contains(machine.Id()) {
			continue
		}
		macBase, err := corebase.ParseBaseFromString(machine.Base())
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		usedBases.Add(macBase.String())

		newMachine := &charm.MachineSpec{
			Annotations: machine.Annotations(),
		}
		if macBase != defaultBase {
			newMachine.Base = macBase.String()
		}

		if result := b.constraints(machine.Constraints()); len(result) != 0 {
			newMachine.Constraints = strings.Join(result, " ")
		}

		machineData[machine.Id()] = newMachine
	}
	return machineData, usedBases, nil
}

func bundleDataRemoteApplications(remoteApps []description.RemoteApplication) map[string]*charm.SaasSpec {
	saas := make(map[string]*charm.SaasSpec, len(remoteApps))
	for _, application := range remoteApps {
		newSaas := &charm.SaasSpec{
			URL: application.URL(),
		}
		saas[application.Name()] = newSaas
	}
	return saas
}

func bundleDataRelations(relations []description.Relation) [][]string {
	var relationData [][]string
	for _, relation := range relations {
		var endpointRelation []string
		for _, endpoint := range relation.Endpoints() {
			// skipping the 'peer' role which is not of concern in exporting the current model configuration.
			if endpoint.Role() == "peer" {
				continue
			}
			endpointRelation = append(endpointRelation, endpoint.ApplicationName()+":"+endpoint.Name())
		}
		if len(endpointRelation) != 0 {
			relationData = append(relationData, endpointRelation)
		}
	}
	return relationData
}

// mapExposedEndpoints converts the description package representation of the
// exposed endpoint settings into a format that can be included in the exported
// bundle output.  The provided spaceInfos list is used to convert space IDs
// into space names.
func mapExposedEndpoints(exposedEndpoints map[string]description.ExposedEndpoint, spaceInfos network.SpaceInfos) (map[string]charm.ExposedEndpointSpec, error) {
	if len(exposedEndpoints) == 0 {
		return nil, nil
	} else if allEndpointParams, found := exposedEndpoints[""]; found && len(exposedEndpoints) == 1 {
		// We have a single entry for the wildcard endpoint; check if
		// it only includes an expose to all networks CIDR.
		var allNetworkCIDRCount int
		for _, cidr := range allEndpointParams.ExposeToCIDRs() {
			if cidr == firewall.AllNetworksIPV4CIDR || cidr == firewall.AllNetworksIPV6CIDR {
				allNetworkCIDRCount++
			}
		}

		if len(allEndpointParams.ExposeToSpaceIDs()) == 0 &&
			len(allEndpointParams.ExposeToCIDRs()) == allNetworkCIDRCount {
			return nil, nil // equivalent to using non-granular expose like pre 2.9 juju
		}
	}

	res := make(map[string]charm.ExposedEndpointSpec, len(exposedEndpoints))
	for endpointName, exposeDetails := range exposedEndpoints {
		exposeToCIDRs := exposeDetails.ExposeToCIDRs()
		exposeToSpaceNames, err := mapSpaceIDsToNames(spaceInfos, exposeDetails.ExposeToSpaceIDs())
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Ensure consistent ordering of results
		sort.Strings(exposeToSpaceNames)
		sort.Strings(exposeToCIDRs)

		res[endpointName] = charm.ExposedEndpointSpec{
			ExposeToSpaces: exposeToSpaceNames,
			ExposeToCIDRs:  exposeToCIDRs,
		}
	}

	return res, nil
}

func mapSpaceIDsToNames(spaceInfos network.SpaceInfos, spaceIDs []string) ([]string, error) {
	if len(spaceIDs) == 0 {
		return nil, nil
	}

	spaceNames := make([]string, len(spaceIDs))
	for i, spaceID := range spaceIDs {
		sp := spaceInfos.GetByID(spaceID)
		if sp == nil {
			return nil, errors.NotFoundf("space with ID %q", spaceID)
		}

		spaceNames[i] = string(sp.Name)
	}

	return spaceNames, nil
}

func (b *BundleAPI) printSpaceNamesInEndpointBindings(apps []description.Application) bool {
	// Assumption: if all endpoint bindings in the bundle are in the
	// same space, spaces aren't really in use and will "muddy the waters"
	// for export bundle.
	spaceName := set.NewStrings()
	for _, app := range apps {
		for _, v := range app.EndpointBindings() {
			spaceName.Add(v)
		}
		if spaceName.Size() > 1 {
			return true
		}
	}
	return false
}

func (b *BundleAPI) endpointBindings(bindings map[string]string, spaceLookup network.SpaceInfos, printValue bool) (map[string]string, error) {
	if !printValue {
		return nil, nil
	}
	endpointBindings, err := state.NewBindings(nil, bindings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return endpointBindings.MapWithSpaceNames(spaceLookup)
}

// filterOfferACL prunes the input offer ACL to remove internal juju users that
// we shouldn't export as part of the bundle.
func (b *BundleAPI) filterOfferACL(in map[string]string) map[string]string {
	delete(in, permission.EveryoneUserName.Name())
	return in
}

func (b *BundleAPI) constraints(cons description.Constraints) []string {
	if cons == nil {
		return []string{}
	}

	var result []string
	if arch := cons.Architecture(); arch != "" {
		result = append(result, "arch="+arch)
	}
	if cores := cons.CpuCores(); cores != 0 {
		result = append(result, "cpu-cores="+strconv.Itoa(int(cores)))
	}
	if power := cons.CpuPower(); power != 0 {
		result = append(result, "cpu-power="+strconv.Itoa(int(power)))
	}
	if mem := cons.Memory(); mem != 0 {
		result = append(result, "mem="+strconv.Itoa(int(mem)))
	}
	if disk := cons.RootDisk(); disk != 0 {
		result = append(result, "root-disk="+strconv.Itoa(int(disk)))
	}
	if instType := cons.InstanceType(); instType != "" {
		result = append(result, "instance-type="+instType)
	}
	if imageID := cons.ImageID(); imageID != "" {
		result = append(result, "image-id="+imageID)
	}
	if container := cons.Container(); container != "" {
		result = append(result, "container="+container)
	}
	if virtType := cons.VirtType(); virtType != "" {
		result = append(result, "virt-type="+virtType)
	}
	if tags := cons.Tags(); len(tags) != 0 {
		result = append(result, "tags="+strings.Join(tags, ","))
	}
	if spaces := cons.Spaces(); len(spaces) != 0 {
		result = append(result, "spaces="+strings.Join(spaces, ","))
	}
	if zones := cons.Zones(); len(zones) != 0 {
		result = append(result, "zones="+strings.Join(zones, ","))
	}
	if rootDiskSource := cons.RootDiskSource(); rootDiskSource != "" {
		result = append(result, "root-disk-source="+rootDiskSource)
	}
	return result
}

// getModelExporter returns a function that exports a model's partial description
// based on specified configurations.
func getModelExporter(
	factory ModelMigrationFactory,
	modelUUID model.UUID,
	store objectstore.ObjectStore,
	legacyExporter facade.LegacyStateExporter,
) func(ctx context.Context) (description.Model, error) {
	return func(ctx context.Context) (description.Model, error) {
		exporter, err := factory.ModelExporter(ctx, modelUUID, legacyExporter)
		if err != nil {
			return nil, errors.Trace(err)
		}

		exportConfig := state.ExportConfig{
			IgnoreIncompleteModel:    false,
			SkipActions:              true,
			SkipAnnotations:          false,
			SkipCloudImageMetadata:   true,
			SkipCredentials:          true,
			SkipIPAddresses:          true,
			SkipSettings:             false,
			SkipSSHHostKeys:          true,
			SkipLinkLayerDevices:     true,
			SkipUnitAgentBinaries:    true,
			SkipMachineAgentBinaries: true,
			SkipRelationData:         true,
			SkipInstanceData:         true,
			SkipApplicationOffers:    false,
			SkipOfferConnections:     false,
			SkipSecrets:              true,
		}

		exported, err := exporter.ExportModelPartial(ctx, exportConfig, store)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return exported, nil
	}
}
