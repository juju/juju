// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprober

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/internal/observability/probe"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

type Mux interface {
	AddHandler(string, string, http.Handler) error
	RemoveHandler(string, string)
}

type Controller struct {
	catacomb catacomb.Catacomb

	mux    Mux
	probes *CAASProbes
}

const (
	DetailedResponseQueryKey = "detailed"
	PathLivenessProbe        = "/liveness"
	PathReadinessProbe       = "/readiness"
	PathStartupProbe         = "/startup"
)

// NewController constructs a new caas prober Controller.
func NewController(probes *CAASProbes, mux Mux) (*Controller, error) {
	c := &Controller{
		mux:    mux,
		probes: probes,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-prober",
		Site: &c.catacomb,
		Work: c.loop,
	}); err != nil {
		return c, jujuerrors.Trace(err)
	}

	return c, nil
}

// Kill implements worker.Kill
func (c *Controller) Kill() {
	c.catacomb.Kill(nil)
}

func (c *Controller) loop() error {
	if err := c.mux.AddHandler(
		http.MethodGet,
		k8sconstants.AgentHTTPPathLiveness,
		ProbeHandler(probe.ProbeLiveness, c.probes)); err != nil {
		return jujuerrors.Trace(err)
	}
	defer c.mux.RemoveHandler(http.MethodGet, PathLivenessProbe)

	if err := c.mux.AddHandler(
		http.MethodGet,
		k8sconstants.AgentHTTPPathReadiness,
		ProbeHandler(probe.ProbeReadiness, c.probes)); err != nil {
		return jujuerrors.Trace(err)
	}
	defer c.mux.RemoveHandler(http.MethodGet, PathReadinessProbe)

	if err := c.mux.AddHandler(
		http.MethodGet,
		k8sconstants.AgentHTTPPathStartup,
		ProbeHandler(probe.ProbeStartup, c.probes)); err != nil {
		return jujuerrors.Trace(err)
	}
	defer c.mux.RemoveHandler(http.MethodGet, PathStartupProbe)

	<-c.catacomb.Dying()
	return c.catacomb.ErrDying()
}

// ProbeHandler implements a http handler for the supplied probe and probe name.
func ProbeHandler(name probe.ProbeType, probes *CAASProbes) http.Handler {
	var last atomic.Bool
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		shouldDetailResponse := false
		detailedVals, exists := req.URL.Query()[DetailedResponseQueryKey]
		if exists && len(detailedVals) == 1 {
			val, err := strconv.ParseBool(detailedVals[0])
			if err != nil {
				http.Error(res, fmt.Sprintf("invalid detailed query value %s expected boolean", detailedVals[0]),
					http.StatusBadRequest)
				return
			}
			shouldDetailResponse = val
		}

		aggProbe, ok := probes.ProbeAggregate(name)
		if !ok {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusNotImplemented), name),
				http.StatusNotImplemented)
			return
		}

		detail := &bytes.Buffer{}
		good, n, err := aggProbe.ProbeWithResultCallback(
			probe.ProbeResultCallback(func(probeKey string, val bool, err error) {
				if !shouldDetailResponse {
					return
				}

				// We are trying to output 1 line here per probe called.
				// The format should be:
				// + uniter # for success
				// - uniter: some error # for failure

				if val {
					// Print + on probe success
					detail.WriteString("+ ")
				} else {
					// Print - on probe failure
					detail.WriteString("- ")
				}

				// Print the probe name
				detail.WriteString(probeKey)

				// Print the error if one exists
				if err != nil {
					_, _ = fmt.Fprintf(detail, ": %s", err)
				}

				// Finish the current line
				detail.WriteString("\n")
			}),
		)

		if jujuerrors.Is(err, jujuerrors.NotImplemented) {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusNotImplemented), name),
				http.StatusNotImplemented)
			if shouldDetailResponse {
				_, _ = io.Copy(res, detail)
			}
			return
		} else if err != nil {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusInternalServerError), name),
				http.StatusInternalServerError)
			if shouldDetailResponse {
				_, _ = io.Copy(res, detail)
			}
			return
		}

		// If no probers were consulted, return the last value.
		if n == 0 {
			good = last.Load()
		} else {
			last.Store(good)
		}

		if !good {
			http.Error(res, fmt.Sprintf("%s: probe %s",
				http.StatusText(http.StatusTeapot), name),
				http.StatusTeapot)
			if shouldDetailResponse {
				_, _ = io.Copy(res, detail)
			}
			return
		}

		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		res.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(res, "%s: probe %s\n", http.StatusText(http.StatusOK), name)
		if shouldDetailResponse {
			_, _ = io.Copy(res, detail)
		}
	})
}

// Wait implements worker.Wait
func (c *Controller) Wait() error {
	return c.catacomb.Wait()
}
