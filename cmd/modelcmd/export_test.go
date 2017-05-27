package modelcmd

import "github.com/juju/cmd"

var NewAPIContext = newAPIContext

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
