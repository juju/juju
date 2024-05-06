// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package validators

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type dummySecretBackendProviderFunc func(string) (bool, error)

type dummySpaceProviderFunc func(string) (bool, error)

type validatorsSuite struct{}

var _ = gc.Suite(&validatorsSuite{})

func (d dummySecretBackendProviderFunc) HasSecretsBackend(s string) (bool, error) {
	return d(s)
}

func (d dummySpaceProviderFunc) HasSpace(s string) (bool, error) {
	return d(s)
}

func (*validatorsSuite) TestCharmhubURLChange(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":         "wallyworld",
		"uuid":         testing.ModelTag.Id(),
		"type":         "sometype",
		"charmhub-url": "https://charmhub.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":         "wallyworld",
		"uuid":         testing.ModelTag.Id(),
		"type":         "sometype",
		"charmhub-url": "https://charmhub1.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)

	var validationError *config.ValidationError
	_, err = CharmhubURLChange()(context.Background(), newCfg, oldCfg)
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"charmhub-url"})
}

func (*validatorsSuite) TestCharmhubURLNoChange(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":         "wallyworld",
		"uuid":         testing.ModelTag.Id(),
		"type":         "sometype",
		"charmhub-url": "https://charmhub.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":         "wallyworld",
		"uuid":         testing.ModelTag.Id(),
		"type":         "sometype",
		"charmhub-url": "https://charmhub.example.com",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = CharmhubURLChange()(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestAgentVersionChanged(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"agent-version": "1.2.3",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"agent-version": "1.3.0",
	})
	c.Assert(err, jc.ErrorIsNil)

	var validationError *config.ValidationError
	_, err = AgentVersionChange()(context.Background(), newCfg, oldCfg)
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"agent-version"})
}

// TestAgentVersionNoChange is testing that if the agent version doesn't change
// between config changes no validation error is produced.
func (*validatorsSuite) TestAgentVersionNoChange(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"agent-version": "1.3.0",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"agent-version": "1.3.0",
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := AgentVersionChange()(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
	_, agentVersionSet := cfg.AgentVersion()
	c.Check(agentVersionSet, jc.IsFalse)

	oldCfg, err = config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": testing.ModelTag.Id(),
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err = config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"agent-version": "1.3.0",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = AgentVersionChange()(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestSpaceCheckerFound(c *gc.C) {
	provider := dummySpaceProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "foobar")
		return true, nil
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SpaceChecker(provider)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestSpaceCheckerNotFound(c *gc.C) {
	provider := dummySpaceProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "foobar")
		return false, nil
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SpaceChecker(provider)(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"default-space"})
}

func (*validatorsSuite) TestSpaceCheckerError(c *gc.C) {
	providerErr := errors.New("some error")
	provider := dummySpaceProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "foobar")
		return false, providerErr
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":          "wallyworld",
		"uuid":          testing.ModelTag.Id(),
		"type":          "sometype",
		"default-space": "foobar",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SpaceChecker(provider)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIs, providerErr)
}

func (*validatorsSuite) TestLoggincTracePermissionNoTrace(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": testing.ModelTag.Id(),
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"logging-config": "root=DEBUG",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = LoggingTracePermissionChecker(false)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestLoggincTracePermissionTrace(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": testing.ModelTag.Id(),
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"logging-config": "root=TRACE",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = LoggingTracePermissionChecker(false)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIs, ErrorLogTracingPermission)

	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"logging-config"})
}

func (*validatorsSuite) TestLoggincTracePermissionTraceAllow(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name": "wallyworld",
		"uuid": testing.ModelTag.Id(),
		"type": "sometype",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"logging-config": "root=TRACE",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = LoggingTracePermissionChecker(true)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestSecretsBackendChecker(c *gc.C) {
	provider := dummySecretBackendProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "vault")
		return true, nil
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "vault",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SecretBackendChecker(provider)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (*validatorsSuite) TestSecretsBackendCheckerNoExist(c *gc.C) {
	provider := dummySecretBackendProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "vault")
		return false, nil
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "vault",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SecretBackendChecker(provider)(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"secret-backend"})
}

func (*validatorsSuite) TestSecretsBackendCheckerProviderError(c *gc.C) {
	providerErr := errors.New("some error")
	provider := dummySecretBackendProviderFunc(func(s string) (bool, error) {
		c.Assert(s, gc.Equals, "vault")
		return false, providerErr
	})

	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "default",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":           "wallyworld",
		"uuid":           testing.ModelTag.Id(),
		"type":           "sometype",
		"secret-backend": "vault",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = SecretBackendChecker(provider)(context.Background(), newCfg, oldCfg)
	c.Assert(err, jc.ErrorIs, providerErr)
}

// TestAuthorizedKeysChanged asserts that if we change the value of authorised
// keys between two revisions of model config we get a [config.ValidationError]
func (*validatorsSuite) TestAuthorizedKeysChanged(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":            "wallyworld",
		"uuid":            testing.ModelTag.Id(),
		"type":            "sometype",
		"authorized-keys": "123",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":            "wallyworld",
		"uuid":            testing.ModelTag.Id(),
		"type":            "sometype",
		"authorized-keys": "123,456",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = AuthorizedKeysChange()(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"authorized-keys"})
}

// TestAuthorizedKeysChangedNoChange asserts that if don't change the value of
// authorized-keys between model config revisions no error is produced.
func (*validatorsSuite) TestAuthorizedKeysChangedNoChange(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":            "wallyworld",
		"uuid":            testing.ModelTag.Id(),
		"type":            "sometype",
		"authorized-keys": "123",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":            "wallyworld",
		"uuid":            testing.ModelTag.Id(),
		"type":            "sometype",
		"authorized-keys": "123",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = AuthorizedKeysChange()(context.Background(), newCfg, oldCfg)
	c.Check(err, jc.ErrorIsNil)
}
