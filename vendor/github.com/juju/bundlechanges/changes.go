// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
)

// Logger defines the logging methods needed
type Logger interface {
	Tracef(string, ...interface{})
}

// ChangesConfig is used to provide the required data for determining changes.
type ChangesConfig struct {
	Bundle *charm.BundleData
	Model  *Model
	Logger Logger
	// TODO: add charm metadata for validation.
}

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
	}
	model.initializeSequence()
	model.InferMachineMap(config.Bundle)
	changes := &changeset{}
	resolver := resolver{
		bundle:  config.Bundle,
		model:   model,
		logger:  config.Logger,
		changes: changes,
	}
	addedApplications := resolver.handleApplications()
	addedMachines := resolver.handleMachines()
	resolver.handleRelations(addedApplications)
	err := resolver.handleUnits(addedApplications, addedMachines)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return changes.sorted(), nil
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
	// GUIArgs returns positional arguments to pass to the method, suitable for
	// being JSON-serialized and sent to the Juju GUI.
	GUIArgs() []interface{}
	// setId is used to set the identifier for the change.
	setId(string)

	// Description returns a human readable summary of the change.
	Description() string
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

// GUIArgs implements Change.GUIArgs.
func (ch *AddCharmChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Charm, ch.Params.Series}
}

// Description implements Change.
func (ch *AddCharmChange) Description() string {
	series := ""
	if ch.Params.Series != "" {
		series = " for series " + ch.Params.Series
	}
	return fmt.Sprintf("upload charm %s%s", ch.Params.Charm, series)
}

// AddCharmParams holds parameters for adding a charm to the environment.
type AddCharmParams struct {
	// Charm holds the URL of the charm to be added.
	Charm string
	// Series holds the series of the charm to be added
	// if the charm default is not sufficient.
	Series string
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

// GUIArgs implements Change.GUIArgs.
func (ch *UpgradeCharmChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Charm, ch.Params.Application, ch.Params.Series}
}

// Description implements Change.
func (ch *UpgradeCharmChange) Description() string {
	series := ""
	if ch.Params.Series != "" {
		series = " for series " + ch.Params.Series
	}
	return fmt.Sprintf("upgrade %s to use charm %s%s", ch.Params.Application, ch.Params.charmURL, series)
}

