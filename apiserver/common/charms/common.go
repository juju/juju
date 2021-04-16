// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
)

type State interface {
	Model() (*state.Model, error)
	Charm(curl *charm.URL) (*state.Charm, error)
}

type backend interface {
	Charm(curl *charm.URL) (*state.Charm, error)
	ModelTag() names.ModelTag
}

// CharmsAPI implements the charms interface and is the concrete
// implementation of the CharmsAPI end point.
type CharmsAPI struct {
	authorizer facade.Authorizer
	backend    backend
}

func (a *CharmsAPI) checkCanRead() error {
	if a.authorizer.AuthController() {
		return nil
	}
	canRead, err := a.authorizer.HasPermission(permission.ReadAccess, a.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

// NewCharmsAPI provides the signature required for facade registration.
func NewCharmsAPI(st State, authorizer facade.Authorizer) (*CharmsAPI, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &CharmsAPI{
		authorizer: authorizer,
		backend:    getState(st, m),
	}, nil
}

// TODO - CAAS(ericclaudejones): This should contain state alone, model will be
// removed once all relevant methods are moved from state to model.
type stateShim struct {
	State
	*state.Model
}

var getState = func(st State, m *state.Model) backend {
	return stateShim{st, m}
}

// CharmInfo returns information about the requested charm.
// NOTE: thumper 2016-06-29, this is not a bulk call and probably should be.
func (a *CharmsAPI) CharmInfo(args params.CharmURL) (params.Charm, error) {
	if err := a.checkCanRead(); err != nil {
		return params.Charm{}, errors.Trace(err)
	}

	curl, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}
	aCharm, err := a.backend.Charm(curl)
	if err != nil {
		return params.Charm{}, errors.Trace(err)
	}
	info := params.Charm{
		Revision: aCharm.Revision(),
		URL:      curl.String(),
		Config:   params.ToCharmOptionMap(aCharm.Config()),
		Meta:     convertCharmMeta(aCharm.Meta()),
		Actions:  convertCharmActions(aCharm.Actions()),
		Metrics:  convertCharmMetrics(aCharm.Metrics()),
		Manifest: convertCharmManifest(aCharm.Manifest()),
	}

	// we don't need to check that this is a charm.LXDProfiler, as we can
	// state that the function exists.
	if profile := aCharm.LXDProfile(); profile != nil && !profile.Empty() {
		info.LXDProfile = convertCharmLXDProfile(profile)
	}

	return info, nil
}

func convertCharmMeta(meta *charm.Meta) *params.CharmMeta {
	if meta == nil {
		return nil
	}
	return &params.CharmMeta{
		Name:           meta.Name,
		Summary:        meta.Summary,
		Description:    meta.Description,
		Subordinate:    meta.Subordinate,
		Provides:       convertCharmRelationMap(meta.Provides),
		Requires:       convertCharmRelationMap(meta.Requires),
		Peers:          convertCharmRelationMap(meta.Peers),
		ExtraBindings:  convertCharmExtraBindingMap(meta.ExtraBindings),
		Categories:     meta.Categories,
		Tags:           meta.Tags,
		Series:         meta.Series,
		Storage:        convertCharmStorageMap(meta.Storage),
		Deployment:     convertCharmDeployment(meta.Deployment),
		Devices:        convertCharmDevices(meta.Devices),
		PayloadClasses: convertCharmPayloadClassMap(meta.PayloadClasses),
		Resources:      convertCharmResourceMetaMap(meta.Resources),
		Terms:          meta.Terms,
		MinJujuVersion: meta.MinJujuVersion.String(),
		Containers:     convertCharmContainers(meta.Containers),
		Assumes:        meta.Assumes,
	}
}

func convertCharmManifest(manifest *charm.Manifest) *params.CharmManifest {
	if manifest == nil {
		return nil
	}
	return &params.CharmManifest{
		Bases: convertCharmBases(manifest.Bases),
	}
}

func convertCharmRelationMap(relations map[string]charm.Relation) map[string]params.CharmRelation {
	if len(relations) == 0 {
		return nil
	}
	result := make(map[string]params.CharmRelation)
	for key, value := range relations {
		result[key] = convertCharmRelation(value)
	}
	return result
}

func convertCharmRelation(relation charm.Relation) params.CharmRelation {
	return params.CharmRelation{
		Name:      relation.Name,
		Role:      string(relation.Role),
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Limit:     relation.Limit,
		Scope:     string(relation.Scope),
	}
}

func convertCharmStorageMap(storage map[string]charm.Storage) map[string]params.CharmStorage {
	if len(storage) == 0 {
		return nil
	}
	result := make(map[string]params.CharmStorage)
	for key, value := range storage {
		result[key] = convertCharmStorage(value)
	}
	return result
}

func convertCharmStorage(storage charm.Storage) params.CharmStorage {
	return params.CharmStorage{
		Name:        storage.Name,
		Description: storage.Description,
		Type:        string(storage.Type),
		Shared:      storage.Shared,
		ReadOnly:    storage.ReadOnly,
		CountMin:    storage.CountMin,
		CountMax:    storage.CountMax,
		MinimumSize: storage.MinimumSize,
		Location:    storage.Location,
		Properties:  storage.Properties,
	}
}

func convertCharmPayloadClassMap(payload map[string]charm.PayloadClass) map[string]params.CharmPayloadClass {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]params.CharmPayloadClass)
	for key, value := range payload {
		result[key] = convertCharmPayloadClass(value)
	}
	return result
}

