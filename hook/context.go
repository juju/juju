package hook

import (
	"fmt"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/state"
	"strings"
)

// Context holds all information necessary to run a hook command.
type Context struct {
	state    *state.State
	local    string
	relation string
	members  []string
}

// NewLocalContext returns a Context tied to a specific unit.
func NewLocalContext(state *state.State, unitName string) *Context {
	return &Context{state, unitName, "", nil}
}

// NewRelationContext returns a Context tied to a specific unit relation .
func NewRelationContext(
	state *state.State, unitName, relationName string, members []string,
) *Context {
	return &Context{state, unitName, relationName, members}
}

// NewBrokenContext returns a Context tied to a specific unit relation which is
// known to have been broken.
func NewBrokenContext(state *state.State, unitName, relationName string) *Context {
	return &Context{state, unitName, relationName, nil}
}

// Vars returns an os.Environ-style list of strings that should be set when
// executing a hook in this Context.
func (ctx *Context) Vars() []string {
	vars := []string{}
	if ctx.local != "" {
		vars = append(vars, "JUJU_UNIT_NAME="+ctx.local)
	}
	if ctx.relation != "" {
		vars = append(vars, "JUJU_RELATION="+ctx.relation)
	}
	return vars
}

// Log is the core of the `log` hook command, and is always meaningful in any
// Context.
func (ctx *Context) Log(debug bool, msg string) {
	s := []string{}
	if ctx.local != "" {
		s = append(s, ctx.local)
	}
	if ctx.relation != "" {
		s = append(s, ctx.relation)
	}
	full := fmt.Sprintf("Context<%s>: %s", strings.Join(s, ", "), msg)
	if debug {
		log.Debugf(full)
	} else {
		log.Printf(full)
	}
}

// Members is the core of the `relation-list` hook command. It returns a list
// of unit names (exluding the local unit) participating in this context's
// relation, and is always meaningful in any Context.
func (ctx *Context) Members() []string {
	if ctx.members != nil {
		return ctx.members
	}
	return []string{}
}
