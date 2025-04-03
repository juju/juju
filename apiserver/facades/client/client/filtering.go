// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"fmt"
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/state"
)

var InvalidFormatErr = errors.Errorf("the given filter did not match any known patterns.")

// UnitChainPredicateFn builds a function which runs the given
// predicate over a unit and all of its subordinates. If one unit in
// the chain matches, the entire chain matches.
func UnitChainPredicateFn(
	ctx context.Context,
	predicate Predicate,
	getUnit func(string) *state.Unit,
) func(*state.Unit) (bool, error) {
	considered := make(map[string]bool)
	var f func(unit *state.Unit) (bool, error)
	f = func(unit *state.Unit) (bool, error) {
		// Don't try and filter the same unit 2x.
		if matches, ok := considered[unit.Name()]; ok {
			logger.Debugf(context.TODO(), "%s has already been examined and found to be: %t", unit.Name(), matches)
			return matches, nil
		}

		// Check the current unit.
		matches, err := predicate(ctx, unit)
		if err != nil {
			return false, errors.Annotate(err, "could not filter units")
		}
		considered[unit.Name()] = matches

		// Now check all of this unit's subordinates.
		for _, subName := range unit.SubordinateNames() {
			// A master match supercedes any subordinate match.
			if matches {
				logger.Debugf(context.TODO(), "%s is a subordinate to a match.", subName)
				considered[subName] = true
				continue
			}

			subUnit := getUnit(subName)
			if subUnit == nil {
				// We have already deleted this unit
				matches = false
				continue
			}
			matches, err = f(subUnit)
			if err != nil {
				return false, err
			}
			considered[subName] = matches
		}

		return matches, nil
	}
	return f
}

// BuildPredicate returns a Predicate which will evaluate a machine,
// service, or unit against the given patterns.
func (c *Client) BuildPredicateFor(patterns []string) Predicate {

	or := func(ctx context.Context, predicates ...closurePredicate) (bool, error) {
		// Differentiate between a valid format that eliminated all
		// elements, and an invalid query.
		oneValidFmt := false
		for _, p := range predicates {
			if matches, ok, err := p(ctx); err != nil {
				return false, err
			} else if ok {
				oneValidFmt = true
				if matches {
					return true, nil
				}
			}
		}

		if !oneValidFmt && len(predicates) > 0 {
			return false, InvalidFormatErr
		}

		return false, nil
	}

	return func(ctx context.Context, i interface{}) (bool, error) {
		switch i.(type) {
		default:
			panic(errors.Errorf("expected a machine or an applications or a unit, got %T", i))
		case *state.Machine:
			shims, err := c.buildMachineMatcherShims(i.(*state.Machine), patterns)
			if err != nil {
				return false, err
			}
			return or(ctx, shims...)
		case *state.Unit:
			return or(ctx, c.buildUnitMatcherShims(i.(*state.Unit), patterns)...)
		case *state.Application:
			shims, err := c.buildApplicationMatcherShims(i.(*state.Application), patterns...)
			if err != nil {
				return false, err
			}
			return or(ctx, shims...)
		}
	}
}

// Predicate is a function that when given a unit, machine, or
// service, will determine whether the unit meets some criteria.
type Predicate func(context.Context, interface{}) (matches bool, _ error)

// closurePredicate is a function which has at some point been closed
// around an element so that it can examine whether this element
// matches some criteria.
type closurePredicate func(context.Context) (matches bool, formatOK bool, _ error)

func matchMachineId(m *state.Machine, patterns []string) (bool, bool, error) {
	var anyValid bool
	for _, p := range patterns {
		if !names.IsValidMachine(p) {
			continue
		}
		anyValid = true
		if m.Id() == p || strings.HasPrefix(m.Id(), p+"/") {
			// Pattern matches the machine, or container's
			// host machine.
			return true, true, nil
		}
	}
	return false, anyValid, nil
}

