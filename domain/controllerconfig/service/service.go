// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
)

// ModificationValidatorFunc is a function that validates a modification
// to the controller config.
type ModificationValidatorFunc = func(map[string]string) error

// State defines an interface for interacting with the underlying state.
type State interface {
	ControllerConfig(context.Context) (map[string]string, error)
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]string, removeAttrs []string, validateModification ModificationValidatorFunc) error

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
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// ControllerConfig returns the config values for the controller.
func (s *Service) ControllerConfig(ctx context.Context) (controller.Config, error) {
	ctrlConfigMap, err := s.st.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to get controller config")
	}
	coerced, err := deserializeMap(ctrlConfigMap)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to coerce controller config")
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
		return nil, errors.Annotatef(err, "unable to create controller config")
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
	err = s.st.UpdateControllerConfig(ctx, coerced, removeAttrs, func(current map[string]string) error {
		// Validate the updateAttrs against the current config.
		// This is done to ensure that the update config values are allowed
		// to be modified to the updated ones.
		//
		// For example, is it possible to move from filestorage to s3storage.
		// But it is not possible to move from s3storage to filestorage.
		if err := validObjectStoreProgression(current, updateAttrs, removeAttrs); err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	return errors.Annotate(err, "updating controller config state")
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
				return errors.Annotatef(err, "unable to coerce controller config key %q", k)
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
// TODO(nvinuesa): Must check that all machine controller addresses are in
// the HA space or in the mgmt space (see checkValidControllerConfig()):
// 	controllerIds, err := st.ControllerIds()
// 	if err != nil {
// 		return errors.Annotate(err, "cannot get controller info")
// 	}

// 	space, err := st.SpaceByName(spaceName)
// 	if err != nil {
// 		return errors.Trace(err)
// 	}
// 	netSpace, err := space.NetworkSpace()
// 	if err != nil {
// 		return errors.Annotate(err, "getting network space")
// 	}

// 	var missing []string
// 	for _, id := range controllerIds {
// 		m, err := st.Machine(id)
// 		if err != nil {
// 			return errors.Annotate(err, "cannot get machine")
// 		}
// 		if _, ok := m.Addresses().InSpaces(netSpace); !ok {
// 			missing = append(missing, id)
// 		}
// 	}

//	if len(missing) > 0 {
//		return errors.Errorf("machines with no addresses in this space: %s", strings.Join(missing, ", "))
//	}
func validateConfigField(name string) error {
	if !controller.ControllerOnlyAttribute(name) {
		return errors.Errorf("unknown controller config setting %q", name)
	}
	if !controller.AllowedUpdateConfigAttributes.Contains(name) {
		return errors.Errorf("can not change %q after bootstrap", name)
	}
	return nil
}

// deserializeMap converts a map[string]any to a controller config
// and coerces any values that are found in the validation schema.
func deserializeMap(m map[string]string) (map[string]any, error) {
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]any, len(m))
	for key, v := range m {
		if field, ok := fields[key]; ok {
			v, err := field.Coerce(m[key], []string{key})
			if err != nil {
				return nil, errors.Annotatef(err, "unable to coerce controller config key %q", key)
			}
			result[key] = v
			continue
		}

		result[key] = v
	}
	return result, nil
}

// validObjectStoreProgression validates that the object store type is allowed
// to be changed from the current config to the update config.
func validObjectStoreProgression(current map[string]string, updateAttrs controller.Config, removeAttrs []string) error {
	if contains(removeAttrs, controller.ObjectStoreType) {
		return errors.Errorf("can not remove %q", controller.ObjectStoreType)
	}

	// We should always have a valid object store type in the current config,
	// so we don't need to check for errors.
	cur := objectstore.BackendType(current[controller.ObjectStoreType])
	upd := updateAttrs.ObjectStoreType()

	// We're not changing the object store type, or we're changing from
	// filestorage to s3storage.
	if cur == upd || cur == objectstore.FileBackend && upd == objectstore.S3Backend {
		return nil
	}
	return errors.Errorf("can not change %q from %q to %q", controller.ObjectStoreType, cur, upd)
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// WatchableService defines a service for interacting with the underlying state
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the
// underlying state and the ability to create watchers.
func NewWatchableService(st State, wf WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: wf,
	}
}

// Watch returns a watcher that returns keys for any changes to controller
// config.
func (s *WatchableService) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher("controller_config", changestream.All, s.st.AllKeysQuery())
}