// UpgradeCharmParams holds parameters for adding a charm to the environment.
type UpgradeCharmParams struct {
	// Charm holds the placeholder or URL of the charm to be added.
	Charm string
	// Application refers to the application that is being upgraded.
	Application string
	// Series holds the series of the charm to be added
	// if the charm default is not sufficient.
	Series string

	// Resources identifies the revision to use for each resource
	// of the application's charm.
	Resources map[string]int
	// LocalResources identifies the path to the local resource
	// of the application's charm.
	LocalResources map[string]string

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

// GUIArgs implements Change.GUIArgs.
func (ch *AddMachineChange) GUIArgs() []interface{} {
	options := AddMachineOptions{
		Series:        ch.Params.Series,
		Constraints:   ch.Params.Constraints,
		ContainerType: ch.Params.ContainerType,
		ParentId:      ch.Params.ParentId,
	}
	return []interface{}{options}
}

// Description implements Change.
func (ch *AddMachineChange) Description() string {
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
	return fmt.Sprintf("add %s", machine)
}

// AddMachineOptions holds GUI options for adding a machine or container.
type AddMachineOptions struct {
	// Series holds the machine OS series.
	Series string `json:"series,omitempty"`
	// Constraints holds the machine constraints.
	Constraints string `json:"constraints,omitempty"`
	// ContainerType holds the machine container type (like "lxc" or "kvm").
	ContainerType string `json:"containerType,omitempty"`
	// ParentId holds the id of the parent machine.
	ParentId string `json:"parentId,omitempty"`
}

// AddMachineParams holds parameters for adding a machine or container.
type AddMachineParams struct {
	// Series holds the optional machine OS series.
	Series string
	// Constraints holds the optional machine constraints.
	Constraints string
	// ContainerType optionally holds the type of the container (for instance
	// ""lxc" or kvm"). It is not specified for top level machines.
	ContainerType string
	// ParentId optionally holds a placeholder pointing to another machine
	// change or to a unit change. This value is only specified in the case
	// this machine is a container, in which case also ContainerType is set.
	ParentId string

	existing           bool
	bundleMachineID    string
	machineID          string
	containerMachineID string
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

// GUIArgs implements Change.GUIArgs.
func (ch *AddRelationChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Endpoint1, ch.Params.Endpoint2}
}

// Description implements Change.
func (ch *AddRelationChange) Description() string {
	return fmt.Sprintf("add relation %s - %s", ch.Params.applicationEndpoint1, ch.Params.applicationEndpoint2)
}

// AddRelationParams holds parameters for adding a relation between two applications.
type AddRelationParams struct {
	// Endpoint1 and Endpoint2 hold relation endpoints in the
	// "application:interface" form, where the application is either a
	// placeholder pointing to an application change or in the case of a model
	// that already has this application deployed, the name of the
	// application, and the interface is optional. Examples are
	// "$deploy-42:web", "$deploy-42", "mysql:db".
	Endpoint1 string
	Endpoint2 string

	// These values are always refering to application names.
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

// GUIArgsWithDevices implements Change.GUIArgs and adds devices support
func (ch *AddApplicationChange) GUIArgsWithDevices() []interface{} {
	return ch.buildArgs(true)
}

func (ch *AddApplicationChange) buildArgs(includeDevices bool) []interface{} {
	options := ch.Params.Options
	if options == nil {
		options = make(map[string]interface{}, 0)
	}
	storage := ch.Params.Storage
	if storage == nil {
		storage = make(map[string]string, 0)
	}
	devices := ch.Params.Devices
	if devices == nil {
		devices = map[string]string{}
	}
	endpointBindings := ch.Params.EndpointBindings
	if endpointBindings == nil {
		endpointBindings = make(map[string]string, 0)
	}
	resources := ch.Params.Resources
	if resources == nil {
		resources = make(map[string]int, 0)
	}
	args := []interface{}{
		ch.Params.Charm,
		ch.Params.Series,
		ch.Params.Application,
		options,
		ch.Params.Constraints,
		storage,
		devices,
		endpointBindings,
		resources,
	}
	if !includeDevices {
		// delete devices after storage
		args = append(args[:6], args[6+1:]...)
	}
	return args
}

// GUIArgs implements Change.GUIArgs.
func (ch *AddApplicationChange) GUIArgs() []interface{} {
	return ch.buildArgs(false)
}

// Description implements Change.
func (ch *AddApplicationChange) Description() string {
	series := ""
	if ch.Params.Series != "" {
		series = " on " + ch.Params.Series
	}
	return fmt.Sprintf("deploy application %s%s using %s", ch.Params.Application, series, ch.Params.charmURL)
}

// AddApplicationParams holds parameters for deploying a Juju application.
type AddApplicationParams struct {
	// Charm holds the URL of the charm to be used to deploy this application.
	Charm string
	// Series holds the series of the application to be deployed
	// if the charm default is not sufficient.
	Series string
	// Application holds the application name.
	Application string
	// Options holds application options.
	Options map[string]interface{}
	// Constraints holds the optional application constraints.
	Constraints string
	// Storage holds the optional storage constraints.
	Storage map[string]string
	// Devices holds the optional devices constraints.
	Devices map[string]string
	// EndpointBindings holds the optional endpoint bindings
	EndpointBindings map[string]string
	// Resources identifies the revision to use for each resource
	// of the application's charm.
	Resources map[string]int
	// LocalResources identifies the path to the local resource
	// of the application's charm.
	LocalResources map[string]string

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

// GUIArgs implements Change.GUIArgs.
func (ch *AddUnitChange) GUIArgs() []interface{} {
	args := []interface{}{ch.Params.Application, nil}
	if ch.Params.To != "" {
		args[1] = ch.Params.To
	}
	return args
}

// Description implements Change.
func (ch *AddUnitChange) Description() string {
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

	return fmt.Sprintf("add unit %s to %s", ch.Params.unitName, placement)
}

// AddUnitParams holds parameters for adding an application unit.
type AddUnitParams struct {
	// Application holds the application placeholder name for which a unit is added.
	Application string
	// To holds the optional location where to add the unit, as a placeholder
	// pointing to another unit change or to a machine change.
	To string

	unitName             string
	placementDescription string
	// If directive is specified, it is added to the placement description
	// to explain why the unit is being placed there.
	directive   string
	baseMachine string
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

// GUIArgs implements Change.GUIArgs.
func (ch *ExposeChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Application}
}

// Description implements Change.
func (ch *ExposeChange) Description() string {
	return fmt.Sprintf("expose %s", ch.Params.appName)
}

// ExposeParams holds parameters for exposing an application.
type ExposeParams struct {
	// Application holds the placeholder name of the application that must be exposed.
	Application string

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

// GUIArgs implements Change.GUIArgs.
func (ch *SetAnnotationsChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Id, string(ch.Params.EntityType), ch.Params.Annotations}
}

// Description implements Change.
func (ch *SetAnnotationsChange) Description() string {
	return fmt.Sprintf("set annotations for %s", ch.Params.target)
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
	Id string
	// EntityType holds the type of the entity, "application" or "machine".
	EntityType EntityType
	// Annotations holds the annotations as key/value pairs.
	Annotations map[string]string

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

// GUIArgs implements Change.GUIArgs.
func (ch *SetOptionsChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Application, ch.Params.Options}
}

// Description implements Change.
func (ch *SetOptionsChange) Description() string {
	return fmt.Sprintf("set application options for %s", ch.Params.Application)
}

// SetOptionsParams holds parameters for setting options.
type SetOptionsParams struct {
	// Application is the name of the application.
	Application string
	// Options holds the changed options for the application.
	Options map[string]interface{}
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

// GUIArgs implements Change.GUIArgs.
func (ch *SetConstraintsChange) GUIArgs() []interface{} {
	return []interface{}{ch.Params.Application, ch.Params.Constraints}
}

// Description implements Change.
func (ch *SetConstraintsChange) Description() string {
	return fmt.Sprintf("set constraints for %s to %q", ch.Params.Application, ch.Params.Constraints)
}

// SetConstraintsParams holds parameters for setting constraints.
type SetConstraintsParams struct {
	// Application is the name of the application.
	Application string
	// Constraints holds the new constraints.
	Constraints string
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

// sorted returns the changes sorted by requirements, required first.
func (cs *changeset) sorted() []Change {
	done := set.NewStrings()
	var sorted []Change
	changes := cs.changes[:]
mainloop:
	for len(changes) != 0 {
		// Note that all valid bundles have at least two changes
		// (add one charm and deploy one application).
		change := changes[0]
		changes = changes[1:]
		for _, r := range change.Requires() {
			if !done.Contains(r) {

				// This change requires a change which is not yet listed.
				// Push this change at the end of the list and retry later.
				changes = append(changes, change)
				continue mainloop
			}
		}
		done.Add(change.Id())
		sorted = append(sorted, change)
	}
	return sorted
}
