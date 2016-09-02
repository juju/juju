// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CharmsList stores parameters for a charms.List call
type CharmsList struct {
	Names []string `json:"names"`
}

// CharmsListResult stores result from a charms.List call
type CharmsListResult struct {
	CharmURLs []string `json:"charm-urls"`
}

// IsMeteredResult stores result from a charms.IsMetered call
type IsMeteredResult struct {
	Metered bool `json:"metered"`
}

// CharmOption mirrors charm.Option
type CharmOption struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// CharmRelation mirrors charm.Relation.
type CharmRelation struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Interface string `json:"interface"`
	Optional  bool   `json:"optional"`
	Limit     int    `json:"limit"`
	Scope     string `json:"scope"`
}

// CharmStorage mirrors charm.Storage.
type CharmStorage struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Shared      bool     `json:"shared"`
	ReadOnly    bool     `json:"read-only"`
	CountMin    int      `json:"count-min"`
	CountMax    int      `json:"count-max"`
	MinimumSize uint64   `json:"minimum-size"`
	Location    string   `json:"location,omitempty"`
	Properties  []string `json:"properties,omitempty"`
}

// CharmPayloadClass mirrors charm.PayloadClass.
type CharmPayloadClass struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CharmResourceMeta mirrors charm.ResourceMeta.
type CharmResourceMeta struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// CharmMeta mirrors charm.Meta.
type CharmMeta struct {
	Name           string                       `json:"name"`
	Summary        string                       `json:"summary"`
	Description    string                       `json:"description"`
	Subordinate    bool                         `json:"subordinate"`
	Provides       map[string]CharmRelation     `json:"provides,omitempty"`
	Requires       map[string]CharmRelation     `json:"requires,omitempty"`
	Peers          map[string]CharmRelation     `json:"peers,omitempty"`
	ExtraBindings  map[string]string            `json:"extra-bindings,omitempty"`
	Categories     []string                     `json:"categories,omitempty"`
	Tags           []string                     `json:"tags,omitempty"`
	Series         []string                     `json:"series,omitempty"`
	Storage        map[string]CharmStorage      `json:"storage,omitempty"`
	PayloadClasses map[string]CharmPayloadClass `json:"payload-classes,omitempty"`
	Resources      map[string]CharmResourceMeta `json:"resources,omitempty"`
	Terms          []string                     `json:"terms,omitempty"`
	MinJujuVersion string                       `json:"min-juju-version,omitempty"`
}

// CharmInfo holds all the charm data that the client needs.
// To be honest, it probably returns way more than what is actually needed.
type CharmInfo struct {
	Revision int                    `json:"revision"`
	URL      string                 `json:"url"`
	Config   map[string]CharmOption `json:"config"`
	Meta     *CharmMeta             `json:"meta,omitempty"`
	Actions  *CharmActions          `json:"actions,omitempty"`
	Metrics  *CharmMetrics          `json:"metrics,omitempty"`
}

// CharmActions mirrors charm.Actions.
type CharmActions struct {
	ActionSpecs map[string]CharmActionSpec `json:"specs,omitempty"`
}

// CharmActionSpec mirrors charm.ActionSpec.
type CharmActionSpec struct {
	Description string                 `json:"description"`
	Params      map[string]interface{} `json:"params"`
}

// CharmMetric mirrors charm.Metric.
type CharmMetric struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// CharmPlan mirrors charm.Plan
type CharmPlan struct {
	Required bool `json:"required"`
}

// CharmMetrics mirrors charm.Metrics.
type CharmMetrics struct {
	Metrics map[string]CharmMetric `json:"metrics"`
	Plan    CharmPlan              `json:"plan"`
}
