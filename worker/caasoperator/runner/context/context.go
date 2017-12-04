// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package context contains the ContextFactory and Context definitions. Context implements
// hooks.Context and is used together with caasoperator.Runner to run hooks, commands and actions.
package context

import (
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/caasoperator/commands"
)

// Paths exposes the paths needed by Context.
type Paths interface {

	// GetToolsDir returns the filesystem path to the dirctory containing
	// the hook tool symlinks.
	GetToolsDir() string

	// GetCharmDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetCharmDir() string

	// GetHookCommandSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing operator process. It might be a
	// filesystem path, or it might be abstract.
	GetHookCommandSocket() string

	// GetMetricsSpoolDir returns the path to a metrics spool dir, used
	// to store metrics recorded during a single hook run.
	GetMetricsSpoolDir() string
}

var logger = loggo.GetLogger("juju.worker.caasoperator.runner.context")
var mutex = sync.Mutex{}

// HookProcess is an interface representing a process running a hook.
type HookProcess interface {
	Pid() int
	Kill() error
}

// HookContext is the implementation of hooks.Context.
type HookContext struct {
	hookAPI hookAPI

	// configSettings holds the service configuration.
	configSettings charm.Settings

	// id identifies the context.
	id string

	// uuid is the universally unique identifier of the environment.
	uuid string

	// modelName is the human friendly name of the environment.
	modelName string

	// applicationName is the name of the application.
	applicationName string

	// status is the status of the application.
	status *commands.StatusInfo

	// relationId identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the relations map.
	relationId int

	// remoteUnitName identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	remoteUnitName string

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

	// apiAddrs contains the API server addresses.
	apiAddrs []string

	// proxySettings are the current proxy settings that the operator knows about.
	proxySettings proxy.Settings

	// process is the process of the command that is being run in the local context,
	// like a juju-run command or a hook
	process HookProcess

	// clock is used for any time operations.
	clock clock.Clock
}

func (ctx *HookContext) GetProcess() HookProcess {
	mutex.Lock()
	defer mutex.Unlock()
	return ctx.process
}

func (ctx *HookContext) SetProcess(process HookProcess) {
	mutex.Lock()
	defer mutex.Unlock()
	ctx.process = process
}

func (ctx *HookContext) Id() string {
	return ctx.id
}

func (ctx *HookContext) ApplicationName() string {
	return ctx.applicationName
}

// ApplicationStatus returns the status for the application.
func (ctx *HookContext) ApplicationStatus() (commands.StatusInfo, error) {
	if ctx.status != nil {
		return *ctx.status, nil
	}
	var err error
	status, err := ctx.hookAPI.ApplicationStatus()
	if err == nil && status.Error != nil {
		err = status.Error
	}
	if err != nil {
		return commands.StatusInfo{}, errors.Trace(err)
	}
	ctx.status = &commands.StatusInfo{
		Tag:    names.NewApplicationTag(ctx.applicationName).String(),
		Status: string(status.Application.Status),
		Info:   status.Application.Info,
		Data:   status.Application.Data,
	}
	return *ctx.status, nil
}

// SetApplicationStatus will set the given application status.
func (ctx *HookContext) SetApplicationStatus(appStatus commands.StatusInfo) error {
	logger.Tracef("[APPLICATION-STATUS] %s: %s", appStatus.Status, appStatus.Info)

	return ctx.hookAPI.SetApplicationStatus(
		status.Status(appStatus.Status),
		appStatus.Info,
		appStatus.Data,
	)
}

func (ctx *HookContext) ApplicationConfig() (charm.Settings, error) {
	if ctx.configSettings == nil {
		var err error
		ctx.configSettings, err = ctx.hookAPI.ApplicationConfig()
		if err != nil {
			return nil, err
		}
	}
	result := charm.Settings{}
	for name, value := range ctx.configSettings {
		result[name] = value
	}
	return result, nil
}

func (ctx *HookContext) HookRelation() (commands.ContextRelation, error) {
	return ctx.Relation(ctx.relationId)
}

func (ctx *HookContext) RemoteUnitName() (string, error) {
	if ctx.remoteUnitName == "" {
		return "", errors.NotFoundf("remote unit")
	}
	return ctx.remoteUnitName, nil
}

func (ctx *HookContext) Relation(id int) (commands.ContextRelation, error) {
	r, found := ctx.relations[id]
	if !found {
		return nil, errors.NotFoundf("relation")
	}
	return r, nil
}

func (ctx *HookContext) RelationIds() ([]int, error) {
	ids := []int{}
	for id := range ctx.relations {
		ids = append(ids, id)
	}
	return ids, nil
}

// HookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into context.
func (context *HookContext) HookVars(paths Paths) ([]string, error) {
	vars := context.proxySettings.AsEnvironmentValues()
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+context.id,
		"JUJU_AGENT_SOCKET="+paths.GetHookCommandSocket(),
		"JUJU_APPLICATION_NAME="+context.applicationName,
		"JUJU_MODEL_UUID="+context.uuid,
		"JUJU_MODEL_NAME="+context.modelName,
		"JUJU_API_ADDRESSES="+strings.Join(context.apiAddrs, " "),
		"JUJU_VERSION="+version.Current.String(),
	)
	if r, err := context.HookRelation(); err == nil {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+context.remoteUnitName,
		)
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return append(vars, OSEnvVars(paths)...), nil
}

// Prepare implements the Context interface.
func (ctx *HookContext) Prepare() error {
	return nil
}

// Flush implements the Context interface.
func (ctx *HookContext) Flush(process string, ctxErr error) (err error) {
	writeChanges := ctxErr == nil

	for id, rctx := range ctx.relations {
		if writeChanges {
			if e := rctx.WriteSettings(); e != nil {
				e = errors.Errorf(
					"could not write settings from %q to relation %d: %v",
					process, id, e,
				)
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
	}

	// TODO (tasdomas) 2014 09 03: context finalization needs to modified to apply all
	//                             changes in one api call to minimize the risk
	//                             of partial failures.

	if !writeChanges {
		return ctxErr
	}

	return ctxErr
}

// NetworkInfo returns the network info for the given bindings on the given relation.
func (ctx *HookContext) NetworkInfo(bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	var relId *int
	if relationId != -1 {
		relId = &relationId
	}
	return ctx.hookAPI.NetworkInfo(bindingNames, relId)
}

// SetContainerSpec updates the yaml spec used to create a container.
func (ctx *HookContext) SetContainerSpec(specYaml, unitName string) error {
	return ctx.hookAPI.SetContainerSpec(specYaml, unitName)
}
