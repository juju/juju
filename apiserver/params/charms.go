// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// ApplicationCharmResults contains a set of ApplicationCharmResults.
type ApplicationCharmResults struct {
	Results []ApplicationCharmResult `json:"results"`
}

// ApplicationCharmResult contains an ApplicationCharm or an error.
type ApplicationCharmResult struct {
	Result *ApplicationCharm `json:"result,omitempty"`
	Error  *Error            `json:"error,omitempty"`
}

// ApplicationCharmInfo contains information about an
// application's charm.
type ApplicationCharm struct {
	// URL holds the URL of the charm assigned to the
	// application.
	URL string `json:"url"`

	// ForceUpgrade indicates whether or not application
	// units should upgrade to the charm even if they
	// are in an error state.
	ForceUpgrade bool `json:"force-upgrade,omitempty"`

	// SHA256 holds the SHA256 hash of the charm archive.
	SHA256 string `json:"sha256"`

	// CharmModifiedVersion increases when the charm changes in some way.
	CharmModifiedVersion int `json:"charm-modified-version"`
}

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

// CharmDevice mirrors charm.Device.
type CharmDevice struct {
	Name        string `bson:"name"`
	Description string `bson:"description"`
	Type        string `bson:"type"`
	CountMin    int64  `bson:"count-min"`
	CountMax    int64  `bson:"count-max"`
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
	Devices        map[string]CharmDevice       `json:"devices,omitempty"`
	PayloadClasses map[string]CharmPayloadClass `json:"payload-classes,omitempty"`
	Resources      map[string]CharmResourceMeta `json:"resources,omitempty"`
	Terms          []string                     `json:"terms,omitempty"`
	MinJujuVersion string                       `json:"min-juju-version,omitempty"`
}

// CharmInfo holds all the charm data that the client needs.
// To be honest, it probably returns way more than what is actually needed.
type CharmInfo struct {
	Revision   int                    `json:"revision"`
	URL        string                 `json:"url"`
	Config     map[string]CharmOption `json:"config"`
	Meta       *CharmMeta             `json:"meta,omitempty"`
	Actions    *CharmActions          `json:"actions,omitempty"`
	Metrics    *CharmMetrics          `json:"metrics,omitempty"`
	LXDProfile *CharmLXDProfile       `json:"lxd-profile,omitempty"`
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

// CharmLXDProfile mirrors charm.LXDProfile
type CharmLXDProfile struct {
	Config      map[string]string            `json:"config"`
	Description string                       `json:"description"`
	Devices     map[string]map[string]string `json:"devices"`
}

// CharmLXDProfileResult returns the result of finding the CharmLXDProfile
type CharmLXDProfileResult struct {
	LXDProfile *CharmLXDProfile `json:"lxd-profile"`
}

// ContainerLXDProfile contains the charm.LXDProfile information in addition to
// the name of the profile.
type ContainerLXDProfile struct {
	Profile CharmLXDProfile `json:"profile" yaml:"profile"`
	Name    string          `json:"name" yaml:"name"`
}

// ContainerProfileResult returns the result of finding the CharmLXDProfile and name of
// the lxd profile to be used for 1 unit on the container
type ContainerProfileResult struct {
	Error       *Error                `json:"error,omitempty"`
	LXDProfiles []ContainerLXDProfile `json:"lxd-profiles,omitempty"`
}

// ContainerProfileResults returns the ContainerProfileResult for each unit to be placed
// on the container.
type ContainerProfileResults struct {
	Results []ContainerProfileResult `json:"results"`
}
