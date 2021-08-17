package raftleaseservice

import (
	"net/http"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/pubsub/v2"
)

// NewNotFoundHandler allows the creation of a new not found handler for the
// apiserver mux.
func NewNotFoundHandler(apiInfo *api.Info, hub *pubsub.StructuredHub) (http.Handler, error) {
	h := &notFoundHandler{
		addresses: apiInfo.Addrs,
	}

	// Subscribe to API server address changes.
	unsubscribe, err := hub.Subscribe(
		apiserver.DetailsTopic,
		h.apiserverDetailsChanged,
	)
	if err != nil {
		return nil, errors.Annotate(err, "subscribing to apiserver details")
	}

	h.unsubscribe = unsubscribe

	// Now that we're subscribed, request the current API server details.
	req := apiserver.DetailsRequest{
		Requester: "raft-lease-services-not-fund",
		LocalOnly: true,
	}
	if _, err := hub.Publish(apiserver.DetailsRequestTopic, req); err != nil {
		unsubscribe()
		return nil, errors.Annotate(err, "requesting current apiserver details")
	}

	return h, nil
}

type notFoundHandler struct {
	mutex       sync.RWMutex
	addresses   []string
	unsubscribe func()
}

func (h *notFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This replicates what the PatternServerMux does.
	if r.URL.Path != "/raft/lease" {
		http.NotFound(w, r)
		return
	}

	h.mutex.RLock()
	if len(h.addresses) == 0 {
		h.mutex.RUnlock()
		http.NotFound(w, r)
		return
	}
	// TODO (stickupkid): If the first address is ourselves, then return
	// not-found, as we can't redirect to ourselves.
	url := h.addresses[0]
	h.mutex.RUnlock()
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *notFoundHandler) Close() {
	if h.unsubscribe != nil {
		h.unsubscribe()
	}
}

func (h *notFoundHandler) apiserverDetailsChanged(topic string, details apiserver.Details, err error) {
	if err != nil {
		return
	}

	var addrs []string
	for _, server := range details.Servers {
		addrs = append(addrs, server.InternalAddress)
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.addresses = addrs
}