func (c *Client) unitMatchUnitName(ctx context.Context, u *state.Unit, patterns []string) (bool, bool, error) {
	um, err := NewUnitMatcher(patterns)
	if err != nil {
		// Currently, the only error possible here is a matching
		// error. We don't want this error to hold up further
		// matching.
		logger.Debugf(context.TODO(), "ignoring matching error: %v", err)
		return false, false, nil
	}
	return um.matchUnit(u), true, nil
}

func (c *Client) unitMatchAgentStatus(ctx context.Context, u *state.Unit, patterns []string) (bool, bool, error) {
	unitName, err := coreunit.NewName(u.Name())
	if err != nil {
		return false, false, err
	}
	agentStatusInfo, _, err := c.statusService.GetUnitAndAgentDisplayStatus(ctx, unitName)
	if err != nil {
		return false, false, err
	}
	return matchAgentStatus(patterns, agentStatusInfo.Status)
}

func (c *Client) unitMatchWorkloadStatus(ctx context.Context, u *state.Unit, patterns []string) (bool, bool, error) {
	unitName, err := coreunit.NewName(u.Name())
	if err != nil {
		return false, false, err
	}
	agentStatusInfo, workloadStatusInfo, err := c.statusService.GetUnitAndAgentDisplayStatus(ctx, unitName)
	if err != nil {
		return false, false, err
	}
	return matchWorkloadStatus(patterns, workloadStatusInfo.Status, agentStatusInfo.Status)
}

func (c *Client) unitMatchExposure(ctx context.Context, u *state.Unit, patterns []string) (bool, bool, error) {
	s, err := u.Application()
	if err != nil {
		return false, false, err
	}
	return matchExposure(patterns, s)
}

func (c *Client) unitMatchPort(ctx context.Context, u *state.Unit, patterns []string) (bool, bool, error) {
	unitName, err := coreunit.NewName(u.Name())
	if err != nil {
		return false, false, err
	}
	unitUUID, err := c.applicationService.GetUnitUUID(ctx, unitName)
	if err != nil {
		return false, false, err
	}
	unitPortRanges, err := c.portService.GetUnitOpenedPorts(ctx, unitUUID)
	if err != nil {
		return false, false, err
	}

	return matchPortRanges(patterns, unitPortRanges.UniquePortRanges()...)
}

// buildApplicationMatcherShims adds matchers for application name, application units and
// whether the application is exposed.
func (c *Client) buildApplicationMatcherShims(a *state.Application, patterns ...string) (shims []closurePredicate, _ error) {
	// Match on name.
	shims = append(shims, func(ctx context.Context) (bool, bool, error) {
		for _, p := range patterns {
			if strings.EqualFold(a.Name(), p) {
				return true, true, nil
			}
		}
		return false, true, nil
	})

	// Match on exposure.
	shims = append(shims, func(ctx context.Context) (bool, bool, error) { return matchExposure(patterns, a) })

	// If the service has an unit instance that matches any of the
	// given criteria, consider the service a match as well.
	unitShims, err := c.buildShimsForUnit(a.AllUnits, patterns...)
	if err != nil {
		return nil, err
	}
	shims = append(shims, unitShims...)

	// Units may be able to match the pattern. Ultimately defer to
	// that logic, and guard against breaking the predicate-chain.
	if len(unitShims) <= 0 {
		shims = append(shims, func(ctx context.Context) (bool, bool, error) { return false, true, nil })
	}

	return shims, nil
}

func (c *Client) buildShimsForUnit(unitsFn func() ([]*state.Unit, error), patterns ...string) (shims []closurePredicate, _ error) {
	units, err := unitsFn()
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		shims = append(shims, c.buildUnitMatcherShims(u, patterns)...)
	}
	return shims, nil
}

