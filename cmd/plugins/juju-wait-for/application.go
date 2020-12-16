// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
)

func newApplicationCommand() cmd.Command {
	cmd := &applicationCommand{}
	cmd.newWatchAllAPIFunc = func() (api.WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return watchAllAPIShim{
			Client: client,
		}, nil
	}
	return modelcmd.Wrap(cmd)
}

const applicationCommandDoc = `
Wait for a given application to reach a goal state.
arguments:
name
   application name identifier
options:
--query (= 'life=="alive" && status=="active"')
   query represents the goal state of a given application
`

// applicationCommand defines a command for waiting for applications.
type applicationCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	summary bool

	found   bool
	appInfo params.ApplicationInfo
	units   map[string]*params.UnitInfo
}

// Info implements Command.Info.
func (c *applicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "application",
		Args:    "[<name>]",
		Purpose: "wait for an application to reach a goal state",
		Doc:     applicationCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *applicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="active"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "output a summary of the application query on exit")
}

// Init implements Command.Init.
func (c *applicationCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("application name must be supplied when waiting for an application")
	}
	if len(args) != 1 {
		return errors.New("only one application name can be supplied as an argument to this command")
	}
	if ok := names.IsValidApplication(args[0]); !ok {
		return errors.Errorf("%q is not valid application name", args[0])
	}
	c.name = args[0]

	return nil
}

func (c *applicationCommand) Run(ctx *cmd.Context) error {
	scopedContext := MakeScopeContext()

	defer func() {
		if !c.summary {
			return
		}

		switch c.appInfo.Life {
		case life.Dead:
			ctx.Infof("Application %q has been removed", c.name)
		case life.Dying:
			ctx.Infof("Application %q is being removed", c.name)
		default:
			ctx.Infof("Application %q is running", c.name)
			outputApplicationSummary(ctx.Stdout, scopedContext, &c.appInfo, c.units)
		}
	}()

	strategy := &Strategy{
		ClientFn: c.newWatchAllAPIFunc,
		Timeout:  c.timeout,
	}
	strategy.Subscribe(func(event EventType) {
		switch event {
		case WatchAllStarted:
			c.primeCache()
		}
	})
	err := strategy.Run(c.name, c.query, c.waitFor(scopedContext))
	return errors.Trace(err)
}

func (c *applicationCommand) primeCache() {
	c.units = make(map[string]*params.UnitInfo)
}

func (c *applicationCommand) waitFor(ctx ScopeContext) func(string, []params.Delta, query.Query) (bool, error) {
	return func(name string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

			switch entityInfo := delta.Entity.(type) {
			case *params.ApplicationInfo:
				if entityInfo.Name != name {
					break
				}
				if delta.Removed {
					return false, errors.Errorf("application %v removed", name)
				}

				c.appInfo = *entityInfo

				scope := MakeApplicationScope(ctx, entityInfo)
				if done, err := runQuery(q, scope); err != nil {
					return false, errors.Trace(err)
				} else if done {
					return true, nil
				}

				c.found = entityInfo.Life != life.Dead

			case *params.UnitInfo:
				if delta.Removed {
					delete(c.units, entityInfo.Name)
					break
				}
				if entityInfo.Application == name {
					c.units[entityInfo.Name] = entityInfo
				}
			}
		}

		if !c.found {
			logger.Infof("application %q not found, waiting...", name)
			return false, nil
		}

		currentStatus := c.appInfo.Status.Current
		logOutput := currentStatus.String() != "unset" && len(c.units) > 0

		appInfo := c.appInfo
		appInfo.Status.Current = deriveApplicationStatus(currentStatus, c.units)

		scope := MakeApplicationScope(ctx, &appInfo)
		if done, err := runQuery(q, scope); err != nil {
			return false, errors.Trace(err)
		} else if done {
			return true, nil
		}

		if logOutput {
			logger.Infof("application %q found with %q, waiting for goal state", name, currentStatus)
		}

		return false, nil
	}
}

// ApplicationScope allows the query to introspect a application entity.
type ApplicationScope struct {
	ctx             ScopeContext
	ApplicationInfo *params.ApplicationInfo
}

// MakeApplicationScope creates an ApplicationScope from an ApplicationInfo
func MakeApplicationScope(ctx ScopeContext, info *params.ApplicationInfo) ApplicationScope {
	return ApplicationScope{
		ctx:             ctx,
		ApplicationInfo: info,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m ApplicationScope) GetIdents() []string {
	return getIdents(m.ApplicationInfo)
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m ApplicationScope) GetIdentValue(name string) (query.Box, error) {
	m.ctx.RecordIdent(name)

	switch name {
	case "name":
		return query.NewString(m.ApplicationInfo.Name), nil
	case "life":
		return query.NewString(string(m.ApplicationInfo.Life)), nil
	case "exposed":
		return query.NewBool(m.ApplicationInfo.Exposed), nil
	case "charm-url":
		return query.NewString(m.ApplicationInfo.CharmURL), nil
	case "min-units":
		return query.NewInteger(int64(m.ApplicationInfo.MinUnits)), nil
	case "subordinate":
		return query.NewBool(m.ApplicationInfo.Subordinate), nil
	case "status":
		return query.NewString(string(m.ApplicationInfo.Status.Current)), nil
	case "workload-version":
		return query.NewString(m.ApplicationInfo.WorkloadVersion), nil
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on ApplicationInfo", name)
}

func deriveApplicationStatus(currentStatus status.Status, units map[string]*params.UnitInfo) status.Status {
	// If the application is unset, then derive it from the units.
	if currentStatus.String() != "unset" {
		return currentStatus
	}

	statuses := make([]status.StatusInfo, 0)
	for _, unit := range units {
		agentStatus := unit.WorkloadStatus
		statuses = append(statuses, status.StatusInfo{
			Status: agentStatus.Current,
		})
	}

	derived := status.DeriveStatus(statuses)
	return derived.Status
}

func outputApplicationSummary(writer io.Writer, scopedContext ScopeContext, appInfo *params.ApplicationInfo, units map[string]*params.UnitInfo) {
	result := struct {
		Elements map[string]interface{} `yaml:"properties"`
	}{
		Elements: make(map[string]interface{}),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		// We have to special case status here because of the issue that
		// unset propagates through and we have to read it via the unit
		// information.
		if ident == "status" {
			currentStatus := appInfo.Status.Current
			currentStatus = deriveApplicationStatus(currentStatus, units)
			result.Elements[ident] = currentStatus.String()
			continue
		}

		scope := MakeApplicationScope(scopedContext, appInfo)
		box, err := scope.GetIdentValue(ident)
		if err != nil {
			continue
		}
		result.Elements[ident] = box.Value()
	}

	_ = yaml.NewEncoder(writer).Encode(result)
}
