// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/juju/tc"
)

var tlaQuotedStringRe = regexp.MustCompile(`"([^"]+)"`)

func (s *phaseSuite) TestTLASpecMatchesGo(c *tc.C) {
	specPath := filepath.Join("tla", "ObjectStoreTransitionPhases.tla")
	specBytes, err := os.ReadFile(specPath)
	c.Assert(err, tc.ErrorIsNil)
	spec := string(specBytes)

	tlaPhases, err := parseTLANamedSet(spec, "Phases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaPhases, tc.DeepEquals, allPhaseNames())

	initialPhase, err := parseTLANamedString(spec, "InitialPhase")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initialPhase, tc.Equals, PhaseUnknown.String())

	tlaTransitions, err := parseTLATransitions(spec)
	c.Assert(err, tc.ErrorIsNil)
	goTransitions, err := goTransitionNames()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaTransitions, tc.DeepEquals, goTransitions)

	c.Check(
		terminalPhasesFromTransitions(tlaPhases, tlaTransitions),
		tc.DeepEquals,
		goTerminalPhaseNames(),
	)

	tlaNotStarted, err := parseTLANamedSet(spec, "NotStartedPhases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaNotStarted, tc.DeepEquals, goNotStartedPhaseNames())

	tlaDraining, err := parseTLANamedSet(spec, "DrainingPhases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaDraining, tc.DeepEquals, goDrainingPhaseNames())

	maxTransitions, err := parseTLANamedInt(spec, "MaxTransitions")
	c.Assert(err, tc.ErrorIsNil)
	longestPath, err := longestTransitionPath(PhaseUnknown.String(), goTransitions)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(maxTransitions, tc.Equals, longestPath)
}

func (s *phaseSuite) TestTLAConfigMatchesExpectedChecks(c *tc.C) {
	cfgPath := filepath.Join("tla", "ObjectStoreTransitionPhases.cfg")
	cfgBytes, err := os.ReadFile(cfgPath)
	c.Assert(err, tc.ErrorIsNil)

	cfg, err := parseTLAConfig(string(cfgBytes))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cfg.specification, tc.Equals, "Spec")
	c.Check(cfg.invariants, tc.DeepEquals, []string{
		"TypeInvariant",
		"StepBoundInvariant",
	})
	c.Check(cfg.properties, tc.DeepEquals, []string{
		"TransitionSafety",
		"EventuallyTerminal",
	})
}

func parseTLANamedSet(spec, name string) ([]string, error) {
	pattern := regexp.MustCompile(
		fmt.Sprintf(`(?ms)\b%s\s*==\s*\{([^}]*)\}`, regexp.QuoteMeta(name)),
	)
	matches := pattern.FindStringSubmatch(spec)
	if len(matches) != 2 {
		return nil, fmt.Errorf("%s set definition not found", name)
	}
	return sortedUniqueStrings(parseQuotedStrings(matches[1])), nil
}

func parseTLANamedString(spec, name string) (string, error) {
	pattern := regexp.MustCompile(
		fmt.Sprintf(`(?m)\b%s\s*==\s*"([^"]+)"`, regexp.QuoteMeta(name)),
	)
	matches := pattern.FindStringSubmatch(spec)
	if len(matches) != 2 {
		return "", fmt.Errorf("%s string definition not found", name)
	}
	return matches[1], nil
}

func parseTLANamedInt(spec, name string) (int, error) {
	pattern := regexp.MustCompile(
		fmt.Sprintf(`(?m)\b%s\s*==\s*(\d+)`, regexp.QuoteMeta(name)),
	)
	matches := pattern.FindStringSubmatch(spec)
	if len(matches) != 2 {
		return 0, fmt.Errorf("%s integer definition not found", name)
	}
	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", name, err)
	}
	return value, nil
}

func parseTLATransitions(spec string) (map[string][]string, error) {
	const transitionPattern = `(?m)p\s*=\s*"([^"]+)"\s*->\s*\{([^}]*)\}`
	matches := regexp.MustCompile(transitionPattern).FindAllStringSubmatch(spec, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("transition definitions not found")
	}

	transitions := make(map[string][]string, len(matches))
	for _, match := range matches {
		source := match[1]
		if _, exists := transitions[source]; exists {
			return nil, fmt.Errorf("duplicate transition definition for %q", source)
		}
		transitions[source] = sortedUniqueStrings(parseQuotedStrings(match[2]))
	}
	return transitions, nil
}