func (c *Client) buildMachineMatcherShims(m *state.Machine, patterns []string) (shims []closurePredicate, _ error) {
	// Look at machine ID.
	shims = append(shims, func(ctx context.Context) (bool, bool, error) { return matchMachineId(m, patterns) })

	// Look at machine status.
	statusInfo, err := m.Status()
	if err != nil {
		return nil, err
	}
	shims = append(shims, func(ctx context.Context) (bool, bool, error) { return matchAgentStatus(patterns, statusInfo.Status) })

	// Look at machine addresses. WARNING: Avoid the temptation to
	// bring the append into the loop. The value we would close over
	// will continue to change after the closure is created, and we'd
	// only examine the last element of the loop for all closures.
	var addrs []string
	for _, a := range m.Addresses() {
		addrs = append(addrs, a.Value)
	}
	shims = append(shims, func(ctx context.Context) (bool, bool, error) { return matchSubnet(patterns, addrs...) })

	// Units may be able to match the pattern. Ultimately defer to
	// that logic, and guard against breaking the predicate-chain.
	shims = append(shims, func(ctx context.Context) (bool, bool, error) { return false, true, nil })

	return
}

func (c *Client) buildUnitMatcherShims(u *state.Unit, patterns []string) []closurePredicate {
	closeOver := func(f func(context.Context, *state.Unit, []string) (bool, bool, error)) closurePredicate {
		return func(ctx context.Context) (bool, bool, error) { return f(ctx, u, patterns) }
	}
	return []closurePredicate{
		closeOver(c.unitMatchUnitName),
		closeOver(c.unitMatchAgentStatus),
		closeOver(c.unitMatchWorkloadStatus),
		closeOver(c.unitMatchExposure),
		closeOver(c.unitMatchPort),
	}
}

// portsFromString gets "from port" and "to port" value from port string.
func getPortsFromString(portStr string) (int, int, error) {
	var portFrom, portTo int
	var err error
	portFromStr, portToStr, isPortRange := strings.Cut(portStr, "-")
	if isPortRange {
		portFrom, err = strconv.Atoi(portFromStr)
		if err != nil {
			return -1, -1, err
		}
		portTo, err = strconv.Atoi(portToStr)
		if err != nil {
			return -1, -1, err
		}
	} else {
		portFrom, err = strconv.Atoi(portStr)
		if err != nil {
			return -1, -1, err
		}
		portTo = portFrom
	}
	return portFrom, portTo, nil
}

func matchPortRanges(patterns []string, portRanges ...network.PortRange) (bool, bool, error) {
	for _, p := range portRanges {
		pNumStr, pProto, _ := strings.Cut(p.String(), "/")
		pFrom, pTo, err := getPortsFromString(pNumStr)
		if err != nil {
			return false, true, nil
		}
		for _, patt := range patterns {
			pattNumStr, pattProto, isPortPattern := strings.Cut(patt, "/")
			pattFrom, pattTo, err := getPortsFromString(pattNumStr)
			if err != nil {
				return false, true, nil
			}
			isPortInRange := pattFrom <= pTo && pattTo >= pFrom
			if isPortPattern && isPortInRange && pProto == pattProto {
				return true, true, nil
			}
		}
	}
	return false, true, nil
}

func matchSubnet(patterns []string, addresses ...string) (bool, bool, error) {
	oneValidPattern := false
	for _, p := range patterns {
		for _, a := range addresses {
			if p == a {
				return true, true, nil
			}
		}
		if _, ipNet, err := net.ParseCIDR(p); err == nil {
			oneValidPattern = true
			for _, a := range addresses {
				if ip := net.ParseIP(a); ip != nil {
					if ipNet.Contains(ip) {
						return true, true, nil
					}
				}
			}
		}
	}
	return false, oneValidPattern, nil
}

func matchExposure(patterns []string, s *state.Application) (bool, bool, error) {
	if len(patterns) >= 1 && patterns[0] == "exposed" {
		return s.IsExposed(), true, nil
	} else if len(patterns) >= 2 && patterns[0] == "not" && patterns[1] == "exposed" {
		return !s.IsExposed(), true, nil
	}
	return false, false, nil
}

