// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/juju/names/v5"
	"golang.org/x/sync/errgroup"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/state"
)

func (h *sshHandler) handleIAAS(st *state.PooledState, w http.ResponseWriter, r *http.Request, entity names.Tag) error {
	if unit, ok := entity.(names.UnitTag); ok {
		unit, err := st.Unit(unit.Id())
		if err != nil {
			return err
		}
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return err
		}
		entity = names.NewMachineTag(machineId)
	}

	machineTag, ok := entity.(names.MachineTag)
	if !ok {
		return fmt.Errorf("unexpected tag %v", entity)
	}

	machine, err := st.Machine(machineTag.Id())
	if err != nil {
		return err
	}

	for _, addr := range machine.Addresses().Values() {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:ssh", addr))
		if err != nil {
			h.logger.Errorf("cannot proxy ssh to %s: %v", addr, err)
			continue
		}
		err = h.proxyMachine(w, r, conn)
		if err != nil {
			h.logger.Errorf("cannot proxy machine: %v", err)
		}
		return errConnHijacked
	}

	return apiservererrors.ErrTryAgain
}

func (h *sshHandler) proxyMachine(w http.ResponseWriter, r *http.Request, upstream net.Conn) error {
	defer upstream.Close()

	conn, err := h.hijack(w)
	if err != nil {
		return err
	}
	defer conn.Close()

	eg, _ := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		_, err := io.Copy(conn, upstream)
		return err
	})
	eg.Go(func() error {
		_, err := io.Copy(upstream, conn)
		return err
	})
	err = eg.Wait()
	if err != nil {
		return err
	}

	return nil
}
