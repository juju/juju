// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/collections/set"
	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	appFacade "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/storage"
)

var logger = loggo.GetLogger("juju.apiserver.bundle")

// APIv1 provides the Bundle API facade for version 1.
type APIv1 struct {
	*APIv2
}

// APIv2 provides the Bundle API facade for version 2.
type APIv2 struct {
	*BundleAPI
}

// APIv3 provides the Bundle API facade for version 3. It is otherwise
// identical to V2 with the exception that the V3 ExportBundle implementation
// also exposes the the current trust status for each application.
type APIv3 struct {
	*BundleAPI
}

// APIv4 provides the Bundle API facade for version 4. It is otherwise
// identical to V3 with the exception that the V4 now has GetChangesAsMap, which
// returns the same data as GetChanges, but with better args data.
type APIv4 struct {
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
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// NewFacadeV3 provides the signature required for facade registration
// for version 3.
func NewFacadeV3(ctx facade.Context) (*APIv3, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// NewFacadeV4 provides the signature required for facade registration
// for version 4.
func NewFacadeV4(ctx facade.Context) (*APIv4, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
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
		return nil, common.ErrPerm
	}

	return &BundleAPI{
		backend:    st,
		authorizer: auth,
		modelTag:   tag,
	}, nil
}

// NewBundleAPIv1 returns the new Bundle APIv1 facade.
func NewBundleAPIv1(
	st Backend,
	auth facade.Authorizer,
	tag names.ModelTag,
) (*APIv1, error) {
	api, err := NewBundleAPI(st, auth, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{&APIv2{api}}, nil
}

func (b *BundleAPI) checkCanRead() error {
	canRead, err := b.authorizer.HasPermission(permission.ReadAccess, b.modelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// V1 GetChanges did not support device.
func (b *APIv1) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	vs := validators{
		verifyConstraints: func(s string) error {
			_, err := constraints.Parse(s)
			return err
		},
		verifyStorage: func(s string) error {
			_, err := storage.ParseConstraints(s)
			return err
		},
		verifyDevices: nil,
	}
	return getChanges(args, vs, func(changes []bundlechanges.Change, results *params.BundleChangesResults) error {
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
	})
}

type validators struct {
	verifyConstraints func(string) error
	verifyStorage     func(string) error
	verifyDevices     func(string) error
}

func getBundleChanges(args params.BundleChangesParams,
	vs validators,
) ([]bundlechanges.Change, []error, error) {
	data, err := charm.ReadBundleData(strings.NewReader(args.BundleDataYAML))
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

func getChanges(
	args params.BundleChangesParams,
	vs validators,
	postProcess func([]bundlechanges.Change, *params.BundleChangesResults) error,
) (params.BundleChangesResults, error) {
	var results params.BundleChangesResults
	changes, validationErrors, err := getBundleChanges(args, vs)
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

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
// GetChanges has been superseded in favour of GetChangesMapArgs. It's
// preferable to use that new method to add new functionality and move clients
// away from this one.
func (b *BundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
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
	return getChanges(args, vs, func(changes []bundlechanges.Change, results *params.BundleChangesResults) error {
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
	})
}

// GetChangesMapArgs is not in V3 API or less.
// Mask the new method from V3 API or less.
func (u *APIv3) GetChangesMapArgs() (_, _ struct{}) { return }

// GetChangesMapArgs returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// V4 GetChangesMapArgs is not supported on anything less than v4
func (b *BundleAPI) GetChangesMapArgs(args params.BundleChangesParams) (params.BundleChangesMapArgsResults, error) {
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
	return getChangesMapArgs(args, vs, func(changes []bundlechanges.Change, results *params.BundleChangesMapArgsResults) error {
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

func getChangesMapArgs(
	args params.BundleChangesParams,
	vs validators,
	postProcess func([]bundlechanges.Change, *params.BundleChangesMapArgsResults) error,
) (params.BundleChangesMapArgsResults, error) {
	var results params.BundleChangesMapArgsResults
	changes, validationErrors, err := getBundleChanges(args, vs)
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
func (b *BundleAPI) ExportBundle() (params.StringResult, error) {
	fail := func(failErr error) (params.StringResult, error) {
		return params.StringResult{}, common.ServerError(failErr)
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
	bundleData, err := b.fillBundleData(model)
	if err != nil {
		return fail(err)
	}

	// Split the bundle into a base and overlay bundle and encode as a
	// yaml multi-doc.
	base, overlay, err := charm.ExtractBaseAndOverlayParts(bundleData)
	if err != nil {
		return fail(err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err = enc.Encode(bundleOutputFromBundleData(base)); err != nil {
		return fail(err)
	} else if err = enc.Encode(overlay); err != nil {
		return fail(err)
	} else if err = enc.Close(); err != nil {
		return fail(err)
	}

	// If the overlay part is empty, strip it off; otherwise, inject a
	// comment to let users know that the second document can be extracted
	// out and used as a standalone overlay.
	yamlOut := buf.String()
	if strings.HasSuffix(yamlOut, "--- {}\n") {
		yamlOut = yamlOut[:strings.Index(yamlOut, "---")]
	} else {
		yamlOut = strings.Replace(yamlOut, "---", "--- # overlay.yaml", 1)
	}

	return params.StringResult{Result: yamlOut}, nil
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

func (b *BundleAPI) fillBundleData(model description.Model) (*charm.BundleData, error) {
	cfg := model.Config()
	value, ok := cfg["default-series"]
	if !ok {
		value = series.LatestLts()
	}
	defaultSeries := fmt.Sprintf("%v", value)

	data := &charm.BundleData{
		Saas:         make(map[string]*charm.SaasSpec),
		Applications: make(map[string]*charm.ApplicationSpec),
		Machines:     make(map[string]*charm.MachineSpec),
		Relations:    [][]string{},
	}
	isCaas := model.Type() == description.CAAS
	if isCaas {
		data.Type = "kubernetes"
	} else {
		data.Series = defaultSeries
	}

	if len(model.Applications()) == 0 {
		return nil, errors.Errorf("nothing to export as there are no applications")
	}
	machineIds := set.NewStrings()
	usedSeries := set.NewStrings()
	spaceNamesByID, err := b.backend.SpaceNamesByID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, application := range model.Applications() {
		var newApplication *charm.ApplicationSpec
		appSeries := application.Series()
		usedSeries.Add(appSeries)
		bindings := make(map[string]string)
		for ep, spaceID := range application.EndpointBindings() {
			// Only care about endpoints with bindings not in the default space.
			if spaceID == network.DefaultSpaceId {
				continue
			}
			spaceName, ok := spaceNamesByID[spaceID]
			if !ok {
				logger.Warningf("export bundle: space name for id %q not found", spaceID)
				continue
			}
			bindings[ep] = spaceName
		}
		if application.Subordinate() {
			newApplication = &charm.ApplicationSpec{
				Charm:            application.CharmURL(),
				Expose:           application.Exposed(),
				Options:          application.CharmConfig(),
				Annotations:      application.Annotations(),
				EndpointBindings: bindings,
			}
			if appSeries != defaultSeries {
				newApplication.Series = appSeries
			}
			if result := b.constraints(application.Constraints()); len(result) != 0 {
				newApplication.Constraints = strings.Join(result, " ")
			}
		} else {
			ut := []string{}
			placement := ""
			numUnits := 0
			scale := 0
			if isCaas {
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
				Charm:            application.CharmURL(),
				NumUnits:         numUnits,
				Scale_:           scale,
				Placement_:       placement,
				To:               ut,
				Expose:           application.Exposed(),
				Options:          application.CharmConfig(),
				Annotations:      application.Annotations(),
				EndpointBindings: bindings,
			}
			if appSeries != defaultSeries {
				newApplication.Series = appSeries
			}
			if result := b.constraints(application.Constraints()); len(result) != 0 {
				newApplication.Constraints = strings.Join(result, " ")
			}
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
				newApplication.Offers[offer.OfferName()] = &charm.OfferSpec{
					Endpoints: offer.Endpoints(),
					ACL:       b.filterOfferACL(offer.ACL()),
				}
			}
		}

		data.Applications[application.Name()] = newApplication
	}

	for _, machine := range model.Machines() {
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

		data.Machines[machine.Id()] = newMachine
	}

	for _, application := range model.RemoteApplications() {
		newSaas := &charm.SaasSpec{
			URL: application.URL(),
		}
		data.Saas[application.Name()] = newSaas
	}

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

	// Kubernetes bundles don't specify series right now.
	if isCaas {
		data.Series = ""
	}

	for _, relation := range model.Relations() {
		endpointRelation := []string{}
		for _, endpoint := range relation.Endpoints() {
			// skipping the 'peer' role which is not of concern in exporting the current model configuration.
			if endpoint.Role() == "peer" {
				continue
			}
			endpointRelation = append(endpointRelation, endpoint.ApplicationName()+":"+endpoint.Name())
		}
		if len(endpointRelation) != 0 {
			data.Relations = append(data.Relations, endpointRelation)
		}
	}

	return data, nil
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

	var constraints []string
	if arch := cons.Architecture(); arch != "" {
		constraints = append(constraints, "arch="+arch)
	}
	if cores := cons.CpuCores(); cores != 0 {
		constraints = append(constraints, "cpu-cores="+strconv.Itoa(int(cores)))
	}
	if power := cons.CpuPower(); power != 0 {
		constraints = append(constraints, "cpu-power="+strconv.Itoa(int(power)))
	}
	if mem := cons.Memory(); mem != 0 {
		constraints = append(constraints, "mem="+strconv.Itoa(int(mem)))
	}
	if disk := cons.RootDisk(); disk != 0 {
		constraints = append(constraints, "root-disk="+strconv.Itoa(int(disk)))
	}
	if instType := cons.InstanceType(); instType != "" {
		constraints = append(constraints, "instance-type="+instType)
	}
	if container := cons.Container(); container != "" {
		constraints = append(constraints, "container="+container)
	}
	if virtType := cons.VirtType(); virtType != "" {
		constraints = append(constraints, "virt-type="+virtType)
	}
	if tags := cons.Tags(); len(tags) != 0 {
		constraints = append(constraints, "tags="+strings.Join(tags, ","))
	}
	if spaces := cons.Spaces(); len(spaces) != 0 {
		constraints = append(constraints, "spaces="+strings.Join(spaces, ","))
	}
	return constraints
}
