// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package bundle defines an API endpoint for functions dealing with bundles.
package bundle

import (
	"strings"

	"github.com/juju/bundlechanges"
	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// NewFacade provides the required signature for facade registration.
func NewFacade(_ *state.State, _ facade.Resources, auth facade.Authorizer) (Bundle, error) {
	return NewBundle(auth)
}

// NewBundle creates and returns a new Bundle API facade.
func NewBundle(auth facade.Authorizer) (Bundle, error) {
	if !auth.AuthClient() {
		return nil, common.ErrPerm
	}
	return &bundleAPI{}, nil
}

// Bundle defines the API endpoint used to retrieve bundle changes.
type Bundle interface {
	// GetChanges returns the list of changes required to deploy the given
	// bundle data.
	GetChanges(params.BundleChangesParams) (params.BundleChangesResults, error)

	// ExportBundle exports the model configuration as bundle.
	ExportBundle(params.ExportBundleParams) (params.BundleData, error)
}

// bundleAPI implements the Bundle interface and is the concrete implementation
// of the API end point.
type bundleAPI struct {
	state common.ModelManagerBackend
}

// GetChanges returns the list of changes required to deploy the given bundle
// data. The changes are sorted by requirements, so that they can be applied in
// order.
func (b *bundleAPI) GetChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
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
	verifyDevices := func(s string) error {
		_, err := devices.ParseConstraints(s)
		return err
	}
	if err := data.Verify(verifyConstraints, verifyStorage, verifyDevices); err != nil {
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

// ExportBundle exports the model configuration specified in the argument as bundle.
func (b bundleAPI) ExportBundle(args params.ExportBundleParams) (params.BundleData, error) {
	result := params.BundleData{}

	tag, err := names.ParseModelTag(args.ModelTag)
	if err != nil {
		return result, errors.Annotate(err, "model-tag cannot be parsed.")
	}

	st, releaseSt, err := b.state.GetBackend(tag.Id())
	if err != nil {
		return result, errors.Trace(err)
	}
	defer releaseSt()

	var model description.Model
	model, err = st.Export()
	if err != nil {
		return result, errors.Trace(err)
	}

	// Fill in the BundleApplicationSpec of BundleData.
	app := make(map[string]*params.BundleApplicationSpec)

	for _, application := range model.Applications() {
		app[application.Name()].Charm = application.CharmURL()
		app[application.Name()].Series = application.Series()
		app[application.Name()].NumUnits = len(application.Units())
		//app[application.Name()].To              = application.To
		app[application.Name()].Expose = application.Exposed()
		app[application.Name()].Options = application.CharmConfig()
		app[application.Name()].Annotations = application.Annotations()

		app[application.Name()].EndpointBindings = application.EndpointBindings()
	}

	// Fill in the BundleMachineSpec of BundleData.
	mac := make(map[string]*params.BundleMachineSpec)

	for _, machine := range model.Machines() {
		constraints := []string{"arch=" + machine.Constraints().Architecture(),
			"cpu-cores=" + string(machine.Constraints().CpuCores()),
			"cpu-power=" + string(machine.Constraints().CpuPower()),
			"mem=" + string(machine.Constraints().Memory()),
			"root-disk=" + string(machine.Constraints().RootDisk())}
		mac[machine.Id()].Constraints = strings.Join(constraints, " ")
		mac[machine.Id()].Annotations = machine.Annotations()
		mac[machine.Id()].Series = machine.Series()
	}

	result = params.BundleData{
		Applications: app,
		Machines:     mac,
		Series:       series.LatestLts(),
	}
	return result, nil
}
