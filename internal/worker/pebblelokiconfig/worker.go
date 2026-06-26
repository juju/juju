// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebblelokiconfig

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/canonical/pebble/client"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/logger"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

const (
	// layerLabel is the Pebble layer label used for the log-forwarding
	// configuration. AddLayer with Combine=true merges into any existing
	// layer bearing this label.
	layerLabel = "juju-loki-log-forwarding"

	// logTargetName is the name of the single log-target entry within the
	// layer.
	logTargetName = "juju-loki"

	// containerAgentService is the Pebble service name for the
	// containeragent process.
	containerAgentService = "container-agent"

	// defaultPebbleSocket is used when PEBBLE_SOCKET is not set.
	defaultPebbleSocket = "/var/lib/pebble/default/.pebble.socket"

	// pebbleReservedLabelPrefix is the prefix Pebble reserves for its own
	// labels. Custom labels must never use this prefix.
	pebbleReservedLabelPrefix = "pebble_"

	// retry constants for transient Pebble errors.
	retryInitialDelay  = 1 * time.Second
	retryMaxDelay      = 30 * time.Second
	retryMaxDuration   = 5 * time.Minute
	retryBackoffFactor = 1.6
	retryAttempts      = 3
)

// LoggerAPI represents the API calls the worker makes to obtain and watch the
// controller-wide Loki configuration.
type LoggerAPI interface {
	// GetControllerLokiConfig returns the controller-wide Loki configuration
	// for the supplied agent.
	GetControllerLokiConfig(ctx context.Context, agentTag names.Tag) (logger.ControllerLokiConfig, error)

	// WatchControllerLokiConfig returns a watcher for controller-wide Loki
	// configuration changes.
	WatchControllerLokiConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error)
}

// PebbleClient is the subset of the Pebble client used by this worker.
type PebbleClient interface {
	// AddLayer adds or combines a layer in the Pebble plan.
	AddLayer(opts *client.AddLayerOptions) error

	// Replan stops and restarts services whose configuration has changed.
	Replan(opts *client.ServiceOptions) (string, error)

	// CloseIdleConnections closes unused connections.
	CloseIdleConnections()
}

// NewPebbleClientFunc creates a PebbleClient connected to the given socket
// path.
type NewPebbleClientFunc func(socketPath string) (PebbleClient, error)

// WorkerConfig contains the information required by the worker.
type WorkerConfig struct {
	Agent            agent.Agent
	API              LoggerAPI
	Clock            clock.Clock
	Logger           corelogger.Logger
	NewPebbleClient  NewPebbleClientFunc
	PebbleSocketPath string
}

