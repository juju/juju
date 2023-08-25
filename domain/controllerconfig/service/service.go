// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerConfig(context.Context) (map[string]string, error)
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]string, removeAttrs []string) error

	// AllKeysQuery is used to get the initial state
	// for the controller configuration watcher.
	AllKeysQuery() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, wf WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: wf,
	}
}

// ControllerConfig returns the config values for the controller.
func (s *Service) ControllerConfig(ctx context.Context) (controller.Config, error) {
	ctrlConfigMap, err := s.st.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "get controller config")
	}
	coerced, err := decodeMap(ctrlConfigMap)
	if err != nil {
		return nil, errors.Annotatef(err, "coerce controller config")
	}

	// Get the controller UUID and CA cert from the config, so we can generate
	// a new controller config.
	var (
		ctrlUUID, caCert string
		ok               bool
	)
	if ctrlUUID, ok = coerced[controller.ControllerUUIDKey].(string); !ok {
		return nil, errors.NotFoundf("controller UUID")
	}
	if caCert, ok = coerced[controller.CACertKey].(string); !ok {
		return nil, errors.NotFoundf("controller CACert")
	}

	// Make a new controller config based on the coerced controller config map
	// returned from state.
	ctrlConfig, err := controller.NewConfig(ctrlUUID, caCert, coerced)
	if err != nil {
		return nil, errors.Annotatef(err, "create controller config")
	}
	return ctrlConfig, errors.Annotate(err, "getting controller config state")
}

// UpdateControllerConfig updates the controller config.
func (s *Service) UpdateControllerConfig(ctx context.Context, updateAttrs controller.Config, removeAttrs []string) error {
	coerced, err := controller.EncodeToString(updateAttrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = validateConfig(coerced, removeAttrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.UpdateControllerConfig(ctx, coerced, removeAttrs)
	return errors.Annotate(err, "updating controller config state")
}

// Watch returns a watcher that returns keys for any changes to controller
// config.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher("controller_config", changestream.All, s.st.AllKeysQuery())
}

// validateConfig validates the given updateAttrs and removeAttrs.
func validateConfig(updateAttrs map[string]string, removeAttrs []string) error {
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return errors.Trace(err)
	}

	for k := range updateAttrs {
		if err := validateConfigField(k); err != nil {
			return errors.Trace(err)
		}
		if field, ok := fields[k]; ok {
			_, err := field.Coerce(updateAttrs[k], []string{k})
			if err != nil {
				return errors.Annotatef(err, "coerce controller config key %q", k)
			}
		}
	}
	for _, r := range removeAttrs {
		if err := validateConfigField(r); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// validateConfigField checks that the given field is a valid controller config.
func validateConfigField(name string) error {
	if !controller.ControllerOnlyAttribute(name) {
		return errors.Errorf("unknown controller config setting %q", name)
	}
	if !controller.AllowedUpdateConfigAttributes.Contains(name) {
		return errors.Errorf("can not change %q after bootstrap", name)
	}
	return nil
}

// decodeMap converts a map[string]any to a controller config
// and coerces any values that are found in the validation schema.
func decodeMap(m map[string]string) (map[string]any, error) {
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]any, len(m))
	for key, v := range m {
		if field, ok := fields[key]; ok {
			v, err := field.Coerce(m[key], []string{key})
			if err != nil {
				return nil, errors.Annotatef(err, "coerce controller config key %q", key)
			}
			result[key] = v
			continue
		}

		result[key] = v
	}
	return result, nil
}