func matchWorkloadStatus(patterns []string, workloadStatus status.Status, agentStatus status.Status) (bool, bool, error) {
	oneValidStatus := false
	for _, p := range patterns {
		// If the pattern isn't a known status, ignore it.
		ps := status.Status(p)
		if !ps.KnownWorkloadStatus() {
			continue
		}

		oneValidStatus = true
		// To preserve current expected behaviour, we only report on workload status
		// if the agent itself is not in error.
		if agentStatus != status.Error && workloadStatus.WorkloadMatches(ps) {
			return true, true, nil
		}
	}
	return false, oneValidStatus, nil
}

func matchAgentStatus(patterns []string, agentStatus status.Status) (bool, bool, error) {
	oneValidStatus := false
	for _, p := range patterns {
		// If the pattern isn't a known status, ignore it.
		ps := status.Status(p)
		if !ps.KnownAgentStatus() {
			continue
		}

		oneValidStatus = true
		if agentStatus.Matches(ps) {
			return true, true, nil
		}
	}
	return false, oneValidStatus, nil
}

type unitMatcher struct {
	patterns []string
}

// matchesAny returns true if the unitMatcher will
// match any unit, regardless of its attributes.
func (m unitMatcher) matchesAny() bool {
	return len(m.patterns) == 0
}

// matchUnit attempts to match a state.Unit to one of
// a set of patterns, taking into account subordinate
// relationships.
func (m unitMatcher) matchUnit(u *state.Unit) bool {
	if m.matchesAny() {
		return true
	}

	// Keep the unit if:
	//  (a) its name matches a pattern, or
	//  (b) it's a principal and one of its subordinates matches, or
	//  (c) it's a subordinate and its principal matches.
	//
	// Note: do *not* include a second subordinate if the principal is
	// only matched on account of a first subordinate matching.
	if m.matchString(u.Name()) {
		return true
	}
	if u.IsPrincipal() {
		for _, s := range u.SubordinateNames() {
			if m.matchString(s) {
				return true
			}
		}
		return false
	}
	principal, valid := u.PrincipalName()
	if !valid {
		panic("PrincipalName failed for subordinate unit")
	}
	return m.matchString(principal)
}

// matchString matches a string to one of the patterns in
// the unit matcher, returning an error if a pattern with
// invalid syntax is encountered.
func (m unitMatcher) matchString(s string) bool {
	for _, pattern := range m.patterns {
		ok, err := path.Match(pattern, s)
		if err != nil {
			// We validate patterns, so should never get here.
			panic(fmt.Errorf("pattern syntax error in %q", pattern))
		} else if ok {
			return true
		}
	}
	return false
}

// validPattern must match the parts of a unit or application name
// pattern either side of the '/' for it to be valid.
var validPattern = regexp.MustCompile("^[a-z0-9-*]+$")

// NewUnitMatcher returns a unitMatcher that matches units
// with one of the specified patterns, or all units if no
// patterns are specified.
//
// An error will be returned if any of the specified patterns
// is invalid. Patterns are valid if they contain only
// alpha-numeric characters, hyphens, or asterisks (and one
// optional '/' to separate service/unit).
func NewUnitMatcher(patterns []string) (unitMatcher, error) {
	pattCopy := make([]string, len(patterns))
	for i, pattern := range patterns {
		pattCopy[i] = patterns[i]
		fields := strings.Split(pattern, "/")
		if len(fields) > 2 {
			return unitMatcher{}, fmt.Errorf("pattern %q contains too many '/' characters", pattern)
		}
		for _, f := range fields {
			if !validPattern.MatchString(f) {
				return unitMatcher{}, fmt.Errorf("pattern %q contains invalid characters", pattern)
			}
		}
		if len(fields) == 1 {
			pattCopy[i] += "/*"
		}
	}
	return unitMatcher{pattCopy}, nil
}
