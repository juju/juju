// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"net"
	"path"
	"regexp"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

var InvalidFormatErr = errors.Errorf("the given filter did not match any known patterns.")

// UnitChainPredicateFn builds a function which runs the given
// predicate over a unit and all of its subordinates. If one unit in
// the chain matches, the entire chain matches.
func UnitChainPredicateFn(
	predicate Predicate,
	getUnit func(string) *state.Unit,
) func(*state.Unit) (bool, error) {
	considered := make(map[string]bool)
	var f func(unit *state.Unit) (bool, error)
	f = func(unit *state.Unit) (bool, error) {
		// Don't try and filter the same unit 2x.
		if matches, ok := considered[unit.Name()]; ok {
			logger.Debugf("%s has already been examined and found to be: %t", unit.Name(), matches)
			return matches, nil
		}

		// Check the current unit.
		matches, err := predicate(unit)
		if err != nil {
			return false, errors.Annotate(err, "could not filter units")
		}
		considered[unit.Name()] = matches

		// Now check all of this unit's subordinates.
		for _, subName := range unit.SubordinateNames() {
			// A master match supercedes any subordinate match.
			if matches {
				logger.Infof("%s is a subordinate to a match.", subName)
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
func BuildPredicateFor(patterns []string) Predicate {

	or := func(predicates ...closurePredicate) (bool, error) {
		// Differentiate between a valid format that elimintated all
		// elements, and an invalid query.
		oneValidFmt := false
		for _, p := range predicates {
			if matches, ok, err := p(); err != nil {
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

	return func(i interface{}) (bool, error) {
		switch i.(type) {
		default:
			panic(errors.Errorf("Programming error. We should only ever pass in machines, services, or units. Received %T.", i))
		case *state.Machine:
			shims, err := buildMachineMatcherShims(i.(*state.Machine), patterns)
			if err != nil {
				return false, err
			}
			return or(shims...)
		case *state.Unit:
			return or(buildUnitMatcherShims(i.(*state.Unit), patterns)...)
		case *state.Service:
			shims, err := buildServiceMatcherShims(i.(*state.Service), patterns...)
			if err != nil {
				return false, err
			}
			return or(shims...)
		}
	}
}

// Predicate is a function that when given a unit, machine, or
// service, will determine whether the unit meets some criteria.
type Predicate func(interface{}) (matches bool, _ error)

// closurePredicate is a function which has at some point been closed
// around an element so that it can examine whether this element
// matches some criteria.
type closurePredicate func() (matches bool, formatOK bool, _ error)

func unitMatchUnitName(u *state.Unit, patterns []string) (bool, bool, error) {
	um, err := NewUnitMatcher(patterns)
	if err != nil {
		// Currently, the only error possible here is a matching
		// error. We don't want this error to hold up further
		// matching.
		logger.Debugf("ignoring matching error: %v", err)
		return false, false, nil
	}
	return um.matchUnit(u), true, nil
}

func unitMatchAgentStatus(u *state.Unit, patterns []string) (bool, bool, error) {
	statusInfo, err := u.AgentStatus()
	if err != nil {
		return false, false, err
	}
	return matchAgentStatus(patterns, statusInfo.Status)
}

func unitMatchWorkloadStatus(u *state.Unit, patterns []string) (bool, bool, error) {
	workloadStatusInfo, err := u.Status()
	if err != nil {
		return false, false, err
	}
	agentStatusInfo, err := u.AgentStatus()
	if err != nil {
		return false, false, err
	}
	return matchWorkloadStatus(patterns, workloadStatusInfo.Status, agentStatusInfo.Status)
}

func unitMatchExposure(u *state.Unit, patterns []string) (bool, bool, error) {
	s, err := u.Service()
	if err != nil {
		return false, false, err
	}
	return matchExposure(patterns, s)
}

func unitMatchSubnet(u *state.Unit, patterns []string) (bool, bool, error) {
	pub, pubOK := u.PublicAddress()
	priv, privOK := u.PrivateAddress()
	if !pubOK && !privOK {
		return true, false, nil
	}
	return matchSubnet(patterns, pub, priv)
}

func unitMatchPort(u *state.Unit, patterns []string) (bool, bool, error) {
	portRanges, err := u.OpenedPorts()
	if err != nil {
		return false, false, err
	}
	return matchPortRanges(patterns, portRanges...)
}

func buildServiceMatcherShims(s *state.Service, patterns ...string) (shims []closurePredicate, _ error) {
	// Match on name.
	shims = append(shims, func() (bool, bool, error) {
		for _, p := range patterns {
			if strings.ToLower(s.Name()) == strings.ToLower(p) {
				return true, true, nil
			}
		}
		return false, false, nil
	})

	// Match on exposure.
	shims = append(shims, func() (bool, bool, error) { return matchExposure(patterns, s) })

	// Match on network addresses.
	networks, err := s.Networks()
	if err != nil {
		return nil, err
	}
	shims = append(shims, func() (bool, bool, error) { return matchSubnet(patterns, networks...) })

	// If the service has an unit instance that matches any of the
	// given criteria, consider the service a match as well.
	unitShims, err := buildShimsForUnit(s.AllUnits, patterns...)
	if err != nil {
		return nil, err
	}
	shims = append(shims, unitShims...)

	// Units may be able to match the pattern. Ultimately defer to
	// that logic, and guard against breaking the predicate-chain.
	if len(unitShims) <= 0 {
		shims = append(shims, func() (bool, bool, error) { return false, true, nil })
	}

	return shims, nil
}

func buildShimsForUnit(unitsFn func() ([]*state.Unit, error), patterns ...string) (shims []closurePredicate, _ error) {
	units, err := unitsFn()
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		shims = append(shims, buildUnitMatcherShims(u, patterns)...)
	}
	return shims, nil
}

func buildMachineMatcherShims(m *state.Machine, patterns []string) (shims []closurePredicate, _ error) {
	// Look at machine status.
	statusInfo, err := m.Status()
	if err != nil {
		return nil, err
	}
	shims = append(shims, func() (bool, bool, error) { return matchAgentStatus(patterns, statusInfo.Status) })

	// Look at machine addresses. WARNING: Avoid the temptation to
	// bring the append into the loop. The value we would close over
	// will continue to change after the closure is created, and we'd
	// only examine the last element of the loop for all closures.
	var addrs []string
	for _, a := range m.Addresses() {
		addrs = append(addrs, a.Value)
	}
	shims = append(shims, func() (bool, bool, error) { return matchSubnet(patterns, addrs...) })

	// If the machine hosts a unit that matches any of the given
	// criteria, consider the machine a match as well.
	unitShims, err := buildShimsForUnit(m.Units, patterns...)
	if err != nil {
		return nil, err
	}
	shims = append(shims, unitShims...)

	// Units may be able to match the pattern. Ultimately defer to
	// that logic, and guard against breaking the predicate-chain.
	if len(unitShims) <= 0 {
		shims = append(shims, func() (bool, bool, error) { return false, true, nil })
	}

	return
}

func buildUnitMatcherShims(u *state.Unit, patterns []string) []closurePredicate {
	closeOver := func(f func(*state.Unit, []string) (bool, bool, error)) closurePredicate {
		return func() (bool, bool, error) { return f(u, patterns) }
	}
	return []closurePredicate{
		closeOver(unitMatchUnitName),
		closeOver(unitMatchAgentStatus),
		closeOver(unitMatchWorkloadStatus),
		closeOver(unitMatchExposure),
		closeOver(unitMatchSubnet),
		closeOver(unitMatchPort),
	}
}

func matchPortRanges(patterns []string, portRanges ...network.PortRange) (bool, bool, error) {
	for _, p := range portRanges {
		for _, patt := range patterns {
			if strings.HasPrefix(p.String(), patt) {
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
			ip, err := net.ResolveIPAddr("ip", a)
			if err != nil {
				errors.Trace(errors.Annotate(err, "could not parse machine's address"))
				continue
			} else if pip, err := net.ResolveIPAddr("ip", p); err == nil {
				oneValidPattern = true
				if ip.IP.Equal(pip.IP) {
					return true, true, nil
				}
			} else if pip := net.ParseIP(p); pip != nil {
				oneValidPattern = true
				if ip.IP.Equal(pip) {
					return true, true, nil
				}
			} else if _, ipNet, err := net.ParseCIDR(p); err == nil {
				oneValidPattern = true
				if ipNet.Contains(ip.IP) {
					return true, true, nil
				}
			}
		}
	}
	return false, oneValidPattern, nil
}

func matchExposure(patterns []string, s *state.Service) (bool, bool, error) {
	if len(patterns) >= 1 && patterns[0] == "exposed" {
		return s.IsExposed(), true, nil
	} else if len(patterns) >= 2 && patterns[0] == "not" && patterns[1] == "exposed" {
		return !s.IsExposed(), true, nil
	}
	return false, false, nil
}

func matchWorkloadStatus(patterns []string, workloadStatus state.Status, agentStatus state.Status) (bool, bool, error) {
	oneValidStatus := false
	for _, p := range patterns {
		// If the pattern isn't a known status, ignore it.
		ps := state.Status(p)
		if !ps.KnownWorkloadStatus() {
			continue
		}

		oneValidStatus = true
		// To preserve current expected behaviour, we only report on workload status
		// if the agent itself is not in error.
		if agentStatus != state.StatusError && workloadStatus.WorkloadMatches(ps) {
			return true, true, nil
		}
	}
	return false, oneValidStatus, nil
}

func matchAgentStatus(patterns []string, status state.Status) (bool, bool, error) {
	oneValidStatus := false
	for _, p := range patterns {
		// If the pattern isn't a known status, ignore it.
		ps := state.Status(p)
		if !ps.KnownAgentStatus() {
			continue
		}

		oneValidStatus = true
		if status.Matches(ps) {
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

// validPattern must match the parts of a unit or service name
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
