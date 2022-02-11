// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/collections/set"
	"github.com/juju/description/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	appFacade "github.com/juju/juju/apiserver/facades/client/application"
	bundlechanges "github.com/juju/juju/core/bundle/changes"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// APIv1 provides the Bundle API facade for version 1.
type APIv1 struct {
	*APIv2
}

// APIv2 provides the Bundle API facade for version 2.
type APIv2 struct {
	*APIv3
}

// APIv3 provides the Bundle API facade for version 3. It is otherwise
// identical to V2 with the exception that the V3 ExportBundle implementation
// also exposes the the current trust status for each application.
type APIv3 struct {
	*APIv4
}

// APIv4 provides the Bundle API facade for version 4. It is otherwise
// identical to V3 with the exception that the V4 now has GetChangesAsMap, which
// returns the same data as GetChanges, but with better args data.
type APIv4 struct {
	*APIv5
}

// APIv5 provides the Bundle API facade for version 5. It is otherwise
// identical to V4 with the exception that the V5 adds an arg to export
// bundle to control what is exported..
type APIv5 struct {
	*APIv6
}

// APIv6 provides the Bundle API facade for version 6. It is otherwise
// identical to V5 with the exception that the V6 adds the support for
// multi-part yaml handling to GetChanges and GetChangesMapArgs.
type APIv6 struct {
	*BundleAPI
}

// BundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type BundleAPI struct {
	backend    Backend
	authorizer facade.Authorizer
	modelTag   names.ModelTag
}

// NewFacadeV1 provides the signature required for facade registration
// version 1.
func NewFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := NewFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &APIv1{api}, nil
}

