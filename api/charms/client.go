// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// charms provides a client for accessing the charms API.
package charms

import (
	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	return &Client{ClientFacade: frontend, facade: backend}
}

// IsMetered returns whether or not the charm is metered.
func (c *Client) IsMetered(charmURL string) (bool, error) {
	args := params.CharmURL{URL: charmURL}
	var metered params.IsMeteredResult
	if err := c.facade.FacadeCall("IsMetered", args, &metered); err != nil {
		return false, errors.Trace(err)
	}
	return metered.Metered, nil
}

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision   int
	URL        string
	Config     *charm.Config
	Meta       *charm.Meta
	Actions    *charm.Actions
	Metrics    *charm.Metrics
	LXDProfile *charm.LXDProfile
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmURL{URL: charmURL}
	var info params.Charm
	if err := c.facade.FacadeCall("CharmInfo", args, &info); err != nil {
		return nil, errors.Trace(err)
	}
	meta, err := convertCharmMeta(info.Meta)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &CharmInfo{
		Revision:   info.Revision,
		URL:        info.URL,
		Config:     convertCharmConfig(info.Config),
		Meta:       meta,
		Actions:    convertCharmActions(info.Actions),
		Metrics:    convertCharmMetrics(info.Metrics),
		LXDProfile: convertCharmLXDProfile(info.LXDProfile),
	}
	return result, nil
}

func convertCharmConfig(config map[string]params.CharmOption) *charm.Config {
	if len(config) == 0 {
		return nil
	}
	result := &charm.Config{
		Options: make(map[string]charm.Option),
	}
	for key, value := range config {
		result.Options[key] = convertCharmOption(value)
	}
	return result
}

func convertCharmOption(opt params.CharmOption) charm.Option {
	return charm.Option{
		Type:        opt.Type,
		Description: opt.Description,
		Default:     opt.Default,
	}
}

func convertCharmMeta(meta *params.CharmMeta) (*charm.Meta, error) {
	if meta == nil {
		return nil, nil
	}
	minVersion, err := version.Parse(meta.MinJujuVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resources, err := convertCharmResourceMetaMap(meta.Resources)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := &charm.Meta{
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
		Resources:      resources,
		Terms:          meta.Terms,
		MinJujuVersion: minVersion,
	}
	return result, nil
}

func convertCharmRelationMap(relations map[string]params.CharmRelation) map[string]charm.Relation {
	if len(relations) == 0 {
		return nil
	}
	result := make(map[string]charm.Relation)
	for key, value := range relations {
		result[key] = convertCharmRelation(value)
	}
	return result
}

func convertCharmRelation(relation params.CharmRelation) charm.Relation {
	return charm.Relation{
		Name:      relation.Name,
		Role:      charm.RelationRole(relation.Role),
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Limit:     relation.Limit,
		Scope:     charm.RelationScope(relation.Scope),
	}
}

func convertCharmStorageMap(storage map[string]params.CharmStorage) map[string]charm.Storage {
	if len(storage) == 0 {
		return nil
	}
	result := make(map[string]charm.Storage)
	for key, value := range storage {
		result[key] = convertCharmStorage(value)
	}
	return result
}

func convertCharmStorage(storage params.CharmStorage) charm.Storage {
	return charm.Storage{
		Name:        storage.Name,
		Description: storage.Description,
		Type:        charm.StorageType(storage.Type),
		Shared:      storage.Shared,
		ReadOnly:    storage.ReadOnly,
		CountMin:    storage.CountMin,
		CountMax:    storage.CountMax,
		MinimumSize: storage.MinimumSize,
		Location:    storage.Location,
		Properties:  storage.Properties,
	}
}

func convertCharmPayloadClassMap(payload map[string]params.CharmPayloadClass) map[string]charm.PayloadClass {
	if len(payload) == 0 {
		return nil
	}
	result := make(map[string]charm.PayloadClass)
	for key, value := range payload {
		result[key] = convertCharmPayloadClass(value)
	}
	return result
}

func convertCharmPayloadClass(payload params.CharmPayloadClass) charm.PayloadClass {
	return charm.PayloadClass{
		Name: payload.Name,
		Type: payload.Type,
	}
}

func convertCharmResourceMetaMap(resources map[string]params.CharmResourceMeta) (map[string]resource.Meta, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	result := make(map[string]resource.Meta)
	for key, value := range resources {
		converted, err := convertCharmResourceMeta(value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[key] = converted
	}
	return result, nil
}

func convertCharmResourceMeta(meta params.CharmResourceMeta) (resource.Meta, error) {
	resourceType, err := resource.ParseType(meta.Type)
	if err != nil {
		return resource.Meta{}, errors.Trace(err)
	}
	return resource.Meta{
		Name:        meta.Name,
		Type:        resourceType,
		Path:        meta.Path,
		Description: meta.Description,
	}, nil
}

func convertCharmActions(actions *params.CharmActions) *charm.Actions {
	if actions == nil {
		return nil
	}
	return &charm.Actions{
		ActionSpecs: convertCharmActionSpecMap(actions.ActionSpecs),
	}
}

func convertCharmActionSpecMap(specs map[string]params.CharmActionSpec) map[string]charm.ActionSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make(map[string]charm.ActionSpec)
	for key, value := range specs {
		result[key] = convertCharmActionSpec(value)
	}
	return result
}

func convertCharmActionSpec(spec params.CharmActionSpec) charm.ActionSpec {
	return charm.ActionSpec{
		Description: spec.Description,
		Params:      spec.Params,
	}
}

func convertCharmMetrics(metrics *params.CharmMetrics) *charm.Metrics {
	if metrics == nil {
		return nil
	}
	return &charm.Metrics{
		Metrics: convertCharmMetricMap(metrics.Metrics),
		Plan:    convertCharmPlan(metrics.Plan),
	}
}

func convertCharmPlan(plan params.CharmPlan) *charm.Plan {
	return &charm.Plan{Required: plan.Required}
}

func convertCharmMetricMap(metrics map[string]params.CharmMetric) map[string]charm.Metric {
	if len(metrics) == 0 {
		return nil
	}
	result := make(map[string]charm.Metric)
	for key, value := range metrics {
		result[key] = convertCharmMetric(value)
	}
	return result
}

func convertCharmMetric(metric params.CharmMetric) charm.Metric {
	return charm.Metric{
		Type:        charm.MetricType(metric.Type),
		Description: metric.Description,
	}
}

func convertCharmExtraBindingMap(bindings map[string]string) map[string]charm.ExtraBinding {
	if len(bindings) == 0 {
		return nil
	}
	result := make(map[string]charm.ExtraBinding)
	for key, value := range bindings {
		result[key] = charm.ExtraBinding{value}
	}
	return result
}

func convertCharmLXDProfile(lxdProfile *params.CharmLXDProfile) *charm.LXDProfile {
	if lxdProfile == nil {
		return nil
	}
	return &charm.LXDProfile{
		Description: lxdProfile.Description,
		Config:      convertCharmLXDProfileConfigMap(lxdProfile.Config),
		Devices:     convertCharmLXDProfileDevicesMap(lxdProfile.Devices),
	}
}

func convertCharmLXDProfileConfigMap(config map[string]string) map[string]string {
	result := make(map[string]string, len(config))
	for k, v := range config {
		result[k] = v
	}
	return result
}

func convertCharmLXDProfileDevicesMap(devices map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string, len(devices))
	for k, v := range devices {
		nested := make(map[string]string, len(v))
		for nk, nv := range v {
			nested[nk] = nv
		}
		result[k] = nested
	}
	return result
}
