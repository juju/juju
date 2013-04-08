package tools

import (
	"errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"sort"
	"strings"
)

// List is an unordered list of tools available in an environment.
type List []*state.Tools

func (src List) String() string {
	names := make([]string, len(src))
	for i, tools := range src {
		names[i] = tools.Binary.String()
	}
	return strings.Join(names, ";")
}

// Series returns all series for which some tools in src were built.
func (src List) Series() []string {
	return src.collect(func(seen map[string]bool, tools *state.Tools) {
		seen[tools.Series] = true
	})
}

// Arches returns all architectures for which some tools in src were built.
func (src List) Arches() []string {
	return src.collect(func(seen map[string]bool, tools *state.Tools) {
		seen[tools.Arch] = true
	})
}

func (src List) collect(f func(map[string]bool, *state.Tools)) []string {
	seen := map[string]bool{}
	for _, tools := range src {
		f(seen, tools)
	}
	result := []string{}
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// Newest returns a List, derived from src, containing only those tools
// which have a version number greater than or equal to all other tools
// in the list.
func (src List) Newest() List {
	best := src[0].Number
	var result List
	for _, tools := range src {
		if best.Less(tools.Number) {
			best = tools.Number
			result = List{tools}
		} else if tools.Number == best {
			result = append(result, tools)
		}
	}
	return result
}

// Difference returns a List, derived from src, containing only tools with
// binary versions not found in the supplied List.
func (src List) Difference(excluded List) List {
	ignore := make(map[version.Binary]bool, len(excluded))
	for _, tool := range excluded {
		ignore[tool.Binary] = true
	}
	var result List
	for _, tool := range src {
		if !ignore[tool.Binary] {
			result = append(result, tool)
		}
	}
	return result
}

var ErrNoMatches = errors.New("no matching tools available")

// Filter returns a List, derived from src, containing only those tools that
// match the supplied Filter. If no tools match, it returns ErrNoMatches.
func (src List) Filter(f Filter) (List, error) {
	var result List
	for _, tools := range src {
		if f.match(tools) {
			result = append(result, tools)
		}
	}
	if len(result) == 0 {
		return nil, ErrNoMatches
	}
	return result, nil
}

// Filter parameterises List.Filter.
type Filter struct {

	// Release, if true, ignores all tools whose version number indicates
	// that they're development versions.
	Released bool

	// Number, if non-zero, ignores all tools without that version number.
	Number version.Number

	// Series, if not empty, ignores all tools not built for that series.
	Series string

	// Arch, if not empty, ignores all tools not built for that architecture.
	Arch string
}

// match returns true if the supplied tools match all non-zero fields in f.
func (f Filter) match(tools *state.Tools) bool {
	if f.Released && tools.IsDev() {
		return false
	}
	if f.Number != (version.Number{}) && tools.Number != f.Number {
		return false
	}
	if f.Series != "" && tools.Series != f.Series {
		return false
	}
	if f.Arch != "" && tools.Arch != f.Arch {
		return false
	}
	return true
}