// NewFacadeV2 provides the signature required for facade registration
// for version 2.
func NewFacadeV2(ctx facade.Context) (*APIv2, error) {
	api, err := NewFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewFacadeV3 provides the signature required for facade registration
// for version 3.
func NewFacadeV3(ctx facade.Context) (*APIv3, error) {
	api, err := NewFacadeV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// NewFacadeV4 provides the signature required for facade registration
// for version 4.
func NewFacadeV4(ctx facade.Context) (*APIv4, error) {
	api, err := NewFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// NewFacadeV5 provides the signature required for facade registration
// for version 5.
func NewFacadeV5(ctx facade.Context) (*APIv5, error) {
	api, err := NewFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
}

// NewFacadeV6 provides the signature required for facade registration
// for version 6.
func NewFacadeV6(ctx facade.Context) (*APIv6, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}

// NewFacade provides the required signature for facade registration.
func newFacade(ctx facade.Context) (*BundleAPI, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	return NewBundleAPI(
		NewStateShim(st),
		authorizer,
		names.NewModelTag(st.ModelUUID()),
	)
}

// NewBundleAPI returns the new Bundle API facade.
func NewBundleAPI(
	st Backend,
	auth facade.Authorizer,
	tag names.ModelTag,
) (*BundleAPI, error) {
	if !auth.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &BundleAPI{
		backend:    st,
		authorizer: auth,
		modelTag:   tag,
	}, nil
}

// NewBundleAPIv1 returns the new Bundle APIv1 facade.
// Deprecated:this only exists to support the deprecated
// client.GetBundleChanges() API.
func NewBundleAPIv1(
	st Backend,
	auth facade.Authorizer,
	tag names.ModelTag,
) (*APIv1, error) {
	api, err := NewBundleAPI(st, auth, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{&APIv2{&APIv3{&APIv4{&APIv5{&APIv6{api}}}}}}, nil
}

func (b *BundleAPI) checkCanRead() error {
	canRead, err := b.authorizer.HasPermission(permission.ReadAccess, b.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

type validators struct {
	verifyConstraints func(string) error
	verifyStorage     func(string) error
	verifyDevices     func(string) error
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// V1 GetChanges did not support device.
func (b *APIv1) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	return b.doGetChanges(args, 1)
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// GetChanges has been superseded in favour of GetChangesMapArgs. It's
// preferable to use that new method to add new functionality and move clients
// away from this one.
func (b *APIv5) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	return b.doGetChanges(args, 5)
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// GetChanges has been superseded in favour of GetChangesMapArgs. It's
// preferable to use that new method to add new functionality and move clients
// away from this one.
func (b *BundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	return b.doGetChanges(args, 6)
}

func (b *BundleAPI) doGetChanges(args params.BundleChangesParams, version int) (params.BundleChangesResults, error) {
	vs := validators{
		verifyConstraints: func(s string) error {
			_, err := constraints.Parse(s)
			return err
		},
		verifyStorage: func(s string) error {
			_, err := storage.ParseConstraints(s)
			return err
		},
		verifyDevices: func(s string) error {
			_, err := devices.ParseConstraints(s)
			return err
		},
	}

	expandOverlays := version > 5

	mapFn := mapBundleChanges

	if version == 1 {
		mapFn = mapBundleChangesV1
		vs.verifyDevices = nil // device verification is not supported for V1 facades
	}

	return b.getBundleChanges(args, vs, expandOverlays, mapFn)
}

func (b *BundleAPI) getBundleChanges(
	args params.BundleChangesParams,
	vs validators,
	expandOverlays bool,
	postProcess func([]bundlechanges.Change, *params.BundleChangesResults) error,
) (params.BundleChangesResults, error) {
	var results params.BundleChangesResults
	changes, validationErrors, err := b.doGetBundleChanges(args, vs, expandOverlays)
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
	return results, errors.Trace(err)

}

func mapBundleChangesV1(changes []bundlechanges.Change, results *params.BundleChangesResults) error {
	results.Changes = make([]*params.BundleChange, len(changes))
	for i, c := range changes {
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     c.GUIArgs(),
			Requires: c.Requires(),
		}
	}
	return nil
}

func mapBundleChanges(changes []bundlechanges.Change, results *params.BundleChangesResults) error {
	results.Changes = make([]*params.BundleChange, len(changes))
	for i, c := range changes {
		var guiArgs []interface{}
		switch c := c.(type) {
		case *bundlechanges.AddApplicationChange:
			guiArgs = c.GUIArgsWithDevices()
		default:
			guiArgs = c.GUIArgs()
		}
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     guiArgs,
			Requires: c.Requires(),
		}
	}
	return nil
}

func (b *BundleAPI) doGetBundleChanges(
	args params.BundleChangesParams,
	vs validators,
	expandOverlays bool,
) ([]bundlechanges.Change, []error, error) {
	var data *charm.BundleData
	var err error
	if expandOverlays {
		dataSource, _ := charm.StreamBundleDataSource(strings.NewReader(args.BundleDataYAML), args.BundleURL)
		data, err = charm.ReadAndMergeBundleData(dataSource)
	} else {
		data, err = charm.ReadBundleData(strings.NewReader(args.BundleDataYAML))
	}
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
		bundlechanges.ChangesConfig{
			Bundle:    data,
			BundleURL: args.BundleURL,
			Logger:    loggo.GetLogger("juju.apiserver.bundlechanges"),
		})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return changes, nil, nil
}

// GetChangesMapArgs is not in V3 API or less.
// Mask the new method from V3 API or less.
func (u *APIv3) GetChangesMapArgs() (_, _ struct{}) { return }

// GetChangesMapArgs returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// V4 GetChangesMapArgs is not supported on anything less than v4
func (b *APIv5) GetChangesMapArgs(args params.BundleChangesParams) (params.BundleChangesMapArgsResults, error) {
	return b.doGetChangesMapArgs(args, false)
}

// GetChangesMapArgs returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// V4 GetChangesMapArgs is not supported on anything less than v4
func (b *BundleAPI) GetChangesMapArgs(args params.BundleChangesParams) (params.BundleChangesMapArgsResults, error) {
	return b.doGetChangesMapArgs(args, true)
}

func (b *BundleAPI) doGetChangesMapArgs(args params.BundleChangesParams, expandOverlays bool) (params.BundleChangesMapArgsResults, error) {
	vs := validators{
		verifyConstraints: func(s string) error {
			_, err := constraints.Parse(s)
			return err
		},
		verifyStorage: func(s string) error {
			_, err := storage.ParseConstraints(s)
			return err
		},
		verifyDevices: func(s string) error {
			_, err := devices.ParseConstraints(s)
			return err
		},
	}
	return b.doGetBundleChangesMapArgs(args, vs, expandOverlays, func(changes []bundlechanges.Change, results *params.BundleChangesMapArgsResults) error {
		results.Changes = make([]*params.BundleChangesMapArgs, len(changes))
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
	args params.BundleChangesParams,
	vs validators,
	expandOverlays bool,
	postProcess func([]bundlechanges.Change, *params.BundleChangesMapArgsResults) error,
) (params.BundleChangesMapArgsResults, error) {
	var results params.BundleChangesMapArgsResults
	changes, validationErrors, err := b.doGetBundleChanges(args, vs, expandOverlays)
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

// ExportBundle v4 did not have any parameters.
func (b *APIv4) ExportBundle() (params.StringResult, error) {
	return b.APIv5.ExportBundle(params.ExportBundleParams{})
}

// ExportBundle exports the current model configuration as bundle.
func (b *BundleAPI) ExportBundle(arg params.ExportBundleParams) (params.StringResult, error) {
	fail := func(failErr error) (params.StringResult, error) {
		return params.StringResult{}, apiservererrors.ServerError(failErr)
	}

	if err := b.checkCanRead(); err != nil {
		return fail(err)
	}

	exportConfig := b.backend.GetExportConfig()
	model, err := b.backend.ExportPartial(exportConfig)
	if err != nil {
		return fail(err)
	}

	// Fill it in charm.BundleData data structure.
	bundleData, err := b.fillBundleData(model, arg.IncludeCharmDefaults, b.backend)
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
// then the distro series, then the apps, machines and releations.
type bundleOutput struct {
	Type         string                            `yaml:"bundle,omitempty"`
	Description  string                            `yaml:"description,omitempty"`
	Series       string                            `yaml:"series,omitempty"`
	Saas         map[string]*charm.SaasSpec        `yaml:"saas,omitempty"`
	Applications map[string]*charm.ApplicationSpec `yaml:"applications,omitempty"`
	Machines     map[string]*charm.MachineSpec     `yaml:"machines,omitempty"`
	Relations    [][]string                        `yaml:"relations,omitempty"`
}

func bundleOutputFromBundleData(bd *charm.BundleData) *bundleOutput {
	return &bundleOutput{
		Type:         bd.Type,
		Description:  bd.Description,
		Series:       bd.Series,
		Saas:         bd.Saas,
		Applications: bd.Applications,
		Machines:     bd.Machines,
		Relations:    bd.Relations,
	}
}

// ExportBundle is not in V1 API.
// Mask the new method from V1 API.
func (u *APIv1) ExportBundle() (_, _ struct{}) { return }

func (b *BundleAPI) fillBundleData(model description.Model, includeCharmDefaults bool, backend Backend) (*charm.BundleData, error) {
	cfg := model.Config()
	value, ok := cfg["default-series"]
	if !ok {
		value = series.LatestLts()
	}
	defaultSeries := fmt.Sprintf("%v", value)

	data := &charm.BundleData{}

	isCAAS := model.Type() == description.CAAS
	if isCAAS {
		data.Type = "kubernetes"
	} else {
		data.Series = defaultSeries
	}

	if len(model.Applications()) == 0 {
		return nil, errors.Errorf("nothing to export as there are no applications")
	}

	// Application bundle data.
	applications, machineIds, usedSeries, err := b.bundleDataApplications(model.Applications(), defaultSeries, isCAAS, includeCharmDefaults, backend)
	if err != nil {
		return nil, err
	}
	data.Applications = applications

	// Machine bundle data.
	var machineSeries set.Strings
	data.Machines, machineSeries = b.bundleDataMachines(model.Machines(), machineIds, defaultSeries)
	usedSeries = usedSeries.Union(machineSeries)

	// Remote Application bundle data.
	data.Saas = bundleDataRemoteApplications(model.RemoteApplications())

	// Relation bundle data.
	data.Relations = bundleDataRelations(model.Relations())

	// If there is only one series used, make it the default and remove
	// series from all the apps and machines.
	size := usedSeries.Size()
	switch {
	case size == 1:
		used := usedSeries.Values()[0]
		if used != defaultSeries {
			data.Series = used
			for _, app := range data.Applications {
				app.Series = ""
			}
			for _, mac := range data.Machines {
				mac.Series = ""
			}
		}
	case size > 1:
		if !usedSeries.Contains(defaultSeries) {
			data.Series = ""
		}
	}

	if isCAAS {
		// Kubernetes bundles don't specify series right now.
		data.Series = ""
	}

	return data, nil
}

func (b *BundleAPI) bundleDataApplications(
	apps []description.Application,
	defaultSeries string,
	isCAAS, includeCharmDefaults bool,
	backend Backend,
) (map[string]*charm.ApplicationSpec, set.Strings, set.Strings, error) {

	allSpacesInfoLookup, err := b.backend.AllSpaceInfos()
	if err != nil {
		return nil, nil, nil, errors.Annotate(err, "unable to retrieve all space information")
	}

	applicationData := make(map[string]*charm.ApplicationSpec)
	machineIds := set.NewStrings()
	usedSeries := set.NewStrings()

	charmConfigCache := make(map[string]*charm.Config)
	printEndpointBindingSpaceNames := b.printSpaceNamesInEndpointBindings(apps)

	for _, application := range apps {
		var newApplication *charm.ApplicationSpec
		appSeries := application.Series()
		usedSeries.Add(appSeries)

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
		// For charmstore and charmhub charms, ensure that the revision is
		// listed separately, not in the charm url.
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
		case charm.CharmStore.Matches(curl.Schema):
			if curl.Revision >= 0 {
				cRev := curl.Revision
				revision = &cRev
			}
			curl.Revision = -1
			charmURL = curl.String()
		case charm.Local.Matches(curl.Schema):
			charmURL = fmt.Sprintf("local:%s", curl.Name)
			if curl.Revision >= 0 {
				charmURL = fmt.Sprintf("%s-%d", charmURL, curl.Revision)
			}
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
				ch, err := backend.Charm(curl)
				if err != nil {
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
					machineID := unit.Machine().Id()
					unitMachine := unit.Machine()
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

		if appSeries != defaultSeries {
			newApplication.Series = appSeries
		}
		if result := b.constraints(application.Constraints()); len(result) != 0 {
			newApplication.Constraints = strings.Join(result, " ")
		}

		// If this application has been trusted by the operator, set the
		// Trust field of the ApplicationSpec to true
		if appConfig := application.ApplicationConfig(); appConfig != nil {
			newApplication.RequiresTrust = appConfig[appFacade.TrustConfigOptionName] == true
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
	return applicationData, machineIds, usedSeries, nil
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

func (b *BundleAPI) bundleDataMachines(machines []description.Machine, machineIds set.Strings, defaultSeries string) (map[string]*charm.MachineSpec, set.Strings) {
	usedSeries := set.NewStrings()
	machineData := make(map[string]*charm.MachineSpec)
	for _, machine := range machines {
		if !machineIds.Contains(machine.Tag().Id()) {
			continue
		}
		macSeries := machine.Series()
		usedSeries.Add(macSeries)
		newMachine := &charm.MachineSpec{
			Annotations: machine.Annotations(),
		}
		if macSeries != defaultSeries {
			newMachine.Series = macSeries
		}

		if result := b.constraints(machine.Constraints()); len(result) != 0 {
			newMachine.Constraints = strings.Join(result, " ")
		}

		machineData[machine.Id()] = newMachine
	}
	return machineData, usedSeries
}

func bundleDataRemoteApplications(remoteApps []description.RemoteApplication) map[string]*charm.SaasSpec {
	Saas := make(map[string]*charm.SaasSpec, len(remoteApps))
	for _, application := range remoteApps {
		newSaas := &charm.SaasSpec{
			URL: application.URL(),
		}
		Saas[application.Name()] = newSaas
	}
	return Saas
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
	endpointBindings, err := state.NewBindings(b.backend, bindings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return endpointBindings.MapWithSpaceNames(spaceLookup)
}

// filterOfferACL prunes the input offer ACL to remove internal juju users that
// we shouldn't export as part of the bundle.
func (b *BundleAPI) filterOfferACL(in map[string]string) map[string]string {
	delete(in, common.EveryoneTagName)
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
