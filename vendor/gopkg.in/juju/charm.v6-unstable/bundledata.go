// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

type noMethodsBundleData BundleData

type legacyBundleData struct {
	noMethodsBundleData `bson:",inline" yaml:",inline" json:",inline"`

	// LegacyServices holds application entries for older bundle files
	// that have not been migrated to use the new "application" terminology.
	LegacyServices map[string]*ApplicationSpec `json:"services" yaml:"services" bson:"services"`
}

func (lbd *legacyBundleData) setBundleData(bd *BundleData) error {
	if len(lbd.Applications) > 0 && len(lbd.LegacyServices) > 0 {
		return fmt.Errorf("cannot specify both applications and services")
	}
	if len(lbd.LegacyServices) > 0 {
		// We account for the fact that the YAML may contain a legacy entry
		// for "services" instead of "applications".
		lbd.Applications = lbd.LegacyServices
		lbd.unmarshaledWithServices = true
	}
	*bd = BundleData(lbd.noMethodsBundleData)
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (bd *BundleData) UnmarshalJSON(b []byte) error {
	var bdc legacyBundleData
	if err := json.Unmarshal(b, &bdc); err != nil {
		return err
	}
	return bdc.setBundleData(bd)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (bd *BundleData) UnmarshalYAML(f func(interface{}) error) error {
	var bdc legacyBundleData
	if err := f(&bdc); err != nil {
		return err
	}
	return bdc.setBundleData(bd)
}

// SetBSON implements the bson.Setter interface.
func (bd *BundleData) SetBSON(raw bson.Raw) error {
	// TODO(wallyworld) - bson deserialisation is not handling the inline directive,
	// so we need to unmarshal the bundle data manually.
	var b *noMethodsBundleData
	if err := raw.Unmarshal(&b); err != nil {
		return err
	}
	if b == nil {
		return bson.SetZero
	}

	var bdc legacyBundleData
	if err := raw.Unmarshal(&bdc); err != nil {
		return err
	}
	// As per the above TODO, we manually set the inline data.
	bdc.noMethodsBundleData = *b
	return bdc.setBundleData(bd)
}

// BundleData holds the contents of the bundle.
type BundleData struct {
	// Applications holds one entry for each application
	// that the bundle will create, indexed by
	// the application name.
	Applications map[string]*ApplicationSpec `bson:"applications,omitempty" json:"applications,omitempty" yaml:"applications,omitempty"`

	// Machines holds one entry for each machine referred to
	// by unit placements. These will be mapped onto actual
	// machines at bundle deployment time.
	// It is an error if a machine is specified but
	// not referred to by a unit placement directive.
	Machines map[string]*MachineSpec `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Series holds the default series to use when
	// the bundle chooses charms.
	Series string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Relations holds a slice of 2-element slices,
	// each specifying a relation between two applications.
	// Each two-element slice holds two endpoints,
	// each specified as either colon-separated
	// (application, relation) pair or just an application name.
	// The relation is made between each. If the relation
	// name is omitted, it will be inferred from the available
	// relations defined in the applications' charms.
	Relations [][]string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// White listed set of tags to categorize bundles as we do charms.
	Tags []string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Short paragraph explaining what the bundle is useful for.
	Description string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// unmarshaledWithServices holds whether the original marshaled data held a
	// legacy "services" field rather than the "applications" field.
	unmarshaledWithServices bool
}

// UnmarshaledWithServices reports whether the bundle data was
// unmarshaled from a representation that used the legacy "services"
// field rather than the "applications" field.
func (d *BundleData) UnmarshaledWithServices() bool {
	return d.unmarshaledWithServices
}

// MachineSpec represents a notional machine that will be mapped
// onto an actual machine at bundle deployment time.
type MachineSpec struct {
	Constraints string            `bson:",omitempty" json:",omitempty" yaml:",omitempty"`
	Annotations map[string]string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`
	Series      string            `bson:",omitempty" json:",omitempty" yaml:",omitempty"`
}

// ApplicationSpec represents a single application that will
// be deployed as part of the bundle.
type ApplicationSpec struct {
	// Charm holds the charm URL of the charm to
	// use for the given application.
	Charm string

	// Series is the series to use when deploying a local charm,
	// if the charm does not specify a default or the default
	// is not the desired value.
	// Series is not compatible with charm store charms where
	// the series is specified in the URL.
	Series string `bson:",omitempty" yaml:",omitempty" json:",omitempty"`

	// Resources is the set of resource revisions to deploy for the
	// application. Bundles only support charm store resources and not ones
	// that were uploaded to the controller.
	Resources map[string]int `bson:",omitempty" yaml:",omitempty" json:",omitempty"`

	// NumUnits holds the number of units of the
	// application that will be deployed.
	//
	// For a subordinate application, this actually represents
	// an arbitrary number of units depending on
	// the application it is related to.
	NumUnits int `bson:",omitempty" yaml:"num_units,omitempty" json:",omitempty"`

	// To may hold up to NumUnits members with
	// each member specifying a desired placement
	// for the respective unit of the application.
	//
	// In regular-expression-like notation, each
	// element matches the following pattern:
	//
	//      (<containertype>:)?(<unit>|<machine>|new)
	//
	// If containertype is specified, the unit is deployed
	// into a new container of that type, otherwise
	// it will be "hulk-smashed" into the specified location,
	// by co-locating it with any other units that happen to
	// be there, which may result in unintended behavior.
	//
	// The second part (after the colon) specifies where
	// the new unit should be placed - it may refer to
	// a unit of another application specified in the bundle,
	// a machine id specified in the machines section,
	// or the special name "new" which specifies a newly
	// created machine.
	//
	// A unit placement may be specified with an application name only,
	// in which case its unit number is assumed to
	// be one more than the unit number of the previous
	// unit in the list with the same application, or zero
	// if there were none.
	//
	// If there are less elements in To than NumUnits,
	// the last element is replicated to fill it. If there
	// are no elements (or To is omitted), "new" is replicated.
	//
	// For example:
	//
	//     wordpress/0 wordpress/1 lxc:0 kvm:new
	//
	//  specifies that the first two units get hulk-smashed
	//  onto the first two units of the wordpress application,
	//  the third unit gets allocated onto an lxc container
	//  on machine 0, and subsequent units get allocated
	//  on kvm containers on new machines.
	//
	// The above example is the same as this:
	//
	//     wordpress wordpress lxc:0 kvm:new
	To []string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Expose holds whether the application must be exposed.
	Expose bool `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Options holds the configuration values
	// to apply to the new application. They should
	// be compatible with the charm configuration.
	Options map[string]interface{} `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Annotations holds any annotations to apply to the
	// application when deployed.
	Annotations map[string]string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Constraints holds the default constraints to apply
	// when creating new machines for units of the application.
	// This is ignored for units with explicit placement directives.
	Constraints string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// Storage holds the constraints for storage to assign
	// to units of the application.
	Storage map[string]string `bson:",omitempty" json:",omitempty" yaml:",omitempty"`

	// EndpointBindings maps how endpoints are bound to spaces
	EndpointBindings map[string]string `bson:"bindings,omitempty" json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// ReadBundleData reads bundle data from the given reader.
// The returned data is not verified - call Verify to ensure
// that it is OK.
func ReadBundleData(r io.Reader) (*BundleData, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var bd BundleData
	if err := yaml.Unmarshal(bytes, &bd); err != nil {
		return nil, fmt.Errorf("cannot unmarshal bundle data: %v", err)
	}
	return &bd, nil
}

// VerificationError holds an error generated by BundleData.Verify,
// holding all the verification errors found when verifying.
type VerificationError struct {
	Errors []error
}

func (err *VerificationError) Error() string {
	switch len(err.Errors) {
	case 0:
		return "no verification errors!"
	case 1:
		return err.Errors[0].Error()
	}
	return fmt.Sprintf("%s (and %d more errors)", err.Errors[0], len(err.Errors)-1)
}

type bundleDataVerifier struct {
	// bundleDir is the directory containing the bundle file
	bundleDir string
	bd        *BundleData

	// machines holds the reference counts of all machines
	// as referred to by placement directives.
	machineRefCounts map[string]int

	charms map[string]Charm

	errors            []error
	verifyConstraints func(c string) error
	verifyStorage     func(s string) error
}

func (verifier *bundleDataVerifier) addErrorf(f string, a ...interface{}) {
	verifier.addError(fmt.Errorf(f, a...))
}

func (verifier *bundleDataVerifier) addError(err error) {
	verifier.errors = append(verifier.errors, err)
}

func (verifier *bundleDataVerifier) err() error {
	if len(verifier.errors) > 0 {
		return &VerificationError{verifier.errors}
	}
	return nil
}

// RequiredCharms returns a sorted slice of all the charm URLs
// required by the bundle.
func (bd *BundleData) RequiredCharms() []string {
	req := make([]string, 0, len(bd.Applications))
	for _, svc := range bd.Applications {
		req = append(req, svc.Charm)
	}
	sort.Strings(req)
	return req
}

// VerifyLocal verifies that a local bundle file is consistent.
// A local bundle file may contain references to charms which are
// referred to by a directory, either relative or absolute.
//
// bundleDir is used to construct the full path for charms specified
// using a relative directory path. The charm path is therefore expected
// to be relative to the bundle.yaml file.
func (bd *BundleData) VerifyLocal(
	bundleDir string,
	verifyConstraints func(c string) error,
	verifyStorage func(s string) error,
) error {
	return bd.verifyBundle(bundleDir, verifyConstraints, verifyStorage, nil)
}

// Verify is a convenience method that calls VerifyWithCharms
// with a nil charms map.
func (bd *BundleData) Verify(
	verifyConstraints func(c string) error,
	verifyStorage func(s string) error,
) error {
	return bd.VerifyWithCharms(verifyConstraints, verifyStorage, nil)
}

// VerifyWithCharms verifies that the bundle is consistent.
// The verifyConstraints function is called to verify any constraints
// that are found. If verifyConstraints is nil, no checking
// of constraints will be done. Similarly, a non-nil verifyStorage
// function is called to verify any storage constraints.
//
// It verifies the following:
//
// - All defined machines are referred to by placement directives.
// - All applications referred to by placement directives are specified in the bundle.
// - All applications referred to by relations are specified in the bundle.
// - All basic constraints are valid.
// - All storage constraints are valid.
//
// If charms is not nil, it should hold a map with an entry for each
// charm url returned by bd.RequiredCharms. The verification will then
// also check that applications are defined with valid charms,
// relations are correctly made and options are defined correctly.
//
// If the verification fails, Verify returns a *VerificationError describing
// all the problems found.
func (bd *BundleData) VerifyWithCharms(
	verifyConstraints func(c string) error,
	verifyStorage func(s string) error,
	charms map[string]Charm,
) error {
	return bd.verifyBundle("", verifyConstraints, verifyStorage, charms)
}

func (bd *BundleData) verifyBundle(
	bundleDir string,
	verifyConstraints func(c string) error,
	verifyStorage func(s string) error,
	charms map[string]Charm,
) error {
	if verifyConstraints == nil {
		verifyConstraints = func(string) error {
			return nil
		}
	}
	if verifyStorage == nil {
		verifyStorage = func(string) error {
			return nil
		}
	}
	verifier := &bundleDataVerifier{
		bundleDir:         bundleDir,
		verifyConstraints: verifyConstraints,
		verifyStorage:     verifyStorage,
		bd:                bd,
		machineRefCounts:  make(map[string]int),
		charms:            charms,
	}
	for id := range bd.Machines {
		verifier.machineRefCounts[id] = 0
	}
	if bd.Series != "" && !IsValidSeries(bd.Series) {
		verifier.addErrorf("bundle declares an invalid series %q", bd.Series)
	}
	verifier.verifyMachines()
	verifier.verifyApplications()
	verifier.verifyRelations()
	verifier.verifyOptions()
	verifier.verifyEndpointBindings()

	for id, count := range verifier.machineRefCounts {
		if count == 0 {
			verifier.addErrorf("machine %q is not referred to by a placement directive", id)
		}
	}
	return verifier.err()
}

var (
	validMachineId   = regexp.MustCompile("^" + names.NumberSnippet + "$")
	validStorageName = regexp.MustCompile("^" + names.StorageNameSnippet + "$")
)

func (verifier *bundleDataVerifier) verifyMachines() {
	for id, m := range verifier.bd.Machines {
		if !validMachineId.MatchString(id) {
			verifier.addErrorf("invalid machine id %q found in machines", id)
		}
		if m == nil {
			continue
		}
		if m.Constraints != "" {
			if err := verifier.verifyConstraints(m.Constraints); err != nil {
				verifier.addErrorf("invalid constraints %q in machine %q: %v", m.Constraints, id, err)
			}
		}
		if m.Series != "" && !IsValidSeries(m.Series) {
			verifier.addErrorf("invalid series %s for machine %q", m.Series, id)
		}
	}
}

func (verifier *bundleDataVerifier) verifyApplications() {
	if len(verifier.bd.Applications) == 0 {
		verifier.addErrorf("at least one application must be specified")
		return
	}
	for name, svc := range verifier.bd.Applications {
		if svc.Charm == "" {
			verifier.addErrorf("empty charm path")
		}
		// Charm may be a local directory or a charm URL.
		var curl *URL
		var err error
		if strings.HasPrefix(svc.Charm, ".") || filepath.IsAbs(svc.Charm) {
			charmPath := svc.Charm
			if !filepath.IsAbs(charmPath) {
				charmPath = filepath.Join(verifier.bundleDir, charmPath)
			}
			if _, err := os.Stat(charmPath); err != nil {
				if os.IsNotExist(err) {
					verifier.addErrorf("charm path in application %q does not exist: %v", name, charmPath)
				} else {
					verifier.addErrorf("invalid charm path in application %q: %v", name, err)
				}
			}
		} else if curl, err = ParseURL(svc.Charm); err != nil {
			verifier.addErrorf("invalid charm URL in application %q: %v", name, err)
		}

		// Check the series.
		if curl != nil && curl.Series != "" && svc.Series != "" && curl.Series != svc.Series {
			verifier.addErrorf("the charm URL for application %q has a series which does not match, please remove the series from the URL", name)
		}
		if svc.Series != "" && !IsValidSeries(svc.Series) {
			verifier.addErrorf("application %q declares an invalid series %q", name, svc.Series)
		}

		if err := verifier.verifyConstraints(svc.Constraints); err != nil {
			verifier.addErrorf("invalid constraints %q in application %q: %v", svc.Constraints, name, err)
		}
		for storageName, storageConstraints := range svc.Storage {
			if !validStorageName.MatchString(storageName) {
				verifier.addErrorf("invalid storage name %q in application %q", storageName, name)
			}
			if err := verifier.verifyStorage(storageConstraints); err != nil {
				verifier.addErrorf("invalid storage %q in application %q: %v", storageName, name, err)
			}
		}
		if verifier.charms != nil {
			if ch, ok := verifier.charms[svc.Charm]; ok {
				if ch.Meta().Subordinate {
					if len(svc.To) > 0 {
						verifier.addErrorf("application %q is subordinate but specifies unit placement", name)
					}
					if svc.NumUnits > 0 {
						verifier.addErrorf("application %q is subordinate but has non-zero num_units", name)
					}
				}
			} else {
				verifier.addErrorf("application %q refers to non-existent charm %q", name, svc.Charm)
			}
		}
		for resName := range svc.Resources {
			if resName == "" {
				verifier.addErrorf("missing resource name on application %q", name)
			}
			// We do not check the revisions because all values
			// are allowed.
		}
		if svc.NumUnits < 0 {
			verifier.addErrorf("negative number of units specified on application %q", name)
		} else if len(svc.To) > svc.NumUnits {
			verifier.addErrorf("too many units specified in unit placement for application %q", name)
		}
		verifier.verifyPlacement(svc.To)
	}
}

func (verifier *bundleDataVerifier) verifyPlacement(to []string) {
	for _, p := range to {
		up, err := ParsePlacement(p)
		if err != nil {
			verifier.addError(err)
			continue
		}
		switch {
		case up.Application != "":
			spec, ok := verifier.bd.Applications[up.Application]
			if !ok {
				verifier.addErrorf("placement %q refers to an application not defined in this bundle", p)
				continue
			}
			if up.Unit >= 0 && up.Unit >= spec.NumUnits {
				verifier.addErrorf("placement %q specifies a unit greater than the %d unit(s) started by the target application", p, spec.NumUnits)
			}
		case up.Machine == "new":
		default:
			_, ok := verifier.bd.Machines[up.Machine]
			if !ok {
				verifier.addErrorf("placement %q refers to a machine not defined in this bundle", p)
				continue
			}
			verifier.machineRefCounts[up.Machine]++
		}
	}
}

func (verifier *bundleDataVerifier) getCharmMetaForApplication(appName string) (*Meta, error) {
	svc, ok := verifier.bd.Applications[appName]
	if !ok {
		return nil, fmt.Errorf("application %q not found", appName)
	}
	ch, ok := verifier.charms[svc.Charm]
	if !ok {
		return nil, fmt.Errorf("charm %q from application %q not found", svc.Charm, appName)
	}
	return ch.Meta(), nil
}

func (verifier *bundleDataVerifier) verifyRelations() {
	seen := make(map[[2]endpoint]bool)
	for _, relPair := range verifier.bd.Relations {
		if len(relPair) != 2 {
			verifier.addErrorf("relation %q has %d endpoint(s), not 2", relPair, len(relPair))
			continue
		}
		var epPair [2]endpoint
		relParseErr := false
		for i, svcRel := range relPair {
			ep, err := parseEndpoint(svcRel)
			if err != nil {
				verifier.addError(err)
				relParseErr = true
				continue
			}
			if _, ok := verifier.bd.Applications[ep.application]; !ok {
				verifier.addErrorf("relation %q refers to application %q not defined in this bundle", relPair, ep.application)
			}
			epPair[i] = ep
		}
		if relParseErr {
			// We failed to parse at least one relation, so don't
			// bother checking further.
			continue
		}
		if epPair[0].application == epPair[1].application {
			verifier.addErrorf("relation %q relates an application to itself", relPair)
		}
		// Resolve endpoint relations if necessary and we have
		// the necessary charm information.
		if (epPair[0].relation == "" || epPair[1].relation == "") && verifier.charms != nil {
			iep0, iep1, err := inferEndpoints(epPair[0], epPair[1], verifier.getCharmMetaForApplication)
			if err != nil {
				verifier.addErrorf("cannot infer endpoint between %s and %s: %v", epPair[0], epPair[1], err)
			} else {
				// Change the endpoints that get recorded
				// as seen, so we'll diagnose a duplicate
				// relation even if one relation specifies
				// the relations explicitly and the other does
				// not.
				epPair[0], epPair[1] = iep0, iep1
			}
		}

		// Re-order pairs so that we diagnose duplicate relations
		// whichever way they're specified.
		if epPair[1].less(epPair[0]) {
			epPair[1], epPair[0] = epPair[0], epPair[1]
		}
		if _, ok := seen[epPair]; ok {
			verifier.addErrorf("relation %q is defined more than once", relPair)
		}
		if verifier.charms != nil && epPair[0].relation != "" && epPair[1].relation != "" {
			// We have charms to verify against, and the
			// endpoint has been fully specified or inferred.
			verifier.verifyRelation(epPair[0], epPair[1])
		}
		seen[epPair] = true
	}
}

func (verifier *bundleDataVerifier) verifyEndpointBindings() {
	for name, svc := range verifier.bd.Applications {
		charm, ok := verifier.charms[name]
		// Only thest the ok path here because the !ok path is tested in verifyApplications
		if !ok {
			continue
		}
		for endpoint, space := range svc.EndpointBindings {
			_, isInProvides := charm.Meta().Provides[endpoint]
			_, isInRequires := charm.Meta().Requires[endpoint]
			_, isInPeers := charm.Meta().Peers[endpoint]
			_, isInExtraBindings := charm.Meta().ExtraBindings[endpoint]

			if !(isInProvides || isInRequires || isInPeers || isInExtraBindings) {
				verifier.addErrorf(
					"application %q wants to bind endpoint %q to space %q, "+
						"but the endpoint is not defined by the charm",
					name, endpoint, space)
			}
		}

	}
}

var infoRelation = Relation{
	Name:      "juju-info",
	Role:      RoleProvider,
	Interface: "juju-info",
	Scope:     ScopeContainer,
}

// verifyRelation verifies a single relation.
// It checks that both endpoints of the relation are
// defined, and that the relationship is correctly
// symmetrical (provider to requirer) and shares
// the same interface.
func (verifier *bundleDataVerifier) verifyRelation(ep0, ep1 endpoint) {
	svc0 := verifier.bd.Applications[ep0.application]
	svc1 := verifier.bd.Applications[ep1.application]
	if svc0 == nil || svc1 == nil || svc0 == svc1 {
		// An error will be produced by verifyRelations for this case.
		return
	}
	charm0 := verifier.charms[svc0.Charm]
	charm1 := verifier.charms[svc1.Charm]
	if charm0 == nil || charm1 == nil {
		// An error will be produced by verifyApplications for this case.
		return
	}
	relProv0, okProv0 := charm0.Meta().Provides[ep0.relation]
	// The juju-info relation is provided implicitly by every
	// charm - use it if required.
	if !okProv0 && ep0.relation == infoRelation.Name {
		relProv0, okProv0 = infoRelation, true
	}
	relReq0, okReq0 := charm0.Meta().Requires[ep0.relation]
	if !okProv0 && !okReq0 {
		verifier.addErrorf("charm %q used by application %q does not define relation %q", svc0.Charm, ep0.application, ep0.relation)
	}
	relProv1, okProv1 := charm1.Meta().Provides[ep1.relation]
	// The juju-info relation is provided implicitly by every
	// charm - use it if required.
	if !okProv1 && ep1.relation == infoRelation.Name {
		relProv1, okProv1 = infoRelation, true
	}
	relReq1, okReq1 := charm1.Meta().Requires[ep1.relation]
	if !okProv1 && !okReq1 {
		verifier.addErrorf("charm %q used by application %q does not define relation %q", svc1.Charm, ep1.application, ep1.relation)
	}

	var relProv, relReq Relation
	var epProv, epReq endpoint
	switch {
	case okProv0 && okReq1:
		relProv, relReq = relProv0, relReq1
		epProv, epReq = ep0, ep1
	case okReq0 && okProv1:
		relProv, relReq = relProv1, relReq0
		epProv, epReq = ep1, ep0
	case okProv0 && okProv1:
		verifier.addErrorf("relation %q to %q relates provider to provider", ep0, ep1)
		return
	case okReq0 && okReq1:
		verifier.addErrorf("relation %q to %q relates requirer to requirer", ep0, ep1)
		return
	default:
		// Errors were added above.
		return
	}
	if relProv.Interface != relReq.Interface {
		verifier.addErrorf("mismatched interface between %q and %q (%q vs %q)", epProv, epReq, relProv.Interface, relReq.Interface)
	}
}

// verifyOptions verifies that the options are correctly defined
// with respect to the charm config options.
func (verifier *bundleDataVerifier) verifyOptions() {
	if verifier.charms == nil {
		return
	}
	for appName, svc := range verifier.bd.Applications {
		charm := verifier.charms[svc.Charm]
		if charm == nil {
			// An error will be produced by verifyApplications for this case.
			continue
		}
		config := charm.Config()
		for name, value := range svc.Options {
			opt, ok := config.Options[name]
			if !ok {
				verifier.addErrorf("cannot validate application %q: configuration option %q not found in charm %q", appName, name, svc.Charm)
				continue
			}
			_, err := opt.validate(name, value)
			if err != nil {
				verifier.addErrorf("cannot validate application %q: %v", appName, err)
			}
		}
	}
}

var validApplicationRelation = regexp.MustCompile("^(" + names.ApplicationSnippet + "):(" + names.RelationSnippet + ")$")

type endpoint struct {
	application string
	relation    string
}

func (ep endpoint) String() string {
	if ep.relation == "" {
		return ep.application
	}
	return fmt.Sprintf("%s:%s", ep.application, ep.relation)
}

func (ep1 endpoint) less(ep2 endpoint) bool {
	if ep1.application == ep2.application {
		return ep1.relation < ep2.relation
	}
	return ep1.application < ep2.application
}

func parseEndpoint(ep string) (endpoint, error) {
	m := validApplicationRelation.FindStringSubmatch(ep)
	if m != nil {
		return endpoint{
			application: m[1],
			relation:    m[2],
		}, nil
	}
	if !names.IsValidApplication(ep) {
		return endpoint{}, fmt.Errorf("invalid relation syntax %q", ep)
	}
	return endpoint{
		application: ep,
	}, nil
}

// endpointInfo holds information about one endpoint of a relation.
type endpointInfo struct {
	applicationName string
	Relation
}

// String returns the unique identifier of the relation endpoint.
func (ep endpointInfo) String() string {
	return ep.applicationName + ":" + ep.Name
}

// canRelateTo returns whether a relation may be established between ep
// and other.
func (ep endpointInfo) canRelateTo(other endpointInfo) bool {
	return ep.applicationName != other.applicationName &&
		ep.Interface == other.Interface &&
		ep.Role != RolePeer &&
		counterpartRole(ep.Role) == other.Role
}

// endpoint returns the endpoint specifier for ep.
func (ep endpointInfo) endpoint() endpoint {
	return endpoint{
		application: ep.applicationName,
		relation:    ep.Name,
	}
}

// counterpartRole returns the RelationRole that the given RelationRole
// can relate to.
func counterpartRole(r RelationRole) RelationRole {
	switch r {
	case RoleProvider:
		return RoleRequirer
	case RoleRequirer:
		return RoleProvider
	case RolePeer:
		return RolePeer
	}
	panic(fmt.Errorf("unknown relation role %q", r))
}

type UnitPlacement struct {
	// ContainerType holds the container type of the new
	// new unit, or empty if unspecified.
	ContainerType string

	// Machine holds the numeric machine id, or "new",
	// or empty if the placement specifies an application.
	Machine string

	// application holds the application name, or empty if
	// the placement specifies a machine.
	Application string

	// Unit holds the unit number of the application, or -1
	// if unspecified.
	Unit int
}

var snippetReplacer = strings.NewReplacer(
	"container", names.ContainerTypeSnippet,
	"number", names.NumberSnippet,
	"application", names.ApplicationSnippet,
)

// validPlacement holds regexp that matches valid placement requests. To
// make the expression easier to comprehend and maintain, we replace
// symbolic snippet references in the regexp by their actual regexps
// using snippetReplacer.
var validPlacement = regexp.MustCompile(
	snippetReplacer.Replace(
		"^(?:(container):)?(?:(application)(?:/(number))?|(number))$",
	),
)

// ParsePlacement parses a unit placement directive, as
// specified in the To clause of an application entry in the
// applications section of a bundle.
func ParsePlacement(p string) (*UnitPlacement, error) {
	m := validPlacement.FindStringSubmatch(p)
	if m == nil {
		return nil, fmt.Errorf("invalid placement syntax %q", p)
	}
	up := UnitPlacement{
		ContainerType: m[1],
		Application:   m[2],
		Machine:       m[4],
	}
	if unitStr := m[3]; unitStr != "" {
		// We know that unitStr must be a valid integer because
		// it's specified as such in the regexp.
		up.Unit, _ = strconv.Atoi(unitStr)
	} else {
		up.Unit = -1
	}
	if up.Application == "new" {
		if up.Unit != -1 {
			return nil, fmt.Errorf("invalid placement syntax %q", p)
		}
		up.Machine, up.Application = "new", ""
	}
	return &up, nil
}

// inferEndpoints infers missing relation names from the given endpoint
// specifications, using the given get function to retrieve charm
// data if necessary. It returns the fully specified endpoints.
func inferEndpoints(epSpec0, epSpec1 endpoint, get func(svc string) (*Meta, error)) (endpoint, endpoint, error) {
	if epSpec0.relation != "" && epSpec1.relation != "" {
		// The endpoints are already specified explicitly so
		// there is no need to fetch any charm data to infer
		// them.
		return epSpec0, epSpec1, nil
	}
	eps0, err := possibleEndpoints(epSpec0, get)
	if err != nil {
		return endpoint{}, endpoint{}, err
	}
	eps1, err := possibleEndpoints(epSpec1, get)
	if err != nil {
		return endpoint{}, endpoint{}, err
	}
	var candidates [][]endpointInfo
	for _, ep0 := range eps0 {
		for _, ep1 := range eps1 {
			if ep0.canRelateTo(ep1) {
				candidates = append(candidates, []endpointInfo{ep0, ep1})
			}
		}
	}
	switch len(candidates) {
	case 0:
		return endpoint{}, endpoint{}, fmt.Errorf("no relations found")
	case 1:
		return candidates[0][0].endpoint(), candidates[0][1].endpoint(), nil
	}

	// There's ambiguity; try discarding implicit relations.
	filtered := discardImplicitRelations(candidates)
	if len(filtered) == 1 {
		return filtered[0][0].endpoint(), filtered[0][1].endpoint(), nil
	}
	// The ambiguity cannot be resolved, so return an error.
	var keys []string
	for _, cand := range candidates {
		keys = append(keys, fmt.Sprintf("%q", relationKey(cand)))
	}
	sort.Strings(keys)
	return endpoint{}, endpoint{}, fmt.Errorf("ambiguous relation: %s %s could refer to %s",
		epSpec0, epSpec1, strings.Join(keys, "; "))
}

func discardImplicitRelations(candidates [][]endpointInfo) [][]endpointInfo {
	var filtered [][]endpointInfo
outer:
	for _, cand := range candidates {
		for _, ep := range cand {
			if ep.IsImplicit() {
				continue outer
			}
		}
		filtered = append(filtered, cand)
	}
	return filtered
}

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []endpointInfo) string {
	var names []string
	for _, ep := range endpoints {
		names = append(names, ep.String())
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

// possibleEndpoints returns all the endpoints that the given endpoint spec
// could refer to.
func possibleEndpoints(epSpec endpoint, get func(svc string) (*Meta, error)) ([]endpointInfo, error) {
	meta, err := get(epSpec.application)
	if err != nil {
		return nil, err
	}

	var eps []endpointInfo
	add := func(r Relation) {
		if epSpec.relation == "" || epSpec.relation == r.Name {
			eps = append(eps, endpointInfo{
				applicationName: epSpec.application,
				Relation:        r,
			})
		}
	}

	for _, r := range meta.Provides {
		add(r)
	}
	for _, r := range meta.Requires {
		add(r)
	}
	// Every application implicitly provides a juju-info relation.
	add(Relation{
		Name:      "juju-info",
		Role:      RoleProvider,
		Interface: "juju-info",
		Scope:     ScopeGlobal,
	})
	return eps, nil
}
