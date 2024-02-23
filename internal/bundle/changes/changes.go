// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/charm"

	corebase "github.com/juju/juju/core/base"
)

// Logger defines the logging methods needed
type Logger interface {
	Tracef(string, ...interface{})
}

// ArchConstraint defines an architecture constraint. This is used to
// represent a parsed application architecture constraint.
type ArchConstraint interface {
	// Arch returns the arch from the constraint or an error satisfying
	// errors.IsNotFound if the constraint does not include an arch component.
	Arch() (string, error)
}

// ConstraintGetter represents a architecture constraint parser.
type ConstraintGetter func(string) ArchConstraint

// CharmResolver resolves the channel and revision of a charm from the list of
// parameters.
type CharmResolver func(charm string, base corebase.Base, channel, arch string, revision int) (string, int, error)

// ChangesConfig is used to provide the required data for determining changes.
type ChangesConfig struct {
	Bundle           *charm.BundleData
	Model            *Model
	Logger           Logger
	BundleURL        string
	ConstraintGetter ConstraintGetter
	CharmResolver    CharmResolver
	Force            bool
	// TODO: add charm metadata for validation.
}

// Validate attempts to validate the changes config, before usage.
func (c *ChangesConfig) Validate() error {
	if c.Bundle == nil {
		return errors.NotValidf("nil Bundle")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// FromData generates and returns the list of changes required to deploy the
// given bundle data. The changes are sorted by requirements, so that they can
// be applied in order. The bundle data is assumed to be already verified.
func FromData(config ChangesConfig) ([]Change, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	model := config.Model
	if model == nil {
		model = &Model{
			logger: config.Logger,
		}
	} else if model.logger == nil {
		model.logger = config.Logger
	}
	model.initializeSequence()
	model.InferMachineMap(config.Bundle)
	changes := &changeset{}
	resolver := resolver{
		bundle:           config.Bundle,
		model:            model,
		bundleURL:        config.BundleURL,
		logger:           config.Logger,
		constraintGetter: config.ConstraintGetter,
		charmResolver:    config.CharmResolver,
		changes:          changes,
		force:            config.Force,
	}
	addedApplications, err := resolver.handleApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var addedMachines map[string]*AddMachineChange
	if resolver.bundle.Type != Kubernetes {
		var err error
		addedMachines, err = resolver.handleMachines()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	deployedBundleApps := alreadyDeployedApplicationsFromBundle(model, config.Bundle.Applications)
	existingModelOffers := existingOffersFromModel(model)
	if addedApplications, err = resolver.handleOffers(addedApplications, deployedBundleApps, existingModelOffers); err != nil {
		return nil, err
	}
	resolver.handleRelations(addedApplications)
	if resolver.bundle.Type != Kubernetes {
		err := resolver.handleUnits(addedApplications, addedMachines)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return changes.sorted()
}

// alreadyDeployedApplicationsFromBundle returns a set consisting of the
// application names that are already known by the remote model and are also
// present in the provided application map obtained from the bundle that is
// being deployed.
func alreadyDeployedApplicationsFromBundle(ctrlModel *Model, bundleApps map[string]*charm.ApplicationSpec) set.Strings {
	var (
		deployedSet = set.NewStrings()
		bundleSet   = set.NewStrings()
	)

	for appName := range ctrlModel.Applications {
		deployedSet.Add(appName)
	}
	for appName := range bundleApps {
		bundleSet.Add(appName)
	}

	return deployedSet.Intersection(bundleSet)
}

func existingOffersFromModel(ctrlModel *Model) map[string]set.Strings {
	existingOffers := make(map[string]set.Strings)
	for _, app := range ctrlModel.Applications {
		if len(app.Offers) == 0 {
			continue
		}

		existingOffers[app.Name] = set.NewStrings(app.Offers...)
	}

	return existingOffers
}

// Change holds a single change required to deploy a bundle.
type Change interface {
	// Id returns the unique identifier for this change.
	Id() string
	// Requires returns the ids of all the changes that must
	// be applied before this one.
	Requires() []string
	// Method returns the action to be performed to apply this change.
	Method() string
	// Description returns a human readable, potentially multi-line summary
	// of the change.
	Description() []string
	// Args returns a map of arguments that are named.
	Args() (map[string]interface{}, error)
	// setId is used to set the identifier for the change.
	setId(string)
}

// changeInfo holds information on a change, suitable for embedding into a more
// specific change type.
type changeInfo struct {
	id       string
	requires []string
	method   string
}

// Id implements Change.Id.
func (ch *changeInfo) Id() string {
	return ch.id
}

// Requires implements Change.Requires.
func (ch *changeInfo) Requires() []string {
	// Avoid returning a nil interface because so that avoid returning a slice
	// that will serialize to JSON null.
	if ch.requires == nil {
		return []string{}
	}
	return ch.requires
}

// Method implements Change.Method.
func (ch *changeInfo) Method() string {
	return ch.method
}

// setId implements Change.setId.
func (ch *changeInfo) setId(id string) {
	ch.id = id
}

// newAddCharmChange creates a new change for adding a charm.
func newAddCharmChange(params AddCharmParams, requires ...string) *AddCharmChange {
	return &AddCharmChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "addCharm",
		},
		Params: params,
	}
}

// AddCharmChange holds a change for adding a charm to the environment.
type AddCharmChange struct {
	changeInfo
	// Params holds parameters for adding a charm.
	Params AddCharmParams
}

// Args implements Change.Args.
func (ch *AddCharmChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *AddCharmChange) Description() []string {
	p := ch.Params
	var base, channel string
	if p.Base != "" {
		base = " for base " + p.Base
	}
	if p.Revision != nil && *p.Revision >= 0 {
		channel = fmt.Sprintf(" with revision %d", *p.Revision)
	} else if p.Channel != "" {
		channel = " from channel " + p.Channel
	}

	// If we fail when parsing the url, we can consider it being a name rather
	// than a URL.
	var location string
	name := p.Charm
	if curl, err := charm.ParseURL(name); err == nil {
		name = curl.Name

		location = storeLocation(curl.Schema)
		if location != "" {
			location = fmt.Sprintf(" from %s", location)
		}
	}
	var arch string
	if p.Architecture != "" {
		arch = fmt.Sprintf(" with architecture=%s", p.Architecture)
	}

	return []string{fmt.Sprintf("upload charm %s%s%s%s%s", name, location, base, channel, arch)}
}

// AddCharmParams holds parameters for adding a charm to the environment.
type AddCharmParams struct {
	// Charm holds the URL of the charm to be added.
	Charm string `json:"charm"`
	// Revision holds the revision of the charm to be added.
	Revision *int `json:"revision,omitempty"`
	// Base holds the base of the charm to be added
	// if the charm default is not sufficient.
	Base string `json:"base,omitempty"`
	// Channel holds the preferred channel for obtaining the charm.
	// Channel was added to 2.7 release, use omitempty so we're backwards
	// compatible with older clients.
	Channel string `json:"channel,omitempty"`
	// Architecture holds the preferred charm architecture to deploy the
	// application with.
	Architecture string `json:"architecture,omitempty"`
}

// newUpgradeCharm upgrades an existing charm to a new version.
func newUpgradeCharm(params UpgradeCharmParams, requires ...string) *UpgradeCharmChange {
	return &UpgradeCharmChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "upgradeCharm",
		},
		Params: params,
	}
}

