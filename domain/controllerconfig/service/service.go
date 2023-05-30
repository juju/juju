// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	jujucontroller "github.com/juju/juju/controller"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerConfig(context.Context) (map[string]interface{}, error)
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]interface{}, removeAttrs []string) error
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// ControllerConfig returns the config values for the controller.
func (s *Service) ControllerConfig(ctx context.Context) (jujucontroller.Config, error) {
	coercedControllerConfig := make(jujucontroller.Config)
	cc, err := s.st.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	coercedControllerConfig, err = coerceControllerConfigMap(cc)
	return coercedControllerConfig, errors.Annotate(err, "getting controller config state")
}

// UpdateControllerConfig updates the controller config.
func (s *Service) UpdateControllerConfig(ctx context.Context, updateAttrs jujucontroller.Config, removeAttrs []string) error {
	err := s.st.UpdateControllerConfig(ctx, updateAttrs, removeAttrs)
	return errors.Annotate(err, "updating controller config state")
}

// checkUpdateControllerConfig checks that the given name is a valid.
func checkUpdateControllerConfig(name string) error {
	if !jujucontroller.ControllerOnlyAttribute(name) {
		return errors.Errorf("unknown controller config setting %q", name)
	}
	if !jujucontroller.AllowedUpdateConfigAttributes.Contains(name) {
		return errors.Errorf("can't change %q after bootstrap", name)
	}
	return nil
}

// coerceControllerConfigMap converts a map[string]interface{} to a controller config.
func coerceControllerConfigMap(m map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	// Validate the updateAttrs.
	fields, _, err := jujucontroller.ConfigSchema.ValidationSchema()
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
