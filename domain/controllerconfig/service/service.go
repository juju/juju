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
	ControllerConfig(context.Context) (map[string]any, error)
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]any, removeAttrs []string) error

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
	cc, err := s.st.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var coercedControllerConfig controller.Config
	coercedControllerConfig, err = coerceControllerConfigMap(cc)
	return coercedControllerConfig, errors.Annotate(err, "getting controller config state")
}

// SeedControllerConfig sets the controller config without removing any
// attributes. This is used to set the initial config.
func (s *Service) SeedControllerConfig(ctx context.Context, attrs controller.Config) error {
	coercedUpdateAttrs, err := coerceControllerConfigMap(attrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = validateSeedConfig(attrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.UpdateControllerConfig(ctx, coercedUpdateAttrs, nil)
	return errors.Annotate(err, "seeding controller config state")
}

// UpdateControllerConfig updates the controller config.
func (s *Service) UpdateControllerConfig(ctx context.Context, updateAttrs controller.Config, removeAttrs []string) error {
	coercedUpdateAttrs, err := coerceControllerConfigMap(updateAttrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = validateUpdateConfig(updateAttrs, removeAttrs)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.UpdateControllerConfig(ctx, coercedUpdateAttrs, removeAttrs)
	return errors.Annotate(err, "updating controller config state")
}

// Watch returns a watcher that returns keys
// for any changes to controller config.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher("controller_config", changestream.All, s.st.AllKeysQuery())
}

// validateSeedConfig validate the given seed attrs.
func validateSeedConfig(attrs map[string]any) error {
	for name := range attrs {
		if !controller.ControllerOnlyAttribute(name) {
			return errors.Errorf("unknown controller config setting %q", name)
		}
	}
	return nil
}

// validateUpdateConfig validates the given updateAttrs and removeAttrs.
func validateUpdateConfig(updateAttrs map[string]any, removeAttrs []string) error {
	for k := range updateAttrs {
		if err := validateConfigField(k); err != nil {
			return errors.Trace(err)
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
		return errors.Errorf("can't change %q after bootstrap", name)
	}
	return nil
}

// coerceControllerConfigMap converts a map[string]any to a controller config.
func coerceControllerConfigMap(m map[string]any) (map[string]interface{}, error) {
	result := make(map[string]any)
	// Validate the updateAttrs.
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k := range m {
		if field, ok := fields[k]; ok {
			v, err := field.Coerce(m[k], []string{k})
			if err != nil {
				return nil, err
			}
			result[k] = v
		}
	}
	return result, nil
}
