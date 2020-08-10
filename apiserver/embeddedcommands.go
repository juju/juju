// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
)

// ExecEmbeddedCommandFunc defines a function which runs a named Juju command
// with the whitelisted sub commands.
type ExecEmbeddedCommandFunc func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmd string, args []string) int

// allowedEmbeddedCommands is a whitelist of Juju CLI commands which
// are permissible to run embedded on a controller.
var allowedEmbeddedCommands = []string{
	"actions",
	"add-machine",
	"add-relation",
	"add-space",
	"add-storage",
	"add-subnet",
	"add-unit",
	"add-user",
	"agreements",
	"attach",
	"attach-resource",
	"attach-storage",
	"bind",
	"cached-images",
	"cancel-task",
	"change-user-password",
	"charm-resources",
	"clouds",
	"config",
	"consume",
	"controller-config",
	"create-storage-pool",
	"credentials",
	"deploy",
	"detach-storage",
	"disable-user",
	"enable-user",
	"expose",
	"find-offers",
	"firewall-rules",
	"get-constraints",
	"get-model-constraints",
	// TODO(wallyworld) - check if these should be allowed
	//"grant",
	//"grant-cloud",
	"help",
	"import-filesystem",
	"machines",
	"metrics",
	"model-config",
	"model-default",
	"model-defaults",
	"move-to-space",
	"offer",
	"offers",
	"payloads",
	"plans",
	"register",
	"relate",
	"reload-spaces",
	"remove-application",
	"remove-cached-images",
	"remove-consumed-application",
	"remove-credential",
	"remove-machine",
	"remove-offer",
	"remove-relation",
	"remove-saas",
	"remove-space",
	"remove-storage",
	"remove-storage-pool",
	"remove-unit",
	"remove-user",
	"rename-space",
	"resolved",
	"resolve",
	"resources",
	"resume-relation",
	"retry-provisioning",
	"revoke",
	"run",
	"scale-application",
	"set-credential",
	"set-constraints",
	"set-firewall-rule",
	"set-meter-status",
	"set-model-constraints",
	"set-plan",
	"set-series",
	"set-wallet",
	"show-action",
	"show-application",
	"show-cloud",
	"show-controller",
	"show-credential",
	"show-credentials",
	"show-machine",
	"show-model",
	"show-offer",
	"show-status",
	"show-status-log",
	"show-storage",
	"show-space",
	"show-unit",
	"show-user",
	"show-wallet",
	"sla",
	"spaces",
	"status",
	"storage",
	"storage-pools",
	"subnets",
	"suspend-relation",
	"trust",
	"unexpose",
	"update-storage-pool",
	"users",
	"wallets",
}

// runCLICommands creates a CLI command instance with an in-memory copy of the controller,
// model, and account details and runs the command against the host controller.
func (srv *Server) runCLICommands(commands params.CLICommands) (params.StringResults, error) {
	result := params.StringResults{}
	if commands.AuthTag == "" {
		return result, errors.NotSupportedf("CLI command for anonymous user")
	}
	userTag, err := names.ParseTag(commands.AuthTag)
	if err != nil {
		return result, errors.Trace(err)
	}
	if userTag.Kind() != names.UserTagKind {
		return result, errors.NotSupportedf("CLI command for tag %q", userTag.Kind())
	}

	var resolvedModelUUID string
	if commands.ModelTag == "" {
		resolvedModelUUID = srv.shared.statePool.SystemState().ModelUUID()
	} else {
		modelTag, err := names.ParseModelTag(commands.ModelTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		resolvedModelUUID = modelTag.Id()
	}
	m, closer, err := srv.shared.statePool.GetModel(resolvedModelUUID)
	if err != nil {
		return result, errors.Trace(err)
	}
	defer closer.Release()

	cfg, err := m.State().ControllerConfig()
	if err != nil {
		return result, err
	}

	// Set up a juju client store used to configure the
	// embedded command to give it the controller, model
	// and account details to use.
	store := jujuclient.NewMemStore()
	cert, _ := cfg.CACert()
	controllerName := cfg.ControllerName()
	if controllerName == "" {
		controllerName = "interactive"
	}
	store.Controllers[controllerName] = jujuclient.ControllerDetails{
		ControllerUUID: cfg.ControllerUUID(),
		APIEndpoints:   []string{fmt.Sprintf("localhost:%d", cfg.APIPort())},
		CACert:         cert,
	}
	store.CurrentControllerName = controllerName

	qualifiedModelName := jujuclient.JoinOwnerModelName(m.Owner(), m.Name())
	store.Models[controllerName] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			qualifiedModelName: {
				ModelUUID:    resolvedModelUUID,
				ModelType:    model.ModelType(m.Type()),
				ActiveBranch: commands.ActiveBranch,
			},
		},
		CurrentModel: qualifiedModelName,
	}
	store.Accounts[controllerName] = jujuclient.AccountDetails{
		User:      userTag.Id(),
		Password:  commands.Credentials,
		Macaroons: commands.Macaroons,
	}

	result.Results = make([]params.StringResult, len(commands.Commands))
	for i, cliCmd := range commands.Commands {
		out, err := srv.runCLICommand(cliCmd, store)
		result.Results[i] = params.StringResult{
			Error:  apiservererrors.ServerError(err),
			Result: out,
		}
	}
	return result, nil
}

func (srv *Server) runCLICommand(cliCmd params.CLICommandArgs, store jujuclient.ClientStore) (string, error) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		return "", errors.Trace(err)
	}
	var buf []byte
	out := bytes.NewBuffer(buf)
	ctx.Stdout = out
	ctx.Stderr = out
	code := srv.execEmbeddedCommand(ctx, store, allowedEmbeddedCommands, cliCmd.Command, cliCmd.Args)
	if code == 0 {
		return out.String(), nil
	}
	return "", errors.New(out.String())
}