// UpgradeCharmChange holds a change for adding a charm to the environment.
type UpgradeCharmChange struct {
	changeInfo
	// Params holds parameters for upgrading the charm for an application.
	Params UpgradeCharmParams
}

// Args implements Change.Args.
func (ch *UpgradeCharmChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *UpgradeCharmChange) Description() []string {
	var base string
	if ch.Params.Base != "" {
		base = " for base " + ch.Params.Base
	}
	var channel string
	if ch.Params.Channel != "" {
		channel = " from channel " + ch.Params.Channel
	}

	var location string
	name := ch.Params.charmURL
	curl, err := charm.ParseURL(ch.Params.charmURL)
	if err == nil {
		name = curl.Name

		location = storeLocation(curl.Schema)
		if location != "" {
			location = fmt.Sprintf(" from %s ", location)
		}
	}
	return []string{fmt.Sprintf("upgrade %s%susing charm %s%s%s", ch.Params.Application, location, name, base, channel)}
}

// UpgradeCharmParams holds parameters for adding a charm to the environment.
type UpgradeCharmParams struct {
	// Charm holds the placeholder or URL of the charm to be added.
	Charm string `json:"charm"`
	// Application refers to the application that is being upgraded.
	Application string `json:"application"`
	// Base holds the base of the charm to be added
	// if the charm default is not sufficient.
	Base string `json:"base"`
	// Resources identifies the revision to use for each resource
	// of the application's charm.
	Resources map[string]int `json:"resources,omitempty"`
	// LocalResources identifies the path to the local resource
	// of the application's charm.
	LocalResources map[string]string `json:"local-resources,omitempty"`
	// Channel holds the preferred channel for obtaining the charm.
	Channel string `json:"channel,omitempty"`

	charmURL string
}

