// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
)

// DeployApplicationParams contains the arguments required to deploy the referenced
// charm.
type DeployApplicationParams struct {
	ApplicationName string
	Series          string
	Charm           *state.Charm
	Channel         csparams.Channel
	ConfigSettings  charm.Settings
	Constraints     constraints.Value
	NumUnits        int
	// Placement is a list of placement directives which may be used
	// instead of a machine spec.
	Placement        []*instance.Placement
	Storage          map[string]storage.Constraints
	AttachStorage    []names.StorageTag
	EndpointBindings map[string]string
	// Resources is a map of resource name to IDs of pending resources.
	Resources map[string]string
}

type ApplicationDeployer interface {
	AddApplication(state.AddApplicationArgs) (*state.Application, error)
}

type UnitAssigner interface {
	AssignUnit(*state.Unit, state.AssignmentPolicy) error
	AssignUnitWithPlacement(*state.Unit, *instance.Placement) error
}

type UnitAdder interface {
	AddUnit(state.AddUnitParams) (*state.Unit, error)
}

// DeployApplication takes a charm and various parameters and deploys it.
func DeployApplication(st ApplicationDeployer, args DeployApplicationParams) (Application, error) {
	settings, err := args.Charm.Config().ValidateSettings(args.ConfigSettings)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 {
			return nil, fmt.Errorf("subordinate application must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate application must be deployed without constraints")
		}
	}
	// TODO(fwereade): transactional State.AddApplication including settings, constraints
	// (minimumUnitCount, initialMachineIds?).

	effectiveBindings, err := getEffectiveBindingsForCharmMeta(args.Charm.Meta(), args.EndpointBindings)
	if err != nil {
		return nil, errors.Trace(err)
	}

	asa := state.AddApplicationArgs{
		Name:             args.ApplicationName,
		Series:           args.Series,
		Charm:            args.Charm,
		Channel:          args.Channel,
		Storage:          stateStorageConstraints(args.Storage),
		AttachStorage:    args.AttachStorage,
		Settings:         settings,
		NumUnits:         args.NumUnits,
		Placement:        args.Placement,
		Resources:        args.Resources,
		EndpointBindings: effectiveBindings,
	}

	if !args.Charm.Meta().Subordinate {
		asa.Constraints = args.Constraints
	}

	app, err := st.AddApplication(asa)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateApplicationShim{app}, nil
}

func quoteStrings(vals []string) string {
	out := make([]string, len(vals))
	for i, val := range vals {
		out[i] = strconv.Quote(val)
	}
	return strings.Join(out, ", ")
}

func validateGivenBindings(givenBindings map[string]string, defaultBindings map[string]string) error {
	invalidBindings := make([]string, 0)
	for name, _ := range givenBindings {
		if name == "" {
			continue
		}
		if _, ok := defaultBindings[name]; !ok {
			invalidBindings = append(invalidBindings, name)
		}
	}
	if len(invalidBindings) == 0 {
		return nil
	}
	possibleBindings := make([]string, 0)
	for name, _ := range defaultBindings {
		if name == "" {
			continue
		}
		possibleBindings = append(possibleBindings, name)
	}
	sort.Strings(invalidBindings)
	sort.Strings(possibleBindings)
	return errors.Errorf("invalid binding(s) supplied %s, valid binding names are %s",
		quoteStrings(invalidBindings), quoteStrings(possibleBindings))
}

func getEffectiveBindingsForCharmMeta(charmMeta *charm.Meta, givenBindings map[string]string) (map[string]string, error) {
	// defaultBindings contains all bindable endpoints for charmMeta as keys and
	// empty space names as values, so we use defaultBindings as fallback.
	defaultBindings := state.DefaultEndpointBindingsForCharm(charmMeta)
	if givenBindings == nil {
		givenBindings = make(map[string]string, len(defaultBindings))
	}
	if err := validateGivenBindings(givenBindings, defaultBindings); err != nil {
		return nil, err
	}

	// Get the application-level default binding for all unspecified endpoints, if
	// set. Otherwise use the empty default.
	applicationDefaultSpace, defaultSupplied := givenBindings[""]
	if defaultSupplied {
		// Record that a default binding was requested
		defaultBindings[""] = applicationDefaultSpace
	}

	effectiveBindings := make(map[string]string, len(defaultBindings))
	for endpoint, _ := range defaultBindings {
		if givenSpace, isGiven := givenBindings[endpoint]; isGiven {
			effectiveBindings[endpoint] = givenSpace
		} else {
			effectiveBindings[endpoint] = applicationDefaultSpace
		}
	}
	return effectiveBindings, nil
}

// addUnits starts n units of the given application using the specified placement
// directives to allocate the machines.
func addUnits(
	unitAssigner UnitAssigner,
	unitAdder UnitAdder,
	appName string,
	n int,
	placement []*instance.Placement,
) ([]*state.Unit, error) {
	units := make([]*state.Unit, n)
	// Hard code for now till we implement a different approach.
	policy := state.AssignCleanEmpty
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := unitAdder.AddUnit(state.AddUnitParams{})
		if err != nil {
			return nil, errors.Annotatef(err, "cannot add unit %d/%d to application %q", i+1, n, appName)
		}
		// Are there still placement directives to use?
		if i > len(placement)-1 {
			if err := unitAssigner.AssignUnit(unit, policy); err != nil {
				return nil, errors.Trace(err)
			}
			units[i] = unit
			continue
		}
		if err := unitAssigner.AssignUnitWithPlacement(unit, placement[i]); err != nil {
			return nil, errors.Annotatef(err, "adding new machine to host unit %q", unit.Name())
		}
		units[i] = unit
	}
	return units, nil
}

func stateStorageConstraints(cons map[string]storage.Constraints) map[string]state.StorageConstraints {
	result := make(map[string]state.StorageConstraints)
	for name, cons := range cons {
		result[name] = state.StorageConstraints{
			Pool:  cons.Pool,
			Size:  cons.Size,
			Count: cons.Count,
		}
	}
	return result
}
