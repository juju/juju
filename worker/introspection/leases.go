// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

type leaseHandler struct {
	leases Leases
	hub    StructuredHub
	clock  Clock

	runID     int32
	requestID uint64
}

func (h *leaseHandler) snapshot() (*raftlease.Snapshot, error) {
	ss, err := h.leases.Snapshot()
	if err != nil {
		return nil, errors.Annotate(err, "snapshot")
	}
	snapshot, ok := ss.(*raftlease.Snapshot)
	if !ok {
		return nil, errors.New("expected *raftlease.Snapshot")
	}
	if snapshot.Version != 1 {
		return nil, errors.New("only understand how to show version 1 snapshots")
	}
	return snapshot, nil
}

func (h *leaseHandler) list(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.snapshot()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err.Error())
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

func (h *leaseHandler) translateSnapshot(snapshot *raftlease.Snapshot) *leases {
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

func (h *leaseHandler) filterModel(data *leases, partialModelUUID string) *leases {
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

func (h *leaseHandler) filterApp(data *leases, partialAppNames []string) *leases {
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

func (h *leaseHandler) format(leases *leases) map[string]interface{} {
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

// ServeHTTP is part of the http.Handler interface.
func (h *leaseHandler) revoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintln(w, "revoking a lease requires a POST request")
		return
	}

	leaseKey, err := h.parseRevokeForm(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintln(w, err.Error())
		return
	}

	snapshot, err := h.snapshot()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err.Error())
		return
	}

	holder, ok := snapshot.Entries[leaseKey]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		if leaseKey.Namespace == lease.SingularControllerNamespace {
			fmt.Fprintf(w, "singular lease for model %q not found\n", leaseKey.ModelUUID)
		} else {
			fmt.Fprintf(w, "application lease for model %q and app %q not found\n", leaseKey.ModelUUID, leaseKey.Lease)
		}
		return
	}

	if err := h.revokeLeadership(leaseKey, holder.Holder); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err.Error())
		return
	}

	if leaseKey.Namespace == lease.SingularControllerNamespace {
		fmt.Fprintf(w, "singular lease for model %q revoked\n", leaseKey.ModelUUID)
	} else {
		fmt.Fprintf(w, "application lease for model %q and app %q revoked\n", leaseKey.ModelUUID, leaseKey.Lease)
	}
}

func (h *leaseHandler) revokeLeadership(key raftlease.SnapshotKey, holder string) error {
	command := &raftlease.Command{
		Version:   raftlease.CommandVersion,
		Operation: raftlease.OperationRevoke,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    holder,
	}

	bytes, err := command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}

	if h.runID == 0 {
		source := rand.NewSource(h.clock.Now().UnixNano())
		h.runID = rand.New(source).Int31()
	}

	// TODO: this should really be in a shared messages.go file under
	// the top level pubsub package.
	leaseRequestTopic := "lease.request"
	requestID := atomic.AddUint64(&h.requestID, 1)
	responseTopic := fmt.Sprintf("%s.%08x.%d", leaseRequestTopic, h.runID, requestID)

	responseChan := make(chan raftlease.ForwardResponse, 1)
	errChan := make(chan error)
	unsubscribe, err := h.hub.Subscribe(
		responseTopic,
		func(_ string, resp raftlease.ForwardResponse, err error) {
			if err != nil {
				errChan <- err
				return
			}
			responseChan <- resp
		},
	)
	if err != nil {
		return errors.Annotatef(err, "running %s", command)
	}
	defer unsubscribe()

	_, err = h.hub.Publish(leaseRequestTopic, raftlease.ForwardRequest{
		Command:       string(bytes),
		ResponseTopic: responseTopic,
	})
	if err != nil {
		return errors.Annotatef(err, "publishing %s", command)
	}

	select {
	case <-h.clock.After(15 * time.Second):
		return lease.ErrTimeout
	case err := <-errChan:
		return errors.Trace(err)
	case response := <-responseChan:
		return raftlease.RecoverError(response.Error)
	}
}

func (h *leaseHandler) parseRevokeForm(r *http.Request) (raftlease.SnapshotKey, error) {
	var result raftlease.SnapshotKey
	if err := r.ParseForm(); err != nil {
		return result, errors.Annotate(err, "parse form")
	}

	result.ModelUUID = r.Form.Get("model")
	if result.ModelUUID == "" {
		return result, errors.New("missing model")
	}
	result.Lease = r.Form.Get("lease")
	// Default namespace to application, unless overridden.
	result.Namespace = lease.ApplicationLeadershipNamespace
	switch ns := r.Form.Get("ns"); ns {
	case lease.SingularControllerNamespace:
		result.Namespace = ns
		result.Lease = result.ModelUUID
	case "", lease.ApplicationLeadershipNamespace:
		if result.Lease == "" {
			return result, errors.New("missing lease")
		}
	default:
		return result, errors.Errorf("unknown namespace: %q\n", ns)
	}

	return result, nil
}
