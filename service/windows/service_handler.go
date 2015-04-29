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
	cmd  func() int
}

func (s *systemService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEr bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	retCode := make(chan int, 1)

	go func() {
		retCode <- s.cmd()
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case exitCode := <-retCode:
			os.Exit(exitCode)
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

func (s *systemService) Run() error {
	return svc.Run(s.name, s)
}

func NewSystemService(name string, f func() int) *systemService {
	return &systemService{
		name: name,
		cmd:  f,
	}
}