// newAddMachineChange creates a new change for adding a machine or container.
func newAddMachineChange(params AddMachineParams, requires ...string) *AddMachineChange {
	return &AddMachineChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "addMachines",
		},
		Params: params,
	}
}

// AddMachineChange holds a change for adding a machine or container.
type AddMachineChange struct {
	changeInfo
	// Params holds parameters for adding a machine.
	Params AddMachineParams
}

// Args implements Change.Args.
func (ch *AddMachineChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *AddMachineChange) Description() []string {
	machine := "new machine"
	if ch.Params.existing {
		machine = "existing machine"
	}
	machine += " " + ch.Params.machineID
	if ch.Params.bundleMachineID != "" && ch.Params.bundleMachineID != ch.Params.machineID {
		machine += " (bundle machine " + ch.Params.bundleMachineID + ")"
	}

	if ch.Params.ContainerType != "" {
		machine = ch.Params.ContainerType + " container " + ch.Params.containerMachineID + " on " + machine
	}
	return []string{fmt.Sprintf("add %s", machine)}
}

// AddMachineOptions holds options for adding a machine or container.
type AddMachineOptions struct {
	// Base holds the machine OS base.
	Base string `json:"base,omitempty"`
	// Constraints holds the machine constraints.
	Constraints string `json:"constraints,omitempty"`
	// ContainerType holds the machine container type (like "lxc" or "kvm").
	ContainerType string `json:"containerType,omitempty"`
	// ParentId holds the id of the parent machine.
	ParentId string `json:"parentId,omitempty"`
}

// AddMachineParams holds parameters for adding a machine or container.
type AddMachineParams struct {
	// Base holds the optional machine OS base.
	Base string `json:"base,omitempty"`
	// Constraints holds the optional machine constraints.
	Constraints string `json:"constraints,omitempty"`
	// ContainerType optionally holds the type of the container (for instance
	// ""lxc" or kvm"). It is not specified for top level machines.
	ContainerType string `json:"container-type,omitempty"`
	// ParentId optionally holds a placeholder pointing to another machine
	// change or to a unit change. This value is only specified in the case
	// this machine is a container, in which case also ContainerType is set.
	ParentId string `json:"parent-id,omitempty"`

	existing           bool
	bundleMachineID    string
	machineID          string
	containerMachineID string
}

// Machine returns the machine for these params, either the container ID,
// or the machine ID if it doesn't exist.
func (a AddMachineParams) Machine() string {
	if a.containerMachineID == "" {
		return a.machineID
	}
	return a.containerMachineID
}

// newAddRelationChange creates a new change for adding a relation.
func newAddRelationChange(params AddRelationParams, requires ...string) *AddRelationChange {
	return &AddRelationChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "addRelation",
		},
		Params: params,
	}
}

// AddRelationChange holds a change for adding a relation between two applications.
type AddRelationChange struct {
	changeInfo
	// Params holds parameters for adding a relation.
	Params AddRelationParams
}

// Args implements Change.Args.
func (ch *AddRelationChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *AddRelationChange) Description() []string {
	return []string{fmt.Sprintf("add relation %s - %s", ch.Params.applicationEndpoint1, ch.Params.applicationEndpoint2)}
}

// AddRelationParams holds parameters for adding a relation between two applications.
type AddRelationParams struct {
	// Endpoint1 and Endpoint2 hold relation endpoints in the
	// "application:interface" form, where the application is either a
	// placeholder pointing to an application change or in the case of a model
	// that already has this application deployed, the name of the
	// application, and the interface is optional. Examples are
	// "$deploy-42:web", "$deploy-42", "mysql:db".
	Endpoint1 string `json:"endpoint1"`
	Endpoint2 string `json:"endpoint2"`

	// These values are always referring to application names.
	applicationEndpoint1 string
	applicationEndpoint2 string
}

