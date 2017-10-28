// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/audit"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

func newObserverFn(
	agentConfig agent.Config,
	controllerConfig controller.Config,
	clock clock.Clock,
	persistAuditEntry audit.AuditEntrySinkFn,
	prometheusRegisterer prometheus.Registerer,
) (observer.ObserverFactory, error) {

	var observerFactories []observer.ObserverFactory

	// Common logging of RPC requests
	observerFactories = append(observerFactories, func() observer.Observer {
		logger := loggo.GetLogger("juju.apiserver")
		ctx := observer.RequestObserverContext{
			Clock:  clock,
			Logger: logger,
		}
		return observer.NewRequestObserver(ctx)
	})

	// TODO(katco): We should be doing something more serious than
	// logging audit errors. Failures in the auditing systems should
	// stop the api server until the problem can be corrected.
	auditErrorHandler := func(err error) {
		logger.Criticalf("%v", err)
	}

	// Auditing observer
	// TODO(katco): Auditing needs feature tests (lp:1604551)
	if controllerConfig.AuditingEnabled() {
		model := agentConfig.Model()
		observerFactories = append(observerFactories, func() observer.Observer {
			ctx := &observer.AuditContext{
				JujuServerVersion: version.Current,
				ModelUUID:         model.Id(),
			}
			return observer.NewAudit(ctx, persistAuditEntry, auditErrorHandler)
		})
	}

	// Metrics observer.
	metricObserver, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:                clock,
		PrometheusRegisterer: prometheusRegisterer,
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating metric observer factory")
	}
	observerFactories = append(observerFactories, metricObserver)

	return observer.ObserverFactoryMultiplexer(observerFactories...), nil
}

// StoreAuditEntryFunc is the type of a function
// used for storing an audit entry.
type StoreAuditEntryFunc func(audit.AuditEntry) error

// NewStateStoreAuditEntryFunc returns a StoreAuditEntryFunc that
// persists audit entries to the given state.
func NewStateStoreAuditEntryFunc(st *state.State) StoreAuditEntryFunc {
	return st.PutAuditEntryFn()
}

func newAuditEntrySink(persistFn StoreAuditEntryFunc, logDir string) audit.AuditEntrySinkFn {
	fileSinkFn := audit.NewLogFileSink(logDir)
	return func(entry audit.AuditEntry) error {
		// We don't care about auditing anything but user actions.
		if _, err := names.ParseUserTag(entry.OriginName); err != nil {
			return nil
		}
		// TODO(wallyworld) - Pinger requests should not originate as a user action.
		if strings.HasPrefix(entry.Operation, "Pinger:") {
			return nil
		}
		persistErr := persistFn(entry)
		sinkErr := fileSinkFn(entry)
		if persistErr == nil {
			return errors.Annotate(sinkErr, "cannot save audit record to file")
		}
		if sinkErr == nil {
			return errors.Annotate(persistErr, "cannot save audit record to database")
		}
		return errors.Annotate(persistErr, "cannot save audit record to file or database")
	}
}
