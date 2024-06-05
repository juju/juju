// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/controller"
)

// AggregateValidator is a Validator that will run zero or more validators,
// making sure that all succeed or returning the first error encountered.
type AggregateValidator struct {
	// Validators is the slice of Validator's to run as part of the aggregate
	// check.
	Validators []Validator
}

// Validator is an interface for validating model configuration.
type Validator interface {
	// Validate ensures that cfg is a valid configuration.
	// If old is not nil, Validate should use it to determine
	// whether a configuration change is valid.
	//
	// TODO(axw) Validate should just return an error. We should
	// use a separate mechanism for updating config.
	Validate(ctx context.Context, cfg, old *Config) (valid *Config, _ error)
}

// ValidatorFunc is utility type for declaring funcs that implement the
// Validator interface.
type ValidatorFunc func(ctx context.Context, cfg, old *Config) (*Config, error)

// ValidationError represents a specific error that has occurred were validating
// Config. It allows for the placement of one or more attributes with a reason
// as to why either their keys or values are not valid in the current context.
type ValidationError struct {
	// InvalidAttrs is the list of attributes in the config that where invalid.
	InvalidAttrs []string

	// Cause is the reason for why the attributes are invalid.
	Cause error
}

var (
	// disallowedModelConfigAttrs is the set of config attributes that should
	// not be allowed to appear in model config.
	disallowedModelConfigAttrs = [...]string{
		AgentVersionKey,
		AdminSecretKey,
		CAPrivateKeyKey,
	}
)

// Is implements errors.Is interface. We implement is so that the
// ValidationError also satisfies NotValid.
func (v *ValidationError) Is(err error) bool {
	return err == errors.NotValid || err == v
}

// Error implements Error interface.
func (v *ValidationError) Error() string {
	if v.Cause == nil {
		return fmt.Sprintf("config attributes %v not valid", v.InvalidAttrs)
	}
	return fmt.Sprintf("config attributes %v not valid because %s", v.InvalidAttrs, v.Cause)
}

// Validate implements Validator validate interface. This func will run all the
// validators in the aggregate till either a validator errors or there are no
// more validators to run. The returned config from each validator is passed
// into the subsequent validator.
func (a *AggregateValidator) Validate(ctx context.Context, cfg, old *Config) (*Config, error) {
	var err error
	for i, validator := range a.Validators {
		cfg, err = validator.Validate(ctx, cfg, old)
		if err != nil {
			return cfg, fmt.Errorf("config validator %d failed: %w", i, err)
		}
	}
	return cfg, nil
}

// Validate implements the Validator interface.
func (v ValidatorFunc) Validate(ctx context.Context, cfg, old *Config) (*Config, error) {
	return v(ctx, cfg, old)
}

// NoControllerAttributesValidator implements a validator that asserts if the
// supplied config contains any controller specific configuration attributes. A
// ValidationError is returned if the config contains controller attributes.
func NoControllerAttributesValidator() Validator {
	return ValidatorFunc(func(ctx context.Context, cfg, _ *Config) (*Config, error) {
		invalidKeysError := ValidationError{
			InvalidAttrs: []string{},
			Cause:        errors.ConstError("controller only attributes not allowed"),
		}
		allAttrs := cfg.AllAttrs()
		for _, attr := range controller.ControllerOnlyConfigAttributes {
			if _, has := allAttrs[attr]; has {
				invalidKeysError.InvalidAttrs = append(invalidKeysError.InvalidAttrs, attr)
			}
		}

		if len(invalidKeysError.InvalidAttrs) != 0 {
			return cfg, &invalidKeysError
		}
		return cfg, nil
	})
}

// ModelValidator returns a validator that is suitable for validating model
// config. Any attributes found that are not supported in Model Configuration
// are returned in a ValidationError.
func ModelValidator() Validator {
	modelConfigValidator := ValidatorFunc(func(ctx context.Context, cfg, _ *Config) (*Config, error) {
		invalidKeysError := ValidationError{
			InvalidAttrs: []string{},
			Cause:        errors.ConstError("attributes not allowed"),
		}
		allAttrs := cfg.AllAttrs()
		for _, attr := range disallowedModelConfigAttrs {
			if _, has := allAttrs[attr]; has {
				invalidKeysError.InvalidAttrs = append(invalidKeysError.InvalidAttrs, attr)
			}
		}

		if len(invalidKeysError.InvalidAttrs) != 0 {
			return cfg, &invalidKeysError
		}
		return cfg, nil
	})

	return &AggregateValidator{
		Validators: []Validator{
			modelConfigValidator,
			NoControllerAttributesValidator(),
		},
	}
}