// newAddApplicationChange creates a new change for adding an application.
func newAddApplicationChange(params AddApplicationParams, requires ...string) *AddApplicationChange {
	return &AddApplicationChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "deploy",
		},
		Params: params,
	}
}

// AddApplicationChange holds a change for deploying a Juju application.
type AddApplicationChange struct {
	changeInfo
	// Params holds parameters for adding an application.
	Params AddApplicationParams
}

// Args implements Change.Args.
func (ch *AddApplicationChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *AddApplicationChange) Description() []string {
	var base string
	if ch.Params.Base != "" {
		base = " on " + ch.Params.Base
	}
	var channel string
	if ch.Params.Channel != "" {
		channel = " with " + ch.Params.Channel
	}
	var unitsInfo string
	if ch.Params.NumUnits > 0 {
		plural := ""
		if ch.Params.NumUnits > 1 {
			plural = "s"
		}
		unitsInfo = fmt.Sprintf(" with %d unit%s", ch.Params.NumUnits, plural)
	}

	var using string
	var location string
	curl, err := charm.ParseURL(ch.Params.charmURL)
	if err == nil {
		location = storeLocation(curl.Schema)
		if location != "" {
			location = fmt.Sprintf(" from %s", location)
		}
		if ch.Params.Application != curl.Name {
			using = fmt.Sprintf(" using %s", curl.Name)
		}
	}

	return []string{fmt.Sprintf("deploy application %s%s%s%s%s%s", ch.Params.Application, location, unitsInfo, base, channel, using)}
}

// AddApplicationParams holds parameters for deploying a Juju application.
type AddApplicationParams struct {
	// Charm holds the URL of the charm to be used to deploy this application.
	Charm string `json:"charm"`
	// Base holds the base of the application to be deployed
	// if the charm default is not sufficient.
	Base string `json:"base,omitempty"`
	// Application holds the application name.
	Application string `json:"application,omitempty"`
	// NumUnits holds the number of units required.
	// For IAAS models, this will be 0 and separate AddUnitChanges will be used.
	// For Kubernetes models, this will be used to scale the application.
	NumUnits int `json:"num-units,omitempty"`
	// Options holds application options.
	Options map[string]interface{} `json:"options,omitempty"`
	// Constraints holds the optional application constraints.
	Constraints string `json:"constraints,omitempty"`
	// Storage holds the optional storage constraints.
	Storage map[string]string `json:"storage,omitempty"`
	// Devices holds the optional devices constraints.
	Devices map[string]string `json:"devices,omitempty"`
	// EndpointBindings holds the optional endpoint bindings
	EndpointBindings map[string]string `json:"endpoint-bindings,omitempty"`
	// Resources identifies the revision to use for each resource
	// of the application's charm.
	Resources map[string]int `json:"resources,omitempty"`
	// LocalResources identifies the path to the local resource
	// of the application's charm.
	LocalResources map[string]string `json:"local-resources,omitempty"`
	// Channel holds the channel of the application to be deployed.
	Channel string `json:"channel,omitempty"`

	// The public Charm holds either the charmURL of a placeholder for the
	// add charm change.
	charmURL string
}

// newAddUnitChange creates a new change for adding an application unit.
func newAddUnitChange(params AddUnitParams, requires ...string) *AddUnitChange {
	return &AddUnitChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "addUnit",
		},
		Params: params,
	}
}

// AddUnitChange holds a change for adding an application unit.
type AddUnitChange struct {
	changeInfo
	// Params holds parameters for adding a unit.
	Params AddUnitParams
}

// Args implements Change.Args.
func (ch *AddUnitChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *AddUnitChange) Description() []string {
	placement := "new machine"
	if ch.Params.baseMachine != "" {
		placement = placement + " " + ch.Params.baseMachine
	}
	if ch.Params.placementDescription != "" {
		placement = ch.Params.placementDescription
	}
	if ch.Params.directive != "" {
		placement += " to satisfy [" + ch.Params.directive + "]"
	}

	return []string{fmt.Sprintf("add unit %s to %s", ch.Params.unitName, placement)}
}