// Validate ensures all the necessary fields have values.
func (c WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.API == nil {
		return errors.NotValidf("missing api")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	if c.NewPebbleClient == nil {
		return errors.NotValidf("missing NewPebbleClient")
	}
	return nil
}

type pebbleLokiConfigWorker struct {
	catacomb catacomb.Catacomb

	config WorkerConfig

	agentTag      names.Tag
	controllerTag names.ControllerTag
	modelTag      names.ModelTag

	// socketPath is the resolved Pebble socket path.
	socketPath string

	// incompatible is set when the local Pebble does not support
	// log-targets. Once true, the worker stops attempting updates and
	// stays on the direct fallback path.
	incompatible bool
}

// NewWorker returns a worker that reconciles a Pebble log-forwarding layer
// for the containeragent service whenever the controller-wide Loki
// configuration changes.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	currentConfig := config.Agent.CurrentConfig()
	w := &pebbleLokiConfigWorker{
		config:        config,
		agentTag:      currentConfig.Tag(),
		controllerTag: currentConfig.Controller(),
		modelTag:      currentConfig.Model(),
		socketPath:    ResolvePebbleSocket(config.PebbleSocketPath),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "pebble-loki-config",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// ResolvePebbleSocket determines the Pebble socket path. If a path is
// explicitly provided via config it takes precedence, otherwise the
// PEBBLE_SOCKET environment variable is consulted, falling back to the
// default location.
func ResolvePebbleSocket(configured string) string {
	if configured != "" {
		return configured
	}
	if env := os.Getenv("PEBBLE_SOCKET"); env != "" {
		return env
	}
	return defaultPebbleSocket
}

// Kill implements worker.Worker.Kill.
func (w *pebbleLokiConfigWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *pebbleLokiConfigWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *pebbleLokiConfigWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	w.config.Logger.Infof(ctx, "pebble loki config worker started, socket %s", w.socketPath)

	notifyWatcher, err := w.config.API.WatchControllerLokiConfig(ctx, w.agentTag)
	if err != nil {
		return errors.Annotate(err, "watching controller loki config")
	}
	if err := w.catacomb.Add(notifyWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-notifyWatcher.Changes():
			if !ok {
				return errors.New("loki config watcher channel closed")
			}
			if err := w.handleLokiConfigChange(ctx); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// handleLokiConfigChange fetches the current Loki config and reconciles the
// Pebble layer.
func (w *pebbleLokiConfigWorker) handleLokiConfigChange(ctx context.Context) error {
	// Once we know Pebble doesn't support log-targets there is nothing
	// useful we can do; stay on the direct fallback path.
	if w.incompatible {
		w.config.Logger.Debugf(ctx, "pebble does not support log-targets, skipping reconciliation")
		return nil
	}

	lokiConfig, err := w.config.API.GetControllerLokiConfig(ctx, w.agentTag)
	if err != nil {
		return errors.Annotate(err, "getting controller loki config")
	}

	if err := w.reconcile(ctx, lokiConfig); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// reconcile pushes the log-forwarding layer to Pebble via
// AddLayer(combine=true). The layer only ever contains log-targets entries
// (no service definitions), so AddLayer alone is sufficient and replan is
// never needed for log-target-only updates.
func (w *pebbleLokiConfigWorker) reconcile(
	ctx context.Context,
	lokiConfig logger.ControllerLokiConfig,
) error {
	if lokiConfig.Endpoint == "" {
		w.config.Logger.Infof(ctx, "loki endpoint is empty, clearing log-forwarding layer")
	}
	return w.reconcileWithRetry(ctx, lokiConfig)
}

func (w *pebbleLokiConfigWorker) reconcileWithRetry(
	ctx context.Context,
	lokiConfig logger.ControllerLokiConfig,
) error {
	layerData, err := BuildLayerYAML(lokiConfig, w.agentTag, w.controllerTag, w.modelTag)
	if err != nil {
		return errors.Trace(err)
	}

	var incompatible bool
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			pebbleClient, err := w.config.NewPebbleClient(w.socketPath)
			if err != nil {
				return errors.Annotate(err, "creating pebble client")
			}
			defer pebbleClient.CloseIdleConnections()

			addErr := pebbleClient.AddLayer(&client.AddLayerOptions{
				Combine:   true,
				Label:     layerLabel,
				LayerData: layerData,
			})
			if addErr != nil {
				if IsIncompatiblePebbleError(addErr) {
					incompatible = true
					return errPermanentIncompatibility
				}
				return errors.Trace(addErr)
			}
			return nil
		},
		NotifyFunc: func(lastError error, attempt int) {
			w.config.Logger.Warningf(ctx, "pebble AddLayer failed (attempt %d): %v", attempt, lastError)
		},
		Clock:       w.config.Clock,
		Attempts:    retryAttempts,
		Delay:       retryInitialDelay,
		MaxDuration: retryMaxDuration,
		BackoffFunc: retry.ExpBackoff(retryInitialDelay, retryMaxDelay, retryBackoffFactor, true),
		Stop:        w.catacomb.Dying(),
		IsFatalError: func(err error) bool {
			return errors.Is(err, errPermanentIncompatibility) || incompatible
		},
	})

	if incompatible {
		w.incompatible = true
		w.config.Logger.Warningf(ctx, "pebble does not support log-targets, staying on direct fallback")
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	w.config.Logger.Infof(ctx, "pebble log-forwarding layer reconciled")
	return nil
}

// BuildLayerYAML marshals a Pebble layer containing a single log-targets
// entry for the given Loki config. Custom labels never use the reserved
// pebble_ prefix.
func BuildLayerYAML(
	lokiConfig logger.ControllerLokiConfig,
	agentTag names.Tag,
	controllerTag names.ControllerTag,
	modelTag names.ModelTag,
) ([]byte, error) {
	labels := map[string]string{
		"juju_controller": controllerTag.Id(),
		"juju_model":      modelTag.Id(),
		"juju_agent":      agentTag.String(),
	}
	for k := range labels {
		if strings.HasPrefix(k, pebbleReservedLabelPrefix) {
			return nil, errors.Errorf("label %q uses reserved prefix %q", k, pebbleReservedLabelPrefix)
		}
	}

	target := pebbleLogTarget{
		Override: "replace",
		Type:     "loki",
		Location: lokiConfig.Endpoint,
		Services: []string{containerAgentService},
		Labels:   labels,
	}
	if lokiConfig.Endpoint == "" {
		target = pebbleLogTarget{Override: "remove"}
	}

	layer := pebbleLayer{
		Summary: "Juju Loki log forwarding",
		LogTargets: map[string]pebbleLogTarget{
			logTargetName: target,
		},
	}

	data, err := yaml.Marshal(layer)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling pebble layer")
	}
	return data, nil
}

// IsIncompatiblePebbleError reports whether the error from AddLayer indicates
// permanent Pebble incompatibility (e.g. 400 unknown section "log-targets").
func IsIncompatiblePebbleError(err error) bool {
	var pebbleErr *client.Error
	if errors.As(err, &pebbleErr) {
		if pebbleErr.StatusCode == 400 {
			msg := strings.ToLower(pebbleErr.Message)
			if strings.Contains(msg, "log-targets") ||
				strings.Contains(msg, "unknown section") {
				return true
			}
		}
	}
	return false
}

var errPermanentIncompatibility = errors.New("permanent pebble incompatibility")

func (w *pebbleLokiConfigWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

// pebbleLayer is a minimal subset of the Pebble layer format sufficient for
// log-targets. We define our own struct rather than importing pebble's
// internals/plan to avoid pulling internal dependencies.
type pebbleLayer struct {
	Summary     string                     `yaml:"summary,omitempty"`
	Description string                     `yaml:"description,omitempty"`
	LogTargets  map[string]pebbleLogTarget `yaml:"log-targets,omitempty"`
}

// pebbleLogTarget describes a single log-forwarding target.
type pebbleLogTarget struct {
	Override string            `yaml:"override,omitempty"`
	Type     string            `yaml:"type,omitempty"`
	Location string            `yaml:"location,omitempty"`
	Services []string          `yaml:"services,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}
