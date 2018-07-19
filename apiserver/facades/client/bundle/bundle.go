// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/description"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
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
func (b *BundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	var results params.BundleChangesResults
	data, err := charm.ReadBundleData(strings.NewReader(args.BundleDataYAML))
	if err != nil {
		return results, errors.Annotate(err, "cannot read bundle YAML")
	}
	verifyConstraints := func(s string) error {
		_, err := constraints.Parse(s)
		return err
	}
	verifyStorage := func(s string) error {
		_, err := storage.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage); err != nil {
		if err, ok := err.(*charm.VerificationError); ok {
			results.Errors = make([]string, len(err.Errors))
			for i, e := range err.Errors {
				results.Errors[i] = e.Error()
			}
			return results, nil
		}
		// This should never happen as Verify only returns verification errors.
		return results, errors.Annotate(err, "cannot verify bundle")
	}
	changes, err := bundlechanges.FromData(
		bundlechanges.ChangesConfig{
			Bundle: data,
			Logger: loggo.GetLogger("juju.apiserver.bundlechanges"),
		})
	if err != nil {
		return results, err
	}
	results.Changes = make([]*params.BundleChange, len(changes))
	for i, c := range changes {
		results.Changes[i] = &params.BundleChange{
			Id:       c.Id(),
			Method:   c.Method(),
			Args:     c.GUIArgs(),
			Requires: c.Requires(),
		}
	}
	return results, nil
}

// ExportBundle exports the current model configuration as bundle.
func (b *BundleAPI) ExportBundle() (params.StringResult, error) {
	var fail = func(failErr error) (params.StringResult, error) {
		return params.StringResult{}, common.ServerError(failErr)
	}

	if err := b.checkCanRead(); err != nil {
		return fail(err)
	}

	exportConfig := b.backend.GetExportconfig()
	model, err := b.backend.ExportPartial(exportConfig)
	if err != nil {
		return fail(err)
	}

	// Fill it in charm.BundleData datastructure.
	bundleData, err := b.fillBundleData(model)
	if err != nil {
		return fail(err)
	}

	bytes, err := yaml.Marshal(bundleData)
	if err != nil {
		return fail(err)
	}

	return params.StringResult{
		Result: string(bytes),
	}, nil
}

// Mask the new method from V1 API.
// ExportBundle is not in V1 API.
func (u *APIv1) ExportBundle() (_, _ struct{}) { return }

// FillBundle fills the bundledata datastructure required for the exportBundle.
func (b *BundleAPI) fillBundleData(model description.Model) (*charm.BundleData, error) {
	// get the default-series from model config.
	cfg := model.Config()
	value, ok := cfg["default-series"]
	if !ok {
		value = series.LatestLts()
	}
	defaultSeries := fmt.Sprintf("%v", value)

	data := &charm.BundleData{
		Series:       defaultSeries,
		Applications: make(map[string]*charm.ApplicationSpec),
		Relations:    [][]string{},
		Machines:     make(map[string]*charm.MachineSpec),
	}

	if len(model.Applications()) == 0 {
		return &charm.BundleData{}, errors.Errorf("nothing to export as there is no application found.")
	}
	for _, application := range model.Applications() {
		var newApplication *charm.ApplicationSpec
		if application.Subordinate() {
			newApplication = &charm.ApplicationSpec{
				Charm:            application.CharmURL(),
				Series:           application.Series(),
				Expose:           application.Exposed(),
				Options:          application.CharmConfig(),
				Annotations:      application.Annotations(),
				EndpointBindings: application.EndpointBindings(),
			}
		} else {
			ut := []string{}
			for _, unit := range application.Units() {
				ut = append(ut, unit.Machine().Id())
			}

			newApplication = &charm.ApplicationSpec{
				Charm:            application.CharmURL(),
				Series:           application.Series(),
				NumUnits:         len(application.Units()),
				To:               ut,
				Expose:           application.Exposed(),
				Options:          application.CharmConfig(),
				Annotations:      application.Annotations(),
				EndpointBindings: application.EndpointBindings(),
			}
		}

		data.Applications[application.Name()] = newApplication
	}

	for _, machine := range model.Machines() {
		var constraints string
		result := b.constraints(machine.Constraints())
		if len(result) == 0 {
			constraints = strings.Join(result, " ")
		}

		newMachine := &charm.MachineSpec{
			Constraints: constraints,
			Annotations: machine.Annotations(),
			Series:      machine.Series(),
		}
		data.Machines[machine.Id()] = newMachine
	}

	endpointRelation := []string{}
	for _, relation := range model.Relations() {
		for _, endpoint := range relation.Endpoints() {
			// skipping the 'peer' role which is not of concern in exporting the current model configuration.
			if endpoint.Role() == "peer" {
				continue
			}
			endpointRelation = append(endpointRelation, endpoint.ApplicationName()+":"+endpoint.Name())
		}
	}
	data.Relations = append(data.Relations, endpointRelation)

	return data, nil
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
	return constraints
}
