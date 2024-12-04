// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package validators

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testing"
)

type dummySpaceProviderFunc func(context.Context, string) (bool, error)

type validatorsSuite struct{}

var _ = gc.Suite(&validatorsSuite{})

func (d dummySpaceProviderFunc) HasSpace(ctx context.Context, s string) (bool, error) {
	return d(ctx, s)
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
	provider := dummySpaceProviderFunc(func(ctx context.Context, s string) (bool, error) {
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
	provider := dummySpaceProviderFunc(func(ctx context.Context, s string) (bool, error) {
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
	provider := dummySpaceProviderFunc(func(ctx context.Context, s string) (bool, error) {
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

// TestContainerNetworkingMethodValueValid asserts that valid container
// networking method values are accepted by model config.
func (*validatorsSuite) TestContainerNetworkingMethodValueValid(c *gc.C) {
	validContainerNetworkingMethods := []string{"", "local", "provider"}

	for _, containerNetworkingMethod := range validContainerNetworkingMethods {
		cfg, err := config.New(config.NoDefaults, map[string]any{
			"name":                              "wallyworld",
			"uuid":                              testing.ModelTag.Id(),
			"type":                              "sometype",
			config.ContainerNetworkingMethodKey: containerNetworkingMethod,
		})
		c.Assert(err, jc.ErrorIsNil)

		_, err = ContainerNetworkingMethodValue()(context.Background(), cfg, nil)
		c.Check(err, jc.ErrorIsNil)
	}
}

// TestContainerNetworkingMethodChanged asserts that if we change the
// container networking method between two revisions of model config, we get a
// [config.ValidationError].
func (*validatorsSuite) TestContainerNetworkingMethodChanged(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":                              "wallyworld",
		"uuid":                              testing.ModelTag.Id(),
		"type":                              "sometype",
		config.ContainerNetworkingMethodKey: "provider",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":                              "wallyworld",
		"uuid":                              testing.ModelTag.Id(),
		"type":                              "sometype",
		config.ContainerNetworkingMethodKey: "local",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = ContainerNetworkingMethodChange()(context.Background(), newCfg, oldCfg)
	var validationError *config.ValidationError
	c.Assert(errors.As(err, &validationError), jc.IsTrue)
	c.Assert(validationError.InvalidAttrs, gc.DeepEquals, []string{"container-networking-method"})
}

// TestContainerNetworkingMethodNoChange asserts that if we don't change the
// container networking method between model config revisions, no error is
// produced.
func (*validatorsSuite) TestContainerNetworkingMethodNoChange(c *gc.C) {
	oldCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":                              "wallyworld",
		"uuid":                              testing.ModelTag.Id(),
		"type":                              "sometype",
		config.ContainerNetworkingMethodKey: "provider",
	})
	c.Assert(err, jc.ErrorIsNil)

	newCfg, err := config.New(config.NoDefaults, map[string]any{
		"name":                              "wallyworld",
		"uuid":                              testing.ModelTag.Id(),
		"type":                              "sometype",
		config.ContainerNetworkingMethodKey: "provider",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = ContainerNetworkingMethodChange()(context.Background(), newCfg, oldCfg)
	c.Check(err, jc.ErrorIsNil)
}