// AddUnitParams holds parameters for adding an application unit.
type AddUnitParams struct {
	// Application holds the application placeholder name for which a unit is added.
	Application string `json:"application"`
	// To holds the optional location where to add the unit, as a placeholder
	// pointing to another unit change or to a machine change.
	To string `json:"to,omitempty"`

	unitName             string
	placementDescription string
	// If directive is specified, it is added to the placement description
	// to explain why the unit is being placed there.
	directive   string
	baseMachine string
}

// Unit returns the unit name for these params.
func (a AddUnitParams) Unit() string {
	return a.unitName
}

// PlacementDescription returns the placement description for these params.
func (a AddUnitParams) PlacementDescription() string {
	return a.placementDescription
}

// BaseMachine returns the base machine for these params.
func (a AddUnitParams) BaseMachine() string {
	return a.baseMachine
}

// newExposeChange creates a new change for exposing an application.
func newExposeChange(params ExposeParams, requires ...string) *ExposeChange {
	return &ExposeChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "expose",
		},
		Params: params,
	}
}

// ExposeChange holds a change for exposing an application.
type ExposeChange struct {
	changeInfo
	// Params holds parameters for exposing an application.
	Params ExposeParams
}

// Args implements Change.Args.
func (ch *ExposeChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *ExposeChange) Description() []string {
	// Easy case: all application gets exposed and no endpoint-specific
	// parameters are provided.
	if len(ch.Params.ExposedEndpoints) == 0 {
		return []string{fmt.Sprintf("expose all endpoints of %s and allow access from CIDRs 0.0.0.0/0 and ::/0", ch.Params.appName)}
	}

	var (
		descr  bytes.Buffer
		output []string
	)
	for _, exposedGroup := range groupByExposedEndpointParams(ch.Params.ExposedEndpoints) {
		if exposedGroup.endpointNames[0] == "" {
			fmt.Fprint(&descr, "expose all endpoints")
		} else {
			plural := ""
			if len(exposedGroup.endpointNames) > 1 {
				plural = "s"
			}
			fmt.Fprintf(&descr, "override expose settings for endpoint%s %s", plural, strings.Join(exposedGroup.endpointNames, ","))
		}

		fmt.Fprintf(&descr, " of %s and allow access from ", ch.Params.appName)

		if spaceCount, cidrCount := len(exposedGroup.params.ExposeToSpaces), len(exposedGroup.params.ExposeToCIDRs); spaceCount+cidrCount != 0 {
			if spaceCount != 0 {
				plural := ""
				if spaceCount > 1 {
					plural = "s"
				}
				sort.Strings(exposedGroup.params.ExposeToSpaces)
				fmt.Fprintf(&descr, "space%s %s", plural, strings.Join(exposedGroup.params.ExposeToSpaces, ","))
			}

			if spaceCount != 0 && cidrCount != 0 {
				fmt.Fprint(&descr, " and ")
			}

			if cidrCount != 0 {
				plural := ""
				if cidrCount > 1 {
					plural = "s"
				}

				sort.Strings(exposedGroup.params.ExposeToCIDRs)
				fmt.Fprintf(&descr, "CIDR%s %s", plural, strings.Join(exposedGroup.params.ExposeToCIDRs, ","))
			}
		} else {
			fmt.Fprint(&descr, "CIDRs 0.0.0.0/0 and ::/0")
		}

		output = append(output, descr.String())
		descr.Reset()
	}
	return output
}

type exposedEndpointGroup struct {
	endpointNames []string
	params        *ExposedEndpointParams
}

// groupByExposedEndpointParams groups together any endpoints that have the same
// set of exposed endpoint parameters and returns them as a slice of
// exposedEndpointGroup entries sorted by endpoint name.
func groupByExposedEndpointParams(exposedEndpoints map[string]*ExposedEndpointParams) []exposedEndpointGroup {
	groups := make(map[*ExposedEndpointParams][]string)

nextEndpoint:
	for epName, expDetails := range exposedEndpoints {
		if epName == "" {
			continue
		}

		for grpKey := range groups {
			if grpKey.equalTo(expDetails) {
				groups[grpKey] = append(groups[grpKey], epName)
				continue nextEndpoint
			}
		}

		groups[expDetails] = []string{epName}
	}

	// Add entry for wildcard endpoint (if present)
	if expDetails, found := exposedEndpoints[""]; found {
		groups[expDetails] = []string{""}
	}

	// Ensure all endpoints are sorted and convert into a list
	groupList := make([]exposedEndpointGroup, 0, len(groups))
	for expDetails, epList := range groups {
		sort.Strings(epList)
		groupList = append(groupList, exposedEndpointGroup{
			endpointNames: epList,
			params:        expDetails,
		})
	}

	sort.Slice(groupList, func(i, j int) bool {
		// Each list entry has at least one endpoint name and the ones
		// with multiple names are pre-sorted by name.
		return groupList[i].endpointNames[0] < groupList[j].endpointNames[0]
	})

	return groupList
}

