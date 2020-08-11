// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"gopkg.in/yaml.v2"
)

type leaseHandler struct {
	leases Leases
}

// ServeHTTP is part of the http.Handler interface.
func (h leaseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ss, err := h.leases.Snapshot()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}
	snapshot, ok := ss.(*raftlease.Snapshot)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "expected *raftlease.Snapshot\n")
		return
	}
	if snapshot.Version != 1 {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "only understand how to show version 1 snapshots\n")
		return
	}

	data := h.translateSnapshot(snapshot)

	q := r.URL.Query()
	if v := q.Get("model"); v != "" {
		data = h.filterModel(data, v)
	}
	if v := q["app"]; len(v) > 0 {
		data = h.filterApp(data, v)
	}

	bytes, err := yaml.Marshal(h.format(data))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}
	w.Write(bytes)
}

type leases struct {
	// TODO: handle pinning

	// This is a map of model-uuid to info for the singular leases.
	controller map[string]leaseInfo
	// Top level map is keyed on model-uuid, second is applications
	// within that model.
	models map[string]map[string]leaseInfo
}

type leaseInfo struct {
	holder   string
	acquired time.Duration
	expires  time.Duration
}

func (h leaseHandler) translateSnapshot(snapshot *raftlease.Snapshot) *leases {
	result := &leases{
		controller: make(map[string]leaseInfo),
		models:     make(map[string]map[string]leaseInfo),
	}
	now := snapshot.GlobalTime
	makeLeaseInfo := func(entry raftlease.SnapshotEntry) leaseInfo {
		acquired := entry.Start.Sub(now).Round(time.Second)
		return leaseInfo{
			holder:   entry.Holder,
			acquired: acquired,
			// We don't need to round the expiry at this stage because
			// the entry.Duration is always one minute. Even if this changes
			// it is unlikely to be a fraction of a second, so we should
			// still be fine.
			expires: acquired + entry.Duration,
		}
	}
	for key, value := range snapshot.Entries {
		switch key.Namespace {
		case lease.SingularControllerNamespace:
			result.controller[key.Lease] = makeLeaseInfo(value)
		case lease.ApplicationLeadershipNamespace:
			model, ok := result.models[key.ModelUUID]
			if !ok {
				model = make(map[string]leaseInfo)
				result.models[key.ModelUUID] = model
			}
			model[key.Lease] = makeLeaseInfo(value)
		default:
			logger.Warningf("unknown namespace %q", key.Namespace)
		}
	}
	// TODO: handle pinned.
	return result
}

func (h leaseHandler) filterModel(data *leases, partialModelUUID string) *leases {
	result := &leases{
		controller: make(map[string]leaseInfo),
		models:     make(map[string]map[string]leaseInfo),
	}
	for modelUUID, value := range data.controller {
		if strings.Contains(modelUUID, partialModelUUID) {
			result.controller[modelUUID] = value
		}
	}
	for modelUUID, value := range data.models {
		if strings.Contains(modelUUID, partialModelUUID) {
			result.models[modelUUID] = value
		}
	}

	return result
}

func (h leaseHandler) filterApp(data *leases, partialAppNames []string) *leases {
	result := &leases{
		models: make(map[string]map[string]leaseInfo),
	}
	for modelUUID, apps := range data.models {
		appInfo := make(map[string]leaseInfo)
		for appName, value := range apps {
			// If the appName matches any of the partial names, add it in.
			for _, partial := range partialAppNames {
				if strings.Contains(appName, partial) {
					appInfo[appName] = value
					break
				}
			}
		}
		if len(appInfo) > 0 {
			result.models[modelUUID] = appInfo
		}
	}

	return result
}

func (h leaseHandler) format(leases *leases) map[string]interface{} {
	result := make(map[string]interface{})

	// Since we are just making a map for YAML to output, we don't
	// need to worry about ordering, because YAML output will order
	// maps for us.
	asMap := func(info leaseInfo) map[string]interface{} {
		return map[string]interface{}{
			"holder":         info.holder,
			"lease-acquired": fmt.Sprintf("%s ago", -info.acquired),
			"lease-expires":  info.expires.String(),
		}
	}

	if len(leases.controller) > 0 {
		controller := make(map[string]interface{})
		for key, value := range leases.controller {
			controller[key] = asMap(value)
		}
		result["controller-leases"] = controller
	}

	if len(leases.models) > 0 {
		models := make(map[string]interface{})
		for modelUUID, modelValue := range leases.models {
			apps := make(map[string]interface{})
			for key, value := range modelValue {
				apps[key] = asMap(value)
			}
			models[modelUUID] = apps
		}
		result["model-leases"] = models
	}
	return result
}
