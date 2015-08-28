// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build windows

package service

import (
	"github.com/gabriel-samfira/sys/windows/svc"
)

// SystemService type that is responsible for managing the life-cycle of the service
type SystemService struct {
	// Name the label for the service. It is not used for any useful operation
	// by the service handler.
	Name string
	// Cmd is the function the service handler will run as a service.
	Cmd func(args []string) int
	// Args is passed to Cmd() as function arguments.
	Args []string
}

// Execute implements the svc.Handler interface
func (s *SystemService) Execute(args []string, changeReq <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	errChannel := make(chan int, 1)

	go func() {
		err := s.Cmd(s.Args)
		errChannel <- err
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case r := <-changeReq:
			switch r.Cmd {
			case svc.Interrogate:
				changes <- r.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		case err := <-errChannel:
			return false, uint32(err)
		}
	}
}

// Run runs the service
func (s *SystemService) Run() error {
	return svc.Run(s.Name, s)
}