// ExposeParams holds parameters for exposing an application.
type ExposeParams struct {
	// Application holds the placeholder name of the application that must be exposed.
	Application string `json:"application"`

	// ExposedEndpoints stores a subset of the application endpoints that
	// are used to select the set of open ports that should be accessible
	// if the application is exposed. An empty value indicates that all
	// open ports should be made accessible.
	ExposedEndpoints map[string]*ExposedEndpointParams `json:"exposed-endpoints,omitempty"`

	appName        string
	alreadyExposed bool
}

// ExposedEndpointParams encapsulates the expose-related parameters for a
// particular endpoint.
type ExposedEndpointParams struct {
	// ExposeToSpaces contains a list of spaces that should be able to
	// access the application ports if the application is exposed.
	ExposeToSpaces []string `json:"expose-to-spaces,omitempty"`

	// ExposeToCIDRs contains a list of CIDRs that should be able to
	// access the application ports if the application is exposed.
	ExposeToCIDRs []string `json:"expose-to-cidrs,omitempty"`
}

func (exp *ExposedEndpointParams) equalTo(other *ExposedEndpointParams) bool {
	return equalStringSlices(exp.ExposeToSpaces, other.ExposeToSpaces) &&
		equalStringSlices(exp.ExposeToCIDRs, other.ExposeToCIDRs)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	setA := set.NewStrings(a...)
	setB := set.NewStrings(b...)
	return setA.Difference(setB).IsEmpty()
}

// newScaleChange creates a new change for scaling a Kubernetes application.
func newScaleChange(params ScaleParams, requires ...string) *ScaleChange {
	return &ScaleChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "scale",
		},
		Params: params,
	}
}

// ScaleChange holds a change for scaling an application.
type ScaleChange struct {
	changeInfo
	// Params holds parameters for scaling an application.
	Params ScaleParams
}

// Args implements Change.Args.
func (ch *ScaleChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *ScaleChange) Description() []string {
	return []string{fmt.Sprintf("scale %s to %d units", ch.Params.appName, ch.Params.Scale)}
}

// ScaleParams holds parameters for scaling an application.
type ScaleParams struct {
	// Application holds the placeholder name of the application to be scaled.
	Application string `json:"application"`

	// Scale is the new scale value to use.
	Scale int `json:"scale"`

	appName string
}

// newSetAnnotationsChange creates a new change for setting annotations.
func newSetAnnotationsChange(params SetAnnotationsParams, requires ...string) *SetAnnotationsChange {
	return &SetAnnotationsChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "setAnnotations",
		},
		Params: params,
	}
}

// SetAnnotationsChange holds a change for setting application and machine
// annotations.
type SetAnnotationsChange struct {
	changeInfo
	// Params holds parameters for setting annotations.
	Params SetAnnotationsParams
}

// Args implements Change.Args.
func (ch *SetAnnotationsChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *SetAnnotationsChange) Description() []string {
	return []string{fmt.Sprintf("set annotations for %s", ch.Params.target)}
}

// EntityType holds entity types ("application" or "machine").
type EntityType string

const (
	ApplicationType EntityType = "application"
	MachineType     EntityType = "machine"
)

// SetAnnotationsParams holds parameters for setting annotations.
type SetAnnotationsParams struct {
	// Id is the placeholder for the application or machine change corresponding to
	// the entity to be annotated.
	Id string `json:"id"`
	// EntityType holds the type of the entity, "application" or "machine".
	EntityType EntityType `json:"entity-type"`
	// Annotations holds the annotations as key/value pairs.
	Annotations map[string]string `json:"annotations"`

	target string
}

// newSetOptionsChange creates a new change for setting application options.
func newSetOptionsChange(params SetOptionsParams, requires ...string) *SetOptionsChange {
	return &SetOptionsChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "setOptions",
		},
		Params: params,
	}
}

