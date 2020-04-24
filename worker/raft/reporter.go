// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	humanize "github.com/dustin/go-humanize"
	"github.com/hashicorp/raft"
	"github.com/juju/worker/v2/dependency"
)

// Report is part of the dependency.Reporter interface.
func (w *Worker) Report() map[string]interface{} {
	out := make(map[string]interface{})
	r, err := w.Raft()
	if err != nil {
		out[dependency.KeyError] = err.Error()
		return out
	}

	state := r.State()
	out[dependency.KeyState] = state.String()
	out["leader"] = r.Leader()
	out["index"] = map[string]interface{}{
		"applied": r.AppliedIndex(),
		"last":    r.LastIndex(),
	}
	if state != raft.Leader {
		lastContact := "never"
		if t := r.LastContact(); !t.IsZero() {
			lastContact = humanize.Time(t)
		}
		out["last-contact"] = lastContact
	}

	config := make(map[string]interface{})
	future := r.GetConfiguration()
	if err := future.Error(); err != nil {
		config[dependency.KeyError] = err.Error()
	} else {
		servers := make(map[string]interface{})
		for _, server := range future.Configuration().Servers {
			servers[string(server.ID)] = map[string]interface{}{
				"suffrage": server.Suffrage.String(),
				"address":  server.Address,
			}
		}
		config["servers"] = servers
	}
	out["cluster-config"] = config

	return out
}
