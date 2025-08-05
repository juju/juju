// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

// ModificationValidatorFunc is a function that validates a modification
// to the controller config.
type ModificationValidatorFunc = func(map[string]string) error

// State defines an interface for interacting with the underlying state.
type State interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (map[string]string, error)

	// UpdateControllerConfig updates the controller config.
	UpdateControllerConfig(ctx context.Context, updateAttrs map[string]string, removeAttrs []string, validateModification ModificationValidatorFunc) error

	// AllKeysQuery is used to get the initial state
	// for the controller configuration watcher.
	AllKeysQuery() string

	// NamespaceForWatchControllerConfig returns the namespace identifier
	// used for watching controller configuration changes.
	NamespaceForWatchControllerConfig() []string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNamespaceWatcher(
		ctx context.Context,
		query eventsource.NamespaceQuery,
		summary string,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ctrlConfigMap, err := s.st.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Errorf("unable to get controller config: %w", err)
	}
	coerced, err := deserializeMap(ctrlConfigMap)
	if err != nil {
		return nil, errors.Errorf("unable to coerce controller config: %w", err)
	}

	// Get the controller UUID and CA cert from the config, so we can generate
	// a new controller config.
	var (
		ctrlUUID, caCert string
		ok               bool
	)
	if ctrlUUID, ok = coerced[controller.ControllerUUIDKey].(string); !ok {
		return nil, errors.Errorf("controller UUID %w", coreerrors.NotFound)
	}
	if caCert, ok = coerced[controller.CACertKey].(string); !ok {
		return nil, errors.Errorf("controller CACert %w", coreerrors.NotFound)
	}

	// Make a new controller config based on the coerced controller config map
	// returned from state.
	ctrlConfig, err := controller.NewConfig(ctrlUUID, caCert, coerced)
	if err != nil {
		return nil, errors.Errorf("unable to create controller config: %w", err)
	}
	return ctrlConfig, nil
}

// UpdateControllerConfig updates the controller config.
func (s *Service) UpdateControllerConfig(ctx context.Context, updateAttrs controller.Config, removeAttrs []string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	coerced, err := controller.EncodeToString(updateAttrs)
	if err != nil {
		return errors.Capture(err)
	}
	err = validateConfig(coerced, removeAttrs)
	if err != nil {
		return errors.Capture(err)
	}

	// Drop controller UUID, as we don't need to set it and it will be validated
	// in the validate config. It's not possible to update it.
	delete(coerced, controller.ControllerUUIDKey)

	err = s.st.UpdateControllerConfig(ctx, coerced, removeAttrs, func(current map[string]string) error {
		// Validate the updateAttrs against the current config.
		// This is done to ensure that the update config values are allowed
		// to be modified to the updated ones.
		//
		// For example, is it possible to move from filestorage to s3storage.
		// But it is not possible to move from s3storage to filestorage.
		if err := validObjectStoreProgression(current, updateAttrs, removeAttrs); err != nil {
			return errors.Capture(err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("updating controller config state: %w", err)
	}
	return nil
}

// validateConfig validates the given updateAttrs and removeAttrs.
func validateConfig(updateAttrs map[string]string, removeAttrs []string) error {
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return errors.Capture(err)
	}

	for k := range updateAttrs {
		if err := validateConfigField(k); err != nil {
			return errors.Capture(err)
		}
		if field, ok := fields[k]; ok {
			_, err := field.Coerce(updateAttrs[k], []string{k})
			if err != nil {
				return errors.Errorf("unable to coerce controller config key %q: %w", k, err)
			}
		}
	}
	for _, r := range removeAttrs {
		if err := validateConfigField(r); err != nil {
			return errors.Capture(err)
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

// deserializeMap converts a map[string]any to a controller config
// and coerces any values that are found in the validation schema.
func deserializeMap(m map[string]string) (map[string]any, error) {
	fields, _, err := controller.ConfigSchema.ValidationSchema()
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]any, len(m))
	for key, v := range m {
		if field, ok := fields[key]; ok {
			v, err := field.Coerce(m[key], []string{key})
			if err != nil {
				return nil, errors.Errorf("unable to coerce controller config key %q: %w", key, err)
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

	// If we're not changing the object store type, we don't need to validate
	// anything.
	if _, ok := updateAttrs[controller.ObjectStoreType]; !ok {
		return nil
	}

	// We should always have a valid object store type in the current config,
	// so we don't need to check for errors.
	cur := objectstore.BackendType(current[controller.ObjectStoreType])
	upd := updateAttrs.ObjectStoreType()

	// We're not changing the object store type, or we're changing from
	// filestorage to s3storage.
	if cur == upd {
		return nil
	} else if cur == objectstore.FileBackend && upd == objectstore.S3Backend {
		// To be 100% sure that we can change from filestorage to s3storage,
		// we're going to check if the updated config will have a complete s3 config.
		// This is rather expensive, but it's the only way to be sure that we
		// can change from filestorage to s3storage.
		if err := updateCompletesS3Config(current, updateAttrs); err != nil {
			return errors.Errorf("can not change %q from %q to %q without complete s3 config: %w", controller.ObjectStoreType, cur, upd, err)
		}
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

func updateCompletesS3Config(config map[string]string, updateAttrs controller.Config) error {
	endpoint := updateAttrs.ObjectStoreS3Endpoint()
	if endpoint == "" {
		endpoint = config[controller.ObjectStoreS3Endpoint]
	}
	staticKey := updateAttrs.ObjectStoreS3StaticKey()
	if staticKey == "" {
		staticKey = config[controller.ObjectStoreS3StaticKey]
	}
	secretKey := updateAttrs.ObjectStoreS3StaticSecret()
	if secretKey == "" {
		secretKey = config[controller.ObjectStoreS3StaticSecret]
	}
	return controller.HasCompleteS3Config(endpoint, staticKey, secretKey)
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
func (s *WatchableService) WatchControllerConfig(ctx context.Context) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespaces := s.st.NamespaceForWatchControllerConfig()
	if len(namespaces) == 0 {
		return nil, errors.Errorf("no namespaces for watching controller config")
	}

	filters := make([]eventsource.FilterOption, 0, len(namespaces))
	for _, ns := range namespaces {
		filters = append(filters, eventsource.NamespaceFilter(ns, changestream.All))
	}

	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(s.st.AllKeysQuery()),
		"controller config watcher",
		filters[0], filters[1:]...,
	)
}
