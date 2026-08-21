// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
)

const (
	forwardControllerUUIDKey = "user.juju-controller-uuid"
	forwardInstanceIDKey     = "user.juju-instance-id"
)

// EnsureControllerNetworkForward allocates an external IPv4 address and
// forwards the supplied TCP ports to a controller instance.
func (s *Server) EnsureControllerNetworkForward(
	networkName, controllerUUID, instanceID, targetAddress string,
	ports []int,
) (string, error) {
	forwardPut := controllerNetworkForward(controllerUUID, instanceID, targetAddress, ports)

	forward, eTag, err := s.getControllerNetworkForward(networkName, controllerUUID, instanceID)
	if err != nil {
		return "", errors.Trace(err)
	}
	if forward != nil {
		op, err := s.UpdateNetworkForward(networkName, forward.ListenAddress, forwardPut, eTag)
		if err := WaitOp(op, err); err != nil {
			return "", errors.Annotate(err, "updating controller network forward")
		}
		return forward.ListenAddress, nil
	}

	op, err := s.CreateNetworkForward(networkName, api.NetworkForwardsPost{
		ListenAddress:     "0.0.0.0",
		NetworkForwardPut: forwardPut,
	})
	if err := WaitOp(op, err); err != nil {
		return "", errors.Annotate(err, "creating controller network forward")
	}

	forward, _, err = s.getControllerNetworkForward(networkName, controllerUUID, instanceID)
	if err != nil {
		return "", errors.Trace(err)
	}
	if forward == nil {
		return "", errors.NotFoundf("allocated controller network forward")
	}
	return forward.ListenAddress, nil
}

// ControllerNetworkForwardAddress returns the allocated address for a
// controller instance.
func (s *Server) ControllerNetworkForwardAddress(
	networkName, controllerUUID, instanceID string,
) (string, bool, error) {
	forward, _, err := s.getControllerNetworkForward(networkName, controllerUUID, instanceID)
	if err != nil {
		return "", false, errors.Trace(err)
	}
	if forward == nil {
		return "", false, nil
	}
	return forward.ListenAddress, true, nil
}

// DeleteControllerNetworkForwards deletes forwards for a controller. If
// instanceID is non-empty, only forwards for that instance are deleted.
func (s *Server) DeleteControllerNetworkForwards(
	networkName, controllerUUID, instanceID string,
) error {
	forwards, err := s.GetNetworkForwards(networkName)
	if err != nil {
		return errors.Annotate(err, "listing controller network forwards")
	}

	var deleteErrors []error
	for _, forward := range forwards {
		if !isControllerNetworkForward(forward, controllerUUID, instanceID) {
			continue
		}
		op, err := s.DeleteNetworkForward(networkName, forward.ListenAddress)
		if err := WaitOp(op, err); err != nil {
			deleteErrors = append(deleteErrors, errors.Annotatef(
				err, "deleting controller network forward %q", forward.ListenAddress))
		}
	}
	return stderrors.Join(deleteErrors...)
}

func (s *Server) getControllerNetworkForward(
	networkName, controllerUUID, instanceID string,
) (*api.NetworkForward, string, error) {
	forwards, err := s.GetNetworkForwards(networkName)
	if err != nil {
		return nil, "", errors.Annotate(err, "listing controller network forwards")
	}
	for _, forward := range forwards {
		if !isControllerNetworkForward(forward, controllerUUID, instanceID) {
			continue
		}
		current, eTag, err := s.GetNetworkForward(networkName, forward.ListenAddress)
		return current, eTag, errors.Trace(err)
	}
	return nil, "", nil
}

func isControllerNetworkForward(
	forward api.NetworkForward, controllerUUID, instanceID string,
) bool {
	if controllerUUID != "" && forward.Config[forwardControllerUUIDKey] != controllerUUID {
		return false
	}
	return instanceID == "" || forward.Config[forwardInstanceIDKey] == instanceID
}

func controllerNetworkForward(
	controllerUUID, instanceID, targetAddress string, ports []int,
) api.NetworkForwardPut {
	uniquePorts := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port > 0 {
			uniquePorts[port] = struct{}{}
		}
	}
	ports = make([]int, 0, len(uniquePorts))
	for port := range uniquePorts {
		ports = append(ports, port)
	}
	sort.Ints(ports)

	portStrings := make([]string, len(ports))
	for i, port := range ports {
		portStrings[i] = strconv.Itoa(port)
	}

	return api.NetworkForwardPut{
		Description: fmt.Sprintf("Juju controller %s instance %s", controllerUUID, instanceID),
		Config: map[string]string{
			forwardControllerUUIDKey: controllerUUID,
			forwardInstanceIDKey:     instanceID,
		},
		Ports: []api.NetworkForwardPort{{
			Protocol:      "tcp",
			ListenPort:    strings.Join(portStrings, ","),
			TargetAddress: targetAddress,
		}},
	}
}
