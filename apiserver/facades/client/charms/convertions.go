// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"

	"github.com/juju/juju/apiserver/params"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/state"
)

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
		PayloadClasses: convertCharmPayloadClassMap(meta.PayloadClasses),
		Resources:      convertCharmResourceMetaMap(meta.Resources),
		Terms:          meta.Terms,
		MinJujuVersion: meta.MinJujuVersion.String(),
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

func convertOrigin(origin corecharm.Origin) params.CharmOrigin {
	var track *string
	if origin.Channel != nil && origin.Channel.Track != "" {
		track = &origin.Channel.Track
	}
	var risk string
	if origin.Channel != nil {
		risk = string(origin.Channel.Risk)
	}
	return params.CharmOrigin{
		Source:   string(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Risk:     risk,
		Revision: origin.Revision,
		Track:    track,
	}
}

func convertParamsOrigin(origin params.CharmOrigin) corecharm.Origin {
	return corecharm.Origin{
		Source:   corecharm.Source(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel: &corecharm.Channel{
			Track: *origin.Track,
			Risk:  corecharm.Risk(origin.Risk),
		},
	}
}