func parseQuotedStrings(value string) []string {
	matches := tlaQuotedStringRe.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, match[1])
	}
	return result
}

func parseTLAConfig(cfgText string) (tlaConfig, error) {
	cfg := tlaConfig{}
	section := ""

	for raw := range strings.SplitSeq(cfgText, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, `\*`) {
			continue
		}

		switch {
		case strings.HasPrefix(line, "SPECIFICATION "):
			fields := strings.Fields(line)
			if len(fields) != 2 {
				return tlaConfig{}, fmt.Errorf("invalid specification line %q", line)
			}
			cfg.specification = fields[1]
			section = ""
		case line == "INVARIANTS":
			section = "invariants"
		case line == "PROPERTIES":
			section = "properties"
		default:
			switch section {
			case "invariants":
				cfg.invariants = append(cfg.invariants, line)
			case "properties":
				cfg.properties = append(cfg.properties, line)
			default:
				return tlaConfig{}, fmt.Errorf("unexpected config line %q", line)
			}
		}
	}

	if cfg.specification == "" {
		return tlaConfig{}, fmt.Errorf("missing specification")
	}
	return cfg, nil
}

func goTransitionNames() (map[string][]string, error) {
	transitions := make(map[string][]string)
	phases := allPhases()

	for _, source := range phases {
		var allowed []string
		for _, target := range phases {
			newPhase, err := source.TransitionTo(target)
			switch {
			case err == nil:
				candidate := newPhase.String()
				if candidate == "" {
					candidate = target.String()
				}
				allowed = append(allowed, candidate)
			case errors.Is(err, ErrInvalidTransition), errors.Is(err, ErrTerminalPhase):
				continue
			default:
				return nil, err
			}
		}
		if len(allowed) == 0 {
			continue
		}
		transitions[source.String()] = sortedUniqueStrings(allowed)
	}
	return transitions, nil
}

func terminalPhasesFromTransitions(phases []string, transitions map[string][]string) []string {
	var terminals []string
	for _, phase := range phases {
		if len(transitions[phase]) == 0 {
			terminals = append(terminals, phase)
		}
	}
	return sortedUniqueStrings(terminals)
}

func goTerminalPhaseNames() []string {
	var phases []string
	for _, phase := range allPhases() {
		if phase.IsTerminal() {
			phases = append(phases, phase.String())
		}
	}
	return sortedUniqueStrings(phases)
}

func goNotStartedPhaseNames() []string {
	var phases []string
	for _, phase := range allPhases() {
		if phase.IsNotStarted() {
			phases = append(phases, phase.String())
		}
	}
	return sortedUniqueStrings(phases)
}

func goDrainingPhaseNames() []string {
	var phases []string
	for _, phase := range allPhases() {
		if phase.IsDraining() {
			phases = append(phases, phase.String())
		}
	}
	return sortedUniqueStrings(phases)
}

func allPhases() []Phase {
	return []Phase{
		PhaseUnknown,
		PhaseDraining,
		PhaseError,
		PhaseCompleted,
	}
}

func allPhaseNames() []string {
	names := make([]string, 0, len(allPhases()))
	for _, phase := range allPhases() {
		names = append(names, phase.String())
	}
	return sortedUniqueStrings(names)
}

func longestTransitionPath(start string, transitions map[string][]string) (int, error) {
	memo := make(map[string]int)
	visiting := make(map[string]struct{})

	var dfs func(string) (int, error)
	dfs = func(phase string) (int, error) {
		if depth, exists := memo[phase]; exists {
			return depth, nil
		}
		if _, exists := visiting[phase]; exists {
			return 0, fmt.Errorf("cycle detected at phase %q", phase)
		}
		visiting[phase] = struct{}{}
		defer delete(visiting, phase)

		targets := transitions[phase]
		if len(targets) == 0 {
			return 0, nil
		}

		maxDepth := 0
		for _, target := range targets {
			depth, err := dfs(target)
			if err != nil {
				return 0, err
			}
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		}

		memo[phase] = maxDepth
		return maxDepth, nil
	}

	return dfs(start)
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

type tlaConfig struct {
	specification string
	invariants    []string
	properties    []string
}
