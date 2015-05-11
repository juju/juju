// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build windows !linux

package windows

import (
	"code.google.com/p/winsvc/svc"
	"os"
)

type systemService struct {
	name string
	cmd  func(args []string)
	args []string
}

// Execute implements the svc.Handler interface
func (s *systemService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEr bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	go func() {
		s.cmd(s.args)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				os.Exit(0)
			default:
				continue
			}
		}
	}
	return
}

// Run runs the service
func (s *systemService) Run() error {
	return svc.Run(s.name, s)
}

// NewSystemService returns a systemService type that is responsible for managing
// the life-cycle of the service
func NewSystemService(name string, f func(args []string), args []string) *systemService {
	return &systemService{
		name: name,
		cmd:  f,
		args: os.Args,
	}
}