func convertCharmPayloadClass(payload charm.PayloadClass) params.CharmPayloadClass {
	return params.CharmPayloadClass{
		Name: payload.Name,
		Type: payload.Type,
	}
}

func convertCharmResourceMetaMap(resources map[string]resource.Meta) map[string]params.CharmResourceMeta {
	if len(resources) == 0 {
		return nil
	}
	result := make(map[string]params.CharmResourceMeta)
	for key, value := range resources {
		result[key] = convertCharmResourceMeta(value)
	}
	return result
}

func convertCharmResourceMeta(meta resource.Meta) params.CharmResourceMeta {
	return params.CharmResourceMeta{
		Name:        meta.Name,
		Type:        meta.Type.String(),
		Path:        meta.Path,
		Description: meta.Description,
	}
}

func convertCharmActions(actions *charm.Actions) *params.CharmActions {
	if actions == nil {
		return nil
	}
	result := &params.CharmActions{
		ActionSpecs: convertCharmActionSpecMap(actions.ActionSpecs),
	}

	return result
}

func convertCharmActionSpecMap(specs map[string]charm.ActionSpec) map[string]params.CharmActionSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make(map[string]params.CharmActionSpec)
	for key, value := range specs {
		result[key] = convertCharmActionSpec(value)
	}
	return result
}

func convertCharmActionSpec(spec charm.ActionSpec) params.CharmActionSpec {
	return params.CharmActionSpec{
		Description: spec.Description,
		Params:      spec.Params,
	}
}

func convertCharmMetrics(metrics *charm.Metrics) *params.CharmMetrics {
	if metrics == nil {
		return nil
	}
	return &params.CharmMetrics{
		Metrics: convertCharmMetricMap(metrics.Metrics),
		Plan:    convertCharmPlan(metrics.Plan),
	}
}

func convertCharmPlan(plan *charm.Plan) params.CharmPlan {
	if plan == nil {
		return params.CharmPlan{Required: false}
	}
	return params.CharmPlan{Required: plan.Required}
}

func convertCharmMetricMap(metrics map[string]charm.Metric) map[string]params.CharmMetric {
	if len(metrics) == 0 {
		return nil
	}
	result := make(map[string]params.CharmMetric)
	for key, value := range metrics {
		result[key] = convertCharmMetric(value)
	}
	return result
}

func convertCharmMetric(metric charm.Metric) params.CharmMetric {
	return params.CharmMetric{
		Type:        string(metric.Type),
		Description: metric.Description,
	}
}

func convertCharmExtraBindingMap(bindings map[string]charm.ExtraBinding) map[string]string {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[string]string)
	for key, value := range bindings {
		result[key] = value.Name
	}
	return result
}

func convertCharmLXDProfile(profile *state.LXDProfile) *params.CharmLXDProfile {
	return &params.CharmLXDProfile{
		Description: profile.Description,
		Config:      convertCharmLXDProfileConfig(profile.Config),
		Devices:     convertCharmLXDProfileDevices(profile.Devices),
	}
}

func convertCharmLXDProfileConfig(config map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range config {
		result[k] = v
	}
	return result
}

func convertCharmLXDProfileDevices(devices map[string]map[string]string) map[string]map[string]string {
	result := map[string]map[string]string{}
	for k, v := range devices {
		nested := map[string]string{}
		for nk, nv := range v {
			nested[nk] = nv
		}
		result[k] = nested
	}
	return result
}

func convertCharmDeployment(deployment *charm.Deployment) *params.CharmDeployment {
	if deployment == nil {
		return nil
	}
	return &params.CharmDeployment{
		DeploymentType: string(deployment.DeploymentType),
		DeploymentMode: string(deployment.DeploymentMode),
		ServiceType:    string(deployment.ServiceType),
		MinVersion:     deployment.MinVersion,
	}
}

func convertCharmDevices(devices map[string]charm.Device) map[string]params.CharmDevice {
	if devices == nil {
		return nil
	}
	results := make(map[string]params.CharmDevice)
	for k, v := range devices {
		results[k] = params.CharmDevice{
			Name:        v.Name,
			Description: v.Description,
			Type:        string(v.Type),
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
		}
	}
	return results
}

func convertCharmBases(input []charm.Base) []params.CharmBase {
	var bases []params.CharmBase
	for _, v := range input {
		bases = append(bases, params.CharmBase{
			Name:          v.Name,
			Channel:       v.Channel.String(),
			Architectures: v.Architectures,
		})
	}
	return bases
}

func convertCharmContainers(input map[string]charm.Container) map[string]params.CharmContainer {
	containers := map[string]params.CharmContainer{}
	for k, v := range input {
		containers[k] = params.CharmContainer{
			Resource: v.Resource,
			Mounts:   convertCharmMounts(v.Mounts),
		}
	}
	if len(containers) == 0 {
		return nil
	}
	return containers
}

func convertCharmMounts(input []charm.Mount) []params.CharmMount {
	var mounts []params.CharmMount
	for _, v := range input {
		mounts = append(mounts, params.CharmMount{
			Storage:  v.Storage,
			Location: v.Location,
		})
	}
	return mounts
}