// SetOptionsChange holds a change for setting application options.
type SetOptionsChange struct {
	changeInfo
	// Params holds parameters for setting options.
	Params SetOptionsParams
}

// Args implements Change.Args.
func (ch *SetOptionsChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *SetOptionsChange) Description() []string {
	return []string{fmt.Sprintf("set application options for %s", ch.Params.Application)}
}

// SetOptionsParams holds parameters for setting options.
type SetOptionsParams struct {
	// Application is the name of the application.
	Application string `json:"application"`
	// Options holds the changed options for the application.
	Options map[string]interface{} `json:"options,omitempty"`
}

// newSetConstraintsChange creates a new change for setting application constraints.
func newSetConstraintsChange(params SetConstraintsParams) *SetConstraintsChange {
	return &SetConstraintsChange{
		changeInfo: changeInfo{
			method: "setConstraints",
		},
		Params: params,
	}
}

// SetConstraintsChange holds a change for setting application constraints.
type SetConstraintsChange struct {
	changeInfo
	// Params holds parameters for setting constraints.
	Params SetConstraintsParams
}

// Args implements Change.Args.
func (ch *SetConstraintsChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *SetConstraintsChange) Description() []string {
	return []string{fmt.Sprintf("set constraints for %s to %q", ch.Params.Application, ch.Params.Constraints)}
}

// SetConstraintsParams holds parameters for setting constraints.
type SetConstraintsParams struct {
	// Application is the name of the application.
	Application string `json:"application"`
	// Constraints holds the new constraints.
	Constraints string `json:"constraints,omitempty"`
}

// CreateOfferChange holds a change for creating a new application endpoint offer.
type CreateOfferChange struct {
	changeInfo
	// Params holds parameters for creating an offer.
	Params CreateOfferParams
}

// newCreateOfferChange creates a new change for creating an offer.
func newCreateOfferChange(params CreateOfferParams, requires ...string) *CreateOfferChange {
	return &CreateOfferChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "createOffer",
		},
		Params: params,
	}
}

// Args implements Change.Args.
func (ch *CreateOfferChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *CreateOfferChange) Description() []string {
	verb := "create"
	if ch.Params.Update {
		verb = "update"
	}
	return []string{fmt.Sprintf("%s offer %s using %s:%s", verb, ch.Params.OfferName, ch.Params.Application, strings.Join(ch.Params.Endpoints, ","))}
}

// CreateOfferParams holds parameters for creating an application offer.
type CreateOfferParams struct {
	// Application is the name of the application to create an offer for.
	Application string `json:"application"`
	// Endpoint is a list of application endpoint to expose as part of an offer.
	Endpoints []string `json:"endpoints"`
	// OfferName describes the offer name.
	OfferName string `json:"offer-name,omitempty"`
	// Update is set to true if an existing offer is to be updated.
	Update bool `json:"update,omitempty"`
}

// ConsumeOfferChange holds a change for consuming a offer.
type ConsumeOfferChange struct {
	changeInfo
	// Params holds the parameters for consuming an offer.
	Params ConsumeOfferParams
}

func newConsumeOfferChange(params ConsumeOfferParams, requires ...string) *ConsumeOfferChange {
	return &ConsumeOfferChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "consumeOffer",
		},
		Params: params,
	}
}

// Args implements Change.Args.
func (ch *ConsumeOfferChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *ConsumeOfferChange) Description() []string {
	return []string{fmt.Sprintf("consume offer %s at %s", ch.Params.ApplicationName, ch.Params.URL)}
}

// ConsumeOfferParams holds the parameters for consuming an offer.
type ConsumeOfferParams struct {
	// URL contains the location of the offer
	URL string `json:"url"`
	// ApplicationName describes the application name on offer.
	ApplicationName string `json:"application-name,omitempty"`
}

// GrantOfferAccessChange holds a change for granting a user access to an offer.
type GrantOfferAccessChange struct {
	changeInfo
	// Params holds the parameters for the grant.
	Params GrantOfferAccessParams
}

func newGrantOfferAccessChange(params GrantOfferAccessParams, requires ...string) *GrantOfferAccessChange {
	return &GrantOfferAccessChange{
		changeInfo: changeInfo{
			requires: requires,
			method:   "grantOfferAccess",
		},
		Params: params,
	}
}

