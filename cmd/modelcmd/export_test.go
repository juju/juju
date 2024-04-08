// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/jujuclient"
)

var (
	NewAPIContext         = newAPIContext
	ProcessAccountDetails = processAccountDetails
)

func Interactor(ctx *apiContext) httpbakery.Interactor {
	return ctx.interactor
}

func SetRunStarted(b interface {
	setRunStarted()
}) {
	b.setRunStarted()
}

func InitContexts(c *cmd.Context, b interface {
	initContexts(*cmd.Context)
}) {
	b.initContexts(c)
}

func SetModelRefresh(refresh func(jujuclient.ClientStore, string) error, b interface {
	SetModelRefresh(refresh func(jujuclient.ClientStore, string) error)
}) {
	b.SetModelRefresh(refresh)
}
