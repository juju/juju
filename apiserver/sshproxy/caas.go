// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"

	"github.com/juju/names/v5"
	"golang.org/x/crypto/ssh"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

func (h *sshHandler) handleCAAS(st *state.PooledState, m *state.Model, w http.ResponseWriter, r *http.Request,
	entity names.Tag, container string) error {
	unitTag, ok := entity.(names.UnitTag)
	if !ok {
		return fmt.Errorf("%w: entity %v is not a valid unit",
			apiservererrors.ErrBadRequest, entity)
	}

	_, err := st.Unit(unitTag.Id())
	if err != nil {
		return fmt.Errorf("cannot get unit: %w", err)
	}

	hostKey, err := st.GetSSHProxyHostKeys(unitTag)
	if err != nil {
		return fmt.Errorf("cannot get ssh proxy host keys: %w", err)
	}

	cfg, err := m.Config()
	if err != nil {
		return fmt.Errorf("cannot get model config: %w", err)
	}

	authorizedKeys := []ssh.PublicKey{}
	authorizedKeyReader := bufio.NewScanner(bytes.NewReader([]byte(cfg.AuthorizedKeys())))
	for authorizedKeyReader.Scan() {
		b := authorizedKeyReader.Bytes()
		publicKey, _, _, _, err := ssh.ParseAuthorizedKey(b)
		if err != nil {
			return fmt.Errorf("cannot parse authorized key: %w\n%q", err, string(b))
		}
		authorizedKeys = append(authorizedKeys, publicKey)
	}
	err = authorizedKeyReader.Err()
	if err != nil {
		return fmt.Errorf("cannot parse authorized keys from model config: %w", err)
	}

	env, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(m)
	if err != nil {
		return fmt.Errorf("cannot get caas broker: %w", err)
	}

	conn, err := h.hijack(w)
	if err != nil {
		return fmt.Errorf("%w: cannot hijack connection: %v", errConnHijacked, err)
	}
	defer conn.Close()

	err = env.HandleSSHConn(conn, unitTag, container, hostKey, authorizedKeys)
	if err != nil {
		return fmt.Errorf("%w: cannot handle proxied ssh connection: %v", errConnHijacked, err)
	}

	return errConnHijacked
}
