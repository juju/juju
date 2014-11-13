// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/juju/charm.v4"
)

// charmMeta is an intermediate field used to store charm.Meta data,
// decoupling the storage from the charm package.
type charmMeta struct {
	Name        string
	Summary     string
	Description string
	Subordinate bool
	Provides    map[string]charmRelation `bson:",omitempty"`
	Requires    map[string]charmRelation `bson:",omitempty"`
	Peers       map[string]charmRelation `bson:",omitempty"`
	Format      int                      `bson:",omitempty"`
	OldRevision int                      `bson:",omitempty"` // Obsolete
	Categories  []string                 `bson:",omitempty"`
	Tags        []string                 `bson:",omitempty"`
	Series      string                   `bson:",omitempty"`
}

func storeCharmMeta(original *charm.Meta) *charmMeta {
	if original == nil {
		return nil
	}
	var provides map[string]charmRelation
	if len(original.Provides) > 0 {
		provides = make(map[string]charmRelation, len(original.Provides))
		for key, rel := range original.Provides {
			provides[key] = storeCharmRelation(rel)
		}
	}
	var requires map[string]charmRelation
	if len(original.Requires) > 0 {
		requires = make(map[string]charmRelation, len(original.Requires))
		for key, rel := range original.Requires {
			requires[key] = storeCharmRelation(rel)
		}
	}
	var peers map[string]charmRelation
	if len(original.Peers) > 0 {
		peers = make(map[string]charmRelation, len(original.Peers))
		for key, rel := range original.Peers {
			peers[key] = storeCharmRelation(rel)
		}
	}
	return &charmMeta{
		Name:        original.Name,
		Summary:     original.Summary,
		Description: original.Description,
		Subordinate: original.Subordinate,
		Provides:    provides,
		Requires:    requires,
		Peers:       peers,
		Format:      original.Format,
		OldRevision: original.OldRevision,
		Categories:  original.Categories,
		Tags:        original.Tags,
		Series:      original.Series,
	}
}

// convert converts the intermediate structure to the original charm.Meta representation.
func (cm charmMeta) convert() *charm.Meta {
	var provides map[string]charm.Relation
	if len(cm.Provides) > 0 {
		provides = make(map[string]charm.Relation, len(cm.Provides))
		for key, rel := range cm.Provides {
			provides[key] = rel.convert()
		}
	}
	var requires map[string]charm.Relation
	if len(cm.Requires) > 0 {
		requires = make(map[string]charm.Relation, len(cm.Requires))
		for key, rel := range cm.Requires {
			requires[key] = rel.convert()
		}
	}
	var peers map[string]charm.Relation
	if len(cm.Peers) > 0 {
		peers = make(map[string]charm.Relation, len(cm.Peers))
		for key, rel := range cm.Peers {
			peers[key] = rel.convert()
		}
	}
	return &charm.Meta{
		Name:        cm.Name,
		Summary:     cm.Summary,
		Description: cm.Description,
		Subordinate: cm.Subordinate,
		Provides:    provides,
		Requires:    requires,
		Peers:       peers,
		Format:      cm.Format,
		OldRevision: cm.OldRevision,
		Categories:  cm.Categories,
		Tags:        cm.Tags,
		Series:      cm.Series,
	}
}

// charmMeta is an intermediate field used to store charm.Meta data,
// decoupling the storage from the charm package.
type charmRelation struct {
	Name      string
	Role      string
	Interface string
	Optional  bool
	Limit     int
	Scope     string
}

func storeCharmRelation(original charm.Relation) charmRelation {
	return charmRelation{
		Name:      original.Name,
		Role:      string(original.Role),
		Interface: original.Interface,
		Optional:  original.Optional,
		Limit:     original.Limit,
		Scope:     string(original.Scope),
	}
}

// convert converts the intermediate structure to the original charm.Relation structure.
func (cr charmRelation) convert() charm.Relation {
	return charm.Relation{
		Name:      cr.Name,
		Role:      charm.RelationRole(cr.Role),
		Interface: cr.Interface,
		Optional:  cr.Optional,
		Limit:     cr.Limit,
		Scope:     charm.RelationScope(cr.Scope),
	}
}

// charmConfig is an intermediate field used to store charm.Config data.
type charmConfig struct {
	Options map[string]charmOption
}

