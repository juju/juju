// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Charms", 2, NewAPI)
}

var getState = func(st *state.State) charmsAccess {
	return stateShim{st}
}

// Charms defines the methods on the charms API end point.
type Charms interface {
	List(args params.CharmsList) (params.CharmsListResult, error)
	CharmInfo(args params.CharmURL) (params.CharmInfo, error)
	IsMetered(args params.CharmURL) (bool, error)
}

// API implements the charms interface and is the concrete
// implementation of the api end point.
type API struct {
	access     charmsAccess
	authorizer facade.Authorizer
}

func (a *API) checkCanRead() error {
	canRead, err := a.authorizer.HasPermission(description.ReadAccess, a.access.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return common.ErrPerm
	}
	return nil
}

// NewAPI returns a new charms API facade.
func NewAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {

	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		access:     getState(st),
		authorizer: authorizer,
	}, nil
}

// CharmInfo returns information about the requested charm.
// NOTE: thumper 2016-06-29, this is not a bulk call and probably should be.
func (a *API) CharmInfo(args params.CharmURL) (params.CharmInfo, error) {
	if err := a.checkCanRead(); err != nil {
		return params.CharmInfo{}, errors.Trace(err)
	}

	curl, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.CharmInfo{}, errors.Trace(err)
	}
	aCharm, err := a.access.Charm(curl)
	if err != nil {
		return params.CharmInfo{}, errors.Trace(err)
	}
	info := params.CharmInfo{
		Revision: aCharm.Revision(),
		URL:      curl.String(),
		Config:   convertCharmConfig(aCharm.Config()),
		Meta:     convertCharmMeta(aCharm.Meta()),
		Actions:  convertCharmActions(aCharm.Actions()),
		Metrics:  convertCharmMetrics(aCharm.Metrics()),
	}
	return info, nil
}

// List returns a list of charm URLs currently in the state.
// If supplied parameter contains any names, the result will be filtered
// to return only the charms with supplied names.
func (a *API) List(args params.CharmsList) (params.CharmsListResult, error) {
	if err := a.checkCanRead(); err != nil {
		return params.CharmsListResult{}, errors.Trace(err)
	}

	charms, err := a.access.AllCharms()
	if err != nil {
		return params.CharmsListResult{}, errors.Annotatef(err, " listing charms ")
	}

	names := set.NewStrings(args.Names...)
	checkName := !names.IsEmpty()
	charmURLs := []string{}
	for _, aCharm := range charms {
		charmURL := aCharm.URL()
		if checkName {
			if !names.Contains(charmURL.Name) {
				continue
			}
		}
		charmURLs = append(charmURLs, charmURL.String())
	}
	return params.CharmsListResult{CharmURLs: charmURLs}, nil
}

// IsMetered returns whether or not the charm is metered.
func (a *API) IsMetered(args params.CharmURL) (params.IsMeteredResult, error) {
	if err := a.checkCanRead(); err != nil {
		return params.IsMeteredResult{}, errors.Trace(err)
	}

	curl, err := charm.ParseURL(args.URL)
	if err != nil {
		return params.IsMeteredResult{false}, errors.Trace(err)
	}
	aCharm, err := a.access.Charm(curl)
	if err != nil {
		return params.IsMeteredResult{false}, errors.Trace(err)
	}
	if aCharm.Metrics() != nil && len(aCharm.Metrics().Metrics) > 0 {
		return params.IsMeteredResult{true}, nil
	}
	return params.IsMeteredResult{false}, nil
}

func convertCharmConfig(config *charm.Config) map[string]params.CharmOption {
	if config == nil {
		return nil
	}
	result := make(map[string]params.CharmOption)
	for key, value := range config.Options {
		result[key] = convertCharmOption(value)
	}
	return result
}

func convertCharmOption(opt charm.Option) params.CharmOption {
	return params.CharmOption{
		Type:        opt.Type,
		Description: opt.Description,
		Default:     opt.Default,
	}
}

func convertCharmMeta(meta *charm.Meta) *params.CharmMeta {
	if meta == nil {
		return nil
	}
	result := &params.CharmMeta{
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
		PayloadClasses: convertCharmPayloadClassMap(meta.PayloadClasses),
		Resources:      convertCharmResourceMetaMap(meta.Resources),
		Terms:          meta.Terms,
		MinJujuVersion: meta.MinJujuVersion.String(),
	}

	return result
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
	}
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
