// Copyright 2018 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bundlechanges

import (
	"reflect"
	"sort"
	"strings"

	"github.com/juju/charm/v13"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

// DiffSide represents one side of a bundle-model diff.
type DiffSide string

const (
	// None represents neither the bundle or model side (used when neither is missing).
	None DiffSide = ""

	// BundleSide represents the bundle side of a diff.
	BundleSide DiffSide = "bundle"

	// ModelSide represents the model side of a diff.
	ModelSide DiffSide = "model"

	allEndpoints = ""
)

// DiffConfig provides the values and configuration needed to diff the
// bundle and model.
type DiffConfig struct {
	Bundle *charm.BundleData
	Model  *Model

	IncludeAnnotations bool
	Logger             Logger
}

// Validate returns whether this is a valid configuration for diffing.
func (config DiffConfig) Validate() error {
	if config.Bundle == nil {
		return errors.NotValidf("nil bundle")
	}
	if config.Model == nil {
		return errors.NotValidf("nil model")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil logger")
	}
	return config.Bundle.Verify(nil, nil, nil)
}

// BuildDiff returns a BundleDiff with the differences between the
// passed in bundle and model.
func BuildDiff(config DiffConfig) (*BundleDiff, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	differ := &differ{config: config}
	return differ.build()
}

type differ struct {
	config DiffConfig
}

func (d *differ) build() (*BundleDiff, error) {
	return &BundleDiff{
		Applications: d.diffApplications(),
		Machines:     d.diffMachines(),
		Relations:    d.diffRelations(),
	}, nil
}

func (d *differ) diffApplications() map[string]*ApplicationDiff {
	// Collect applications from both sides.
	allApps := set.NewStrings()
	for app := range d.config.Bundle.Applications {
		allApps.Add(app)
	}
	for app := range d.config.Model.Applications {
		allApps.Add(app)
	}

	results := make(map[string]*ApplicationDiff)
	for _, name := range allApps.SortedValues() {
		diff := d.diffApplication(name)
		if diff != nil {
			results[name] = diff
		}
	}
	if len(results) == 0 {
		return nil
	}
	return results
}

func (d *differ) diffApplication(name string) *ApplicationDiff {
	bundle, found := d.config.Bundle.Applications[name]
	if !found {
		return &ApplicationDiff{Missing: BundleSide}
	}
	model, found := d.config.Model.Applications[name]
	if !found {
		return &ApplicationDiff{Missing: ModelSide}
	}

	// To avoid potential security issues, exported bundles with explicit
	// per-endpoint expose settings do not include the "exposed:true" flag.
	// To this end, we must calculate an effective exposed value to use for
	// the comparison.
	effectiveBundleExpose := bundle.Expose || len(bundle.ExposedEndpoints) != 0
	effectiveModelExpose := model.Exposed || len(model.ExposedEndpoints) != 0

	// If any of the sides is exposed but lacks any expose endpoint details
	// assume an implicit expose "" to 0.0.0.0/0 for comparison purposes.
	// This allows us to compute correct diffs with bundles that only
	// specify the "exposed:true" flag. This matches the expose behavior on
	// pre 2.9 controllers.
	if effectiveBundleExpose && len(bundle.ExposedEndpoints) == 0 {
		bundle.ExposedEndpoints = map[string]charm.ExposedEndpointSpec{
			allEndpoints: {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		}
	}
	if effectiveModelExpose && len(model.ExposedEndpoints) == 0 {
		model.ExposedEndpoints = map[string]ExposedEndpoint{
			allEndpoints: {
				ExposeToCIDRs: []string{"0.0.0.0/0", "::/0"},
			},
		}
	}

	// Use the bundle base as the fallback base if the application doesn't
	// supply a base. This is the same machinery that Juju itself uses to
	// apply base for applications.
	bundleBase := bundle.Base
	if bundleBase == "" {
		bundleBase = d.config.Bundle.DefaultBase
	}

	result := &ApplicationDiff{
		Charm:            d.diffStrings(bundle.Charm, model.Charm),
		Expose:           d.diffBools(effectiveBundleExpose, effectiveModelExpose),
		ExposedEndpoints: d.diffExposedEndpoints(bundle.ExposedEndpoints, model.ExposedEndpoints),
		Base:             d.diffStrings(bundleBase, model.Base.String()),
		Channel:          d.diffStrings(bundle.Channel, model.Channel),
		Constraints:      d.diffStrings(bundle.Constraints, model.Constraints),
		Options:          d.diffOptions(bundle.Options, model.Options),
	}

	if bundle.Revision != nil {
		result.Revision = d.diffInts(*bundle.Revision, model.Revision)
	} else {
		result.Revision = d.diffInts(-1, model.Revision)
	}

	if d.config.IncludeAnnotations {
		result.Annotations = d.diffAnnotations(bundle.Annotations, model.Annotations)
	}
	if len(model.SubordinateTo) == 0 {
		// We don't check num_units for subordinate apps.
		if d.config.Bundle.Type == Kubernetes {
			result.Scale = d.diffInts(bundle.NumUnits, model.Scale)
		} else {
			result.NumUnits = d.diffInts(bundle.NumUnits, len(model.Units))
		}
	}
	if d.config.Bundle.Type == Kubernetes && len(bundle.To) > 0 {
		result.Placement = d.diffStrings(bundle.To[0], model.Placement)
	}

	if result.Empty() {
		return nil
	}
	return result
}

func (d *differ) diffMachines() map[string]*MachineDiff {
	unseen := set.NewStrings()
	for machineID := range d.config.Model.Machines {
		unseen.Add(machineID)
	}
	// Go through the machines from the bundle, but keep track of
	// which model machines we've seen.
	results := make(map[string]*MachineDiff)
	for bundleID, bundleMachine := range d.config.Bundle.Machines {
		modelID := d.toModelMachineID(bundleID)
		unseen.Remove(modelID)

		if bundleMachine == nil {
			// This is equivalent to an empty machine spec.
			bundleMachine = &charm.MachineSpec{}
		}
		modelMachine, found := d.config.Model.Machines[modelID]
		if !found {
			results[modelID] = &MachineDiff{Missing: ModelSide}
			continue
		}

		// Use the bundle base as the fallback base if the machine doesn't
		// supply a base. This is the same machinery that Juju itself uses to
		// apply base for machines.
		bundleBase := bundleMachine.Base
		if bundleBase == "" {
			bundleBase = d.config.Bundle.DefaultBase
		}

		diff := &MachineDiff{
			Base: d.diffStrings(
				bundleBase, modelMachine.Base.String(),
			),
		}
		if d.config.IncludeAnnotations {
			diff.Annotations = d.diffAnnotations(
				bundleMachine.Annotations, modelMachine.Annotations,
			)
		}
		if !diff.Empty() {
			results[modelID] = diff
		}
	}

	// Add missing bundle machines for any model machines that weren't
	// seen.
	for _, modelName := range unseen.Values() {
		results[modelName] = &MachineDiff{Missing: BundleSide}
	}

	if len(results) == 0 {
		return nil
	}
	return results
}

func (d *differ) toModelMachineID(bundleMachineID string) string {
	result, found := d.config.Model.MachineMap[bundleMachineID]
	if !found {
		// We always assume use-existing-machines.
		return bundleMachineID
	}
	return result
}

func (d *differ) diffRelations() *RelationsDiff {
	bundleSet := make(map[Relation]bool)
	for _, relation := range d.config.Bundle.Relations {
		bundleSet[relationFromEndpoints(relation)] = true
	}

	modelSet := make(map[Relation]bool)
	var modelAdditions []Relation
	for _, original := range d.config.Model.Relations {
		relation := canonicalRelation(original)
		modelSet[relation] = true
		_, found := bundleSet[relation]
		if !found {
			modelAdditions = append(modelAdditions, relation)
		}
	}

	var bundleAdditions []Relation
	for relation := range bundleSet {
		_, found := modelSet[relation]
		if !found {
			bundleAdditions = append(bundleAdditions, relation)
		}
	}

	if len(bundleAdditions) == 0 && len(modelAdditions) == 0 {
		return nil
	}

	sort.Slice(bundleAdditions, relationLess(bundleAdditions))
	sort.Slice(modelAdditions, relationLess(modelAdditions))
	return &RelationsDiff{
		BundleAdditions: toRelationSlices(bundleAdditions),
		ModelAdditions:  toRelationSlices(modelAdditions),
	}
}

func (d *differ) diffAnnotations(bundle, model map[string]string) map[string]StringDiff {
	all := set.NewStrings()
	for name := range bundle {
		all.Add(name)
	}
	for name := range model {
		all.Add(name)
	}
	result := make(map[string]StringDiff)
	for _, name := range all.Values() {
		bundleValue := bundle[name]
		modelValue := model[name]
		if bundleValue != modelValue {
			result[name] = StringDiff{
				Bundle: bundleValue,
				Model:  modelValue,
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (d *differ) diffOptions(bundle, model map[string]interface{}) map[string]OptionDiff {
	all := set.NewStrings()
	for name := range bundle {
		all.Add(name)
	}
	for name := range model {
		all.Add(name)
	}
	result := make(map[string]OptionDiff)
	for _, name := range all.Values() {
		bundleValue := bundle[name]
		modelValue := model[name]
		if !reflect.DeepEqual(bundleValue, modelValue) {
			result[name] = OptionDiff{
				Bundle: bundleValue,
				Model:  modelValue,
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (d *differ) diffExposedEndpoints(bundle map[string]charm.ExposedEndpointSpec, model map[string]ExposedEndpoint) map[string]ExposedEndpointDiff {
	allEndpoints := set.NewStrings()
	for name := range bundle {
		allEndpoints.Add(name)
	}
	for name := range model {
		allEndpoints.Add(name)
	}
	result := make(map[string]ExposedEndpointDiff)

	for _, name := range allEndpoints.Values() {
		bundleValue, foundInBundle := bundle[name]
		modelValue, foundInModel := model[name]
		if !reflect.DeepEqual(bundleValue.ExposeToSpaces, modelValue.ExposeToSpaces) ||
			!reflect.DeepEqual(bundleValue.ExposeToCIDRs, modelValue.ExposeToCIDRs) ||
			foundInBundle != foundInModel {

			expDiff := ExposedEndpointDiff{}
			if foundInBundle {
				expDiff.Bundle = &ExposedEndpointDiffEntry{
					ExposeToSpaces: bundleValue.ExposeToSpaces,
					ExposeToCIDRs:  bundleValue.ExposeToCIDRs,
				}
			}
			if foundInModel {
				expDiff.Model = &ExposedEndpointDiffEntry{
					ExposeToSpaces: modelValue.ExposeToSpaces,
					ExposeToCIDRs:  modelValue.ExposeToCIDRs,
				}
			}
			result[name] = expDiff
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (d *differ) diffStrings(bundle, model string) *StringDiff {
	if bundle == model {
		return nil
	}
	return &StringDiff{Bundle: bundle, Model: model}
}

func (d *differ) diffInts(bundle, model int) *IntDiff {
	if bundle == model {
		return nil
	}
	return &IntDiff{Bundle: bundle, Model: model}
}

func (d *differ) diffBools(bundle, model bool) *BoolDiff {
	if bundle == model {
		return nil
	}
	return &BoolDiff{Bundle: bundle, Model: model}
}

// BundleDiff stores differences between a bundle and a model.
type BundleDiff struct {
	Applications map[string]*ApplicationDiff `yaml:"applications,omitempty"`
	Machines     map[string]*MachineDiff     `yaml:"machines,omitempty"`
	Relations    *RelationsDiff              `yaml:"relations,omitempty"`
}

// Empty returns whether the compared bundle and model match (at least
// in terms of the details we check).
func (d *BundleDiff) Empty() bool {
	return len(d.Applications) == 0 &&
		len(d.Machines) == 0 &&
		d.Relations == nil
}

// ApplicationDiff stores differences between an application in a bundle and a model.
type ApplicationDiff struct {
	Missing          DiffSide                       `yaml:"missing,omitempty"`
	Charm            *StringDiff                    `yaml:"charm,omitempty"`
	Base             *StringDiff                    `yaml:"base,omitempty"`
	Channel          *StringDiff                    `yaml:"channel,omitempty"`
	Revision         *IntDiff                       `yaml:"revision,omitempty"`
	Placement        *StringDiff                    `yaml:"placement,omitempty"`
	NumUnits         *IntDiff                       `yaml:"num_units,omitempty"`
	Scale            *IntDiff                       `yaml:"scale,omitempty"`
	Expose           *BoolDiff                      `yaml:"expose,omitempty"`
	ExposedEndpoints map[string]ExposedEndpointDiff `yaml:"exposed_endpoints,omitempty"`
	Options          map[string]OptionDiff          `yaml:"options,omitempty"`
	Annotations      map[string]StringDiff          `yaml:"annotations,omitempty"`
	Constraints      *StringDiff                    `yaml:"constraints,omitempty"`

	// TODO (bundlediff): resources, storage, devices, endpoint
	// bindings
}

// Empty returns whether the compared bundle and model applications
// match.
func (d *ApplicationDiff) Empty() bool {
	return d.Missing == None &&
		d.Charm == nil &&
		d.Base == nil &&
		d.Channel == nil &&
		d.Revision == nil &&
		d.Placement == nil &&
		d.NumUnits == nil &&
		d.Scale == nil &&
		d.Expose == nil &&
		len(d.ExposedEndpoints) == 0 &&
		len(d.Options) == 0 &&
		len(d.Annotations) == 0 &&
		d.Constraints == nil
}

// StringDiff stores different bundle and model values for some
// string.
type StringDiff struct {
	Bundle string `yaml:"bundle"`
	Model  string `yaml:"model"`
}

// IntDiff stores different bundle and model values for some int.
type IntDiff struct {
	Bundle int `yaml:"bundle"`
	Model  int `yaml:"model"`
}

// BoolDiff stores different bundle and model values for some bool.
type BoolDiff struct {
	Bundle bool `yaml:"bundle"`
	Model  bool `yaml:"model"`
}

// OptionDiff stores different bundle and model values for some
// configuration value.
type OptionDiff struct {
	Bundle interface{} `yaml:"bundle"`
	Model  interface{} `yaml:"model"`
}

// MachineDiff stores differences between a machine in a bundle and a model.
type MachineDiff struct {
	Missing     DiffSide              `yaml:"missing,omitempty"`
	Annotations map[string]StringDiff `yaml:"annotations,omitempty"`
	Base        *StringDiff           `yaml:"base,omitempty"`
}

// Empty returns whether the compared bundle and model machines match.
func (d *MachineDiff) Empty() bool {
	return d.Missing == None &&
		len(d.Annotations) == 0 &&
		d.Base == nil
}

// RelationsDiff stores differences between relations in a bundle and
// model.
type RelationsDiff struct {
	BundleAdditions [][]string `yaml:"bundle-additions,omitempty"`
	ModelAdditions  [][]string `yaml:"model-additions,omitempty"`
}

// relationFromEndpoints returns a (canonicalised) Relation from a
// [app1:ep1 app2:ep2] bundle relation.
func relationFromEndpoints(relation []string) Relation {
	relation = relation[:]
	sort.Strings(relation)
	parts1 := strings.SplitN(relation[0], ":", 2)
	parts2 := strings.SplitN(relation[1], ":", 2)

	// According to our docs, bundles may optionally omit the endpoint from
	// relations which will cause an index out of bounds panic when trying
	// to construct the Relation instance below.
	if len(parts1) == 1 {
		parts1 = append(parts1, "")
	}
	if len(parts2) == 1 {
		parts2 = append(parts2, "")
	}

	return Relation{
		App1:      parts1[0],
		Endpoint1: parts1[1],
		App2:      parts2[0],
		Endpoint2: parts2[1],
	}
}

// canonicalRelation ensures that the endpoints of the relation are in
// lexicographic order so we can put them into a map and find them
// even a relation was given to us in the other order.
func canonicalRelation(relation Relation) Relation {
	if relation.App1 < relation.App2 {
		return relation
	}
	if relation.App1 == relation.App2 && relation.Endpoint1 <= relation.Endpoint2 {
		return relation
	}
	// The endpoints need to be swapped.
	return Relation{
		App1:      relation.App2,
		Endpoint1: relation.Endpoint2,
		App2:      relation.App1,
		Endpoint2: relation.Endpoint1,
	}
}

// relationLess returns a func that compares Relations
// lexicographically.
func relationLess(relations []Relation) func(i, j int) bool {
	return func(i, j int) bool {
		a := relations[i]
		b := relations[j]
		if a.App1 < b.App1 {
			return true
		}
		if a.App1 > b.App1 {
			return false
		}
		if a.Endpoint1 < b.Endpoint1 {
			return true
		}
		if a.Endpoint1 > b.Endpoint1 {
			return false
		}
		if a.App2 < b.App2 {
			return true
		}
		if a.App2 > b.App2 {
			return false
		}
		return a.Endpoint2 < b.Endpoint2
	}
}

// toRelationSlices converts []Relation to [][]string{{"app:ep",
// "app:ep"}}.
func toRelationSlices(relations []Relation) [][]string {
	result := make([][]string, len(relations))
	for i, relation := range relations {
		result[i] = []string{
			relation.App1 + ":" + relation.Endpoint1,
			relation.App2 + ":" + relation.Endpoint2,
		}
	}
	return result
}

// ExposedEndpointDiff stores different bundle and model values for the expose
// settings for a particular endpoint. Nil values indicate that the value
// was not present in the bundle or model.
type ExposedEndpointDiff struct {
	Bundle *ExposedEndpointDiffEntry `yaml:"bundle"`
	Model  *ExposedEndpointDiffEntry `yaml:"model"`
}

// ExposedEndpointDiffEntry stores the exposed endpoint parameters for
// an ExposedEndpointDiff entry.
type ExposedEndpointDiffEntry struct {
	ExposeToSpaces []string `yaml:"expose_to_spaces,omitempty"`
	ExposeToCIDRs  []string `yaml:"expose_to_cidrs,omitempty"`
}
