// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/juju/tc"
)

var quotedStringRe = regexp.MustCompile(`"([^"]+)"`)

func (s *PhaseInternalSuite) TestTLASpecMatchesGo(c *tc.C) {
	specPath := filepath.Join("tla", "MigrationTransitionPhases.tla")
	specBytes, err := os.ReadFile(specPath)
	c.Assert(err, tc.ErrorIsNil)
	spec := string(specBytes)

	tlaPhases, err := parseTLANamedSet(spec, "Phases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaPhases, tc.DeepEquals, sortedUniqueStrings(phaseNames))

	initialPhase, err := parseTLANamedString(spec, "InitialPhase")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(initialPhase, tc.Equals, QUIESCE.String())

	tlaTransitions, err := parseTLATransitions(spec)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaTransitions, tc.DeepEquals, goTransitionNames())

	tlaRunning, err := parseTLANamedSet(spec, "RunningPhases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaRunning, tc.DeepEquals, goRunningPhaseNames())

	tlaPostSuccess, err := parseTLANamedSet(spec, "PostSuccessPhases")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tlaPostSuccess, tc.DeepEquals, goPostSuccessPhaseNames())

	maxTransitions, err := parseTLANamedInt(spec, "MaxTransitions")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(maxTransitions, tc.Equals, longestTransitionPath(QUIESCE))
}

func (s *PhaseInternalSuite) TestTLAConfigMatchesExpectedChecks(c *tc.C) {
	cfgPath := filepath.Join("tla", "MigrationTransitionPhases.cfg")
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
	matches := quotedStringRe.FindAllStringSubmatch(value, -1)
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

	for _, raw := range strings.Split(cfgText, "\n") {
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

func goTransitionNames() map[string][]string {
	result := make(map[string][]string, len(validTransitions))
	for source, targets := range validTransitions {
		targetNames := make([]string, len(targets))
		for i, target := range targets {
			targetNames[i] = target.String()
		}
		result[source.String()] = sortedUniqueStrings(targetNames)
	}
	return result
}

func goRunningPhaseNames() []string {
	var phases []string
	for idx := range phaseNames {
		phase := Phase(idx)
		if phase.IsRunning() {
			phases = append(phases, phase.String())
		}
	}
	return sortedUniqueStrings(phases)
}

func goPostSuccessPhaseNames() []string {
	var phases []string
	for idx := range phaseNames {
		phase := Phase(idx)
		if phase.IsPostSuccess() {
			phases = append(phases, phase.String())
		}
	}
	return sortedUniqueStrings(phases)
}

func longestTransitionPath(start Phase) int {
	memo := make(map[Phase]int)
	var dfs func(Phase) int
	dfs = func(phase Phase) int {
		if depth, exists := memo[phase]; exists {
			return depth
		}
		targets := validTransitions[phase]
		if len(targets) == 0 {
			return 0
		}

		maxDepth := 0
		for _, next := range targets {
			depth := 1 + dfs(next)
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		memo[phase] = maxDepth
		return maxDepth
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