func storeCharmConfig(original *charm.Config) *charmConfig {
	if original == nil {
		return nil
	}

	options := make(map[string]charmOption, len(original.Options))
	for key, option := range original.Options {
		options[key] = storeCharmOption(option)
	}

	return &charmConfig{Options: options}
}

func (cc charmConfig) convert() *charm.Config {
	options := make(map[string]charm.Option, len(cc.Options))
	for key, option := range cc.Options {
		options[key] = option.convert()
	}

	return &charm.Config{Options: options}
}

// charmOption is an intermediate field used to store charm.Option data.
type charmOption struct {
	Type        string
	Description string
	Default     interface{}
}

func storeCharmOption(original charm.Option) charmOption {
	return charmOption{
		Type:        original.Type,
		Description: original.Description,
		Default:     original.Default,
	}
}

func (co charmOption) convert() charm.Option {
	return charm.Option{
		Type:        co.Type,
		Description: co.Description,
		Default:     co.Default,
	}
}

// charmActions is an intermediate structure for storing charm.Actions data.
type charmActions struct {
	ActionSpecs map[string]charmActionSpec `yaml:"actions,omitempty" bson:",omitempty"`
}

func storeCharmActions(original *charm.Actions) *charmActions {
	if original == nil {
		return nil
	}
	var actionSpecs map[string]charmActionSpec
	if original.ActionSpecs != nil {
		actionSpecs = make(map[string]charmActionSpec, len(original.ActionSpecs))
		for key, actionSpec := range original.ActionSpecs {
			actionSpecs[key] = storeCharmActionSpec(actionSpec)
		}
	}
	return &charmActions{ActionSpecs: actionSpecs}
}

func (ca *charmActions) convert() *charm.Actions {
	var actionSpecs map[string]charm.ActionSpec
	if ca.ActionSpecs != nil {
		actionSpecs = make(map[string]charm.ActionSpec, len(ca.ActionSpecs))
		for key, actionSpec := range ca.ActionSpecs {
			actionSpecs[key] = actionSpec.convert()
		}
	}
	return &charm.Actions{
		ActionSpecs: actionSpecs,
	}
}

// charmActionSpec is an intermediate structure for storing charm.ActionSpec data.
type charmActionSpec struct {
	Description string
	Params      map[string]interface{}
}

func storeCharmActionSpec(original charm.ActionSpec) charmActionSpec {
	params := make(map[string]interface{}, len(original.Params))
	for key, param := range original.Params {
		params[key] = param
	}
	return charmActionSpec{
		Description: original.Description,
		Params:      params,
	}
}

func (cas charmActionSpec) convert() charm.ActionSpec {
	params := make(map[string]interface{}, len(cas.Params))
	for key, param := range cas.Params {
		params[key] = param
	}
	return charm.ActionSpec{
		Description: cas.Description,
		Params:      params,
	}

}

// charmMetrics is an intermediate type for storing charm.Metrics data.
type charmMetrics struct {
	Metrics map[string]charmMetric
}

func storeCharmMetrics(original *charm.Metrics) *charmMetrics {
	if original == nil {
		return nil
	}
	var metrics map[string]charmMetric
	if original.Metrics != nil {
		metrics = make(map[string]charmMetric, len(original.Metrics))
		for key, metric := range original.Metrics {
			metrics[key] = storeCharmMetric(metric)
		}
	}
	return &charmMetrics{
		Metrics: metrics,
	}
}

func (cm charmMetrics) convert() *charm.Metrics {
	var metrics map[string]charm.Metric
	if cm.Metrics != nil {
		metrics = make(map[string]charm.Metric, len(cm.Metrics))
		for key, metric := range cm.Metrics {
			metrics[key] = metric.convert()
		}
	}
	return &charm.Metrics{
		Metrics: metrics,
	}

}

// charmMetric is an intermediate type for storing charm.Metric data.
type charmMetric struct {
	Type        string
	Description string
}

func storeCharmMetric(original charm.Metric) charmMetric {
	return charmMetric{
		Type:        string(original.Type),
		Description: original.Description,
	}
}

func (cm charmMetric) convert() charm.Metric {
	return charm.Metric{
		Type:        charm.MetricType(cm.Type),
		Description: cm.Description,
	}
}