// Args implements Change.Args.
func (ch *GrantOfferAccessChange) Args() (map[string]interface{}, error) {
	return paramsToArgs(ch.Params)
}

// Description implements Change.
func (ch *GrantOfferAccessChange) Description() []string {
	return []string{fmt.Sprintf("grant user %s %s access to offer %s", ch.Params.User, ch.Params.Access, ch.Params.Offer)}
}

func paramsToArgs(params interface{}) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}
	bytes, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// GrantOfferAccessParams holds the parameters for granting access to a user.
type GrantOfferAccessParams struct {
	// User holds the user name to grant access to.
	User string `json:"user"`
	// The type of access to grant.
	Access string `json:"access"`
	// The offer name to be granted access to.
	Offer string `json:"offer"`
}

// changeset holds the list of changes returned by FromData.
type changeset struct {
	changes []Change
}

// add adds the given change to this change set.
func (cs *changeset) add(change Change) {
	change.setId(fmt.Sprintf("%s-%d", change.Method(), len(cs.changes)))
	cs.changes = append(cs.changes, change)
}

// dependents returns a map of change-id -> changes that depend on
// it. We can't calculate this as changes are added because in some
// cases a change's requirements are updated after it's added to the
// changeset.
func (cs *changeset) dependents() map[string][]string {
	result := make(map[string][]string)
	for _, change := range cs.changes {
		for _, dep := range change.Requires() {
			result[dep] = append(result[dep], change.Id())
		}
	}
	return result
}

// sorted returns the changes sorted by requirements, required first.
func (cs *changeset) sorted() ([]Change, error) {
	// Exit it early if there is nothing to sort.
	if len(cs.changes) == 0 {
		return nil, nil
	}

	// Create a map to sort the data.
	data := make(map[string][]string, len(cs.changes))
	dataOrder := make([]string, len(cs.changes))
	for i, c := range cs.changes {
		// Sorting the requirements can be removed if the place in
		// handlers.go where the order of requirements is not
		// idempotent can be found and fixed.
		sort.Sort(sort.StringSlice(c.Requires()))
		data[c.Id()] = c.Requires()
		dataOrder[i] = c.Id()
	}
	sortedChangeIDs, err := toposortFlatten(dataOrder, data)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// convert the sorted change IDs to a sorted slice
	// of changes
	var sortedChanges []Change
	for _, changeID := range sortedChangeIDs {
		for _, change := range cs.changes {
			if changeID == change.Id() {
				sortedChanges = append(sortedChanges, change)
				break
			}
		}
	}
	return sortedChanges, nil
}

// toposortFlatten performs a stable topological sort on the provided
// data.  dataOrder is a slice of the originally ordered changes. data is a
// map of change to requirements.  To be idempotent, requirements must be in
// the same order each time.  Use along with data to ensure that this method
// is deterministic.
func toposortFlatten(dataOrder []string, data map[string][]string) ([]string, error) {

	// inDegree tracks the in-degree of each vertex `ch`
	// i.e. the number of changes that must be done before `ch`.
	inDegree := make(map[string]int, len(dataOrder))
	// followers is the inverse of data: for each change `r`, followers[ch] is
	// the list of all changes which require `r`.
	followers := make(map[string][]string, len(dataOrder))

	for ch, reqs := range data {
		inDegree[ch] = len(reqs)
		for _, r := range reqs {
			followers[r] = append(followers[r], ch)
		}
	}

	sorted := make([]string, 0, len(dataOrder))

	// Loop through and take out changes with in-degree 0
	//  - these changes can be done now
	for len(inDegree) > 0 {
		lStart := len(inDegree)

		for _, ch := range dataOrder {
			if d, ok := inDegree[ch]; ok && d == 0 {
				sorted = append(sorted, ch)
				delete(inDegree, ch)
				for _, fo := range followers[ch] {
					inDegree[fo]--
				}
			}
		}

		if lStart == len(inDegree) {
			// If nothing is removed in an iteration then we are stuck.
			// Return error because something must have gone wrong somewhere
			return nil, errors.New("sort failed")
		}
	}

	return sorted, nil
}

func storeLocation(schema string) string {
	switch {
	case charm.CharmHub.Matches(schema):
		return "charm-hub"
	case charm.Local.Matches(schema):
		return "local"
	}
	return ""
}
