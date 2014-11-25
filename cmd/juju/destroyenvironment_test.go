// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type destroyEnvSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&destroyEnvSuite{})

func (s *destroyEnvSuite) TestDestroyEnvironmentCommand(c *gc.C) {
	// Prepare the environment so we can destroy it.
	_, err := environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)

	// check environment is mandatory
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand))
	c.Check(<-errc, gc.Equals, NoEnvironmentError)

	// normal destroy
	opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")

	// Verify that the environment information has been removed.
	_, err = s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// startEnvironment prepare the environment so we can destroy it.
func (s *destroyEnvSuite) startEnvironment(c *gc.C, desiredEnvName string) {
	_, err := environs.PrepareFromName(desiredEnvName, envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *destroyEnvSuite) checkDestroyEnvironment(c *gc.C, blocked, force bool) {
	//Setup environment
	envName := "dummyenv"
	s.startEnvironment(c, envName)
	s.AssertConfigParameterUpdated(c, "block-destroy-environment", blocked)
	opc := make(chan dummy.Operation)
	errc := make(chan error)
	if force {
		opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), envName, "--yes", "--force")
	} else {
		opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), envName, "--yes")
	}
	if force || !blocked {
		c.Check(<-errc, gc.IsNil)
		c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, envName)
		// Verify that the environment information has been removed.
		_, err := s.ConfigStore.ReadInfo(envName)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	} else {
		c.Check(<-errc, gc.Not(gc.IsNil))
		c.Check((<-opc), gc.IsNil)
		// Verify that the environment information has not been removed.
		_, err := s.ConfigStore.ReadInfo(envName)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *destroyEnvSuite) TestDestroyLockedEnvironment(c *gc.C) {
	// lock environment: can't destroy locked environment
	s.checkDestroyEnvironment(c, true, false)
}

func (s *destroyEnvSuite) TestDestroyUnlockedEnvironment(c *gc.C) {
	s.checkDestroyEnvironment(c, false, false)
}

func (s *destroyEnvSuite) TestForceDestroyLockedEnvironment(c *gc.C) {
	s.checkDestroyEnvironment(c, true, true)
}

func (s *destroyEnvSuite) TestForceDestroyUnlockedEnvironment(c *gc.C) {
	s.checkDestroyEnvironment(c, false, true)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandEFlag(c *gc.C) {
	// Prepare the environment so we can destroy it.
	_, err := environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)

	// check that either environment or the flag is mandatory
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand))
	c.Check(<-errc, gc.Equals, NoEnvironmentError)

	// We don't allow them to supply both entries at the same time
	opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), "-e", "dummyenv", "dummyenv", "--yes")
	c.Check(<-errc, gc.Equals, DoubleEnvironmentError)
	// We treat --environment the same way
	opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), "--environment", "dummyenv", "dummyenv", "--yes")
	c.Check(<-errc, gc.Equals, DoubleEnvironmentError)

	// destroy using the -e flag
	opc, errc = cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), "-e", "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")

	// Verify that the environment information has been removed.
	_, err = s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandEmptyJenv(c *gc.C) {
	info := s.ConfigStore.CreateInfo("emptyenv")
	err := info.Write()
	c.Assert(err, jc.ErrorIsNil)

	context, err := coretesting.RunCommand(c, new(DestroyEnvironmentCommand), "-e", "emptyenv")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(coretesting.Stderr(context), gc.Equals, "removing empty environment file\n")
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandBroken(c *gc.C) {
	oldinfo, err := s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.ErrorIsNil)
	bootstrapConfig := oldinfo.BootstrapConfig()
	apiEndpoint := oldinfo.APIEndpoint()
	apiCredentials := oldinfo.APICredentials()
	err = oldinfo.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	newinfo := s.ConfigStore.CreateInfo("dummyenv")

	bootstrapConfig["broken"] = "Destroy"
	newinfo.SetBootstrapConfig(bootstrapConfig)
	newinfo.SetAPIEndpoint(apiEndpoint)
	newinfo.SetAPICredentials(apiCredentials)
	err = newinfo.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Prepare the environment so we can destroy it.
	_, err = environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)

	// destroy with broken environment
	opc, errc := cmdtesting.RunCommand(cmdtesting.NullContext(c), new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	op, ok := (<-opc).(dummy.OpDestroy)
	c.Assert(ok, jc.IsTrue)
	c.Assert(op.Error, gc.ErrorMatches, "dummy.Destroy is broken")
	c.Check(<-errc, gc.Equals, op.Error)
	c.Check(<-opc, gc.IsNil)
}

func (*destroyEnvSuite) TestDestroyEnvironmentCommandConfirmationFlag(c *gc.C) {
	com := new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv"}), gc.IsNil)
	c.Check(com.assumeYes, jc.IsFalse)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv", "-y"}), gc.IsNil)
	c.Check(com.assumeYes, jc.IsTrue)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv", "--yes"}), gc.IsNil)
	c.Check(com.assumeYes, jc.IsTrue)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Prepare the environment so we can destroy it.
	env, err := environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
	c.Assert(err, jc.ErrorIsNil)

	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	opc, errc := cmdtesting.RunCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
	c.Check(<-errc, gc.ErrorMatches, "environment destruction aborted")
	c.Check(<-opc, gc.IsNil)
	c.Check(stdout.String(), gc.Matches, "WARNING!.*dummyenv.*\\(type: dummy\\)(.|\n)*")
	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	opc, errc = cmdtesting.RunCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
	c.Check(<-opc, gc.IsNil)
	c.Check(<-errc, gc.ErrorMatches, "environment destruction aborted")
	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// "--yes" passed: no confirmation request.
	stdin.Reset()
	stdout.Reset()
	opc, errc = cmdtesting.RunCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")
	c.Check(stdout.String(), gc.Equals, "")
	assertEnvironDestroyed(c, env, s.ConfigStore)

	// Any of casing of "y" and "yes" will confirm.
	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		// Prepare the environment so we can destroy it.
		s.Reset(c)
		env, err := environs.PrepareFromName("dummyenv", envcmd.BootstrapContext(cmdtesting.NullContext(c)), s.ConfigStore)
		c.Assert(err, jc.ErrorIsNil)

		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		opc, errc = cmdtesting.RunCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
		c.Check(<-errc, gc.IsNil)
		c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")
		c.Check(stdout.String(), gc.Matches, "WARNING!.*dummyenv.*\\(type: dummy\\)(.|\n)*")
		assertEnvironDestroyed(c, env, s.ConfigStore)
	}
}

func assertEnvironDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	_, err := store.ReadInfo(env.Config().Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = env.Instances([]instance.Id{"invalid"})
	c.Assert(err, gc.ErrorMatches, "environment has been destroyed")
}

func assertEnvironNotDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	info, err := store.ReadInfo(env.Config().Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	_, err = environs.NewFromName(env.Config().Name(), store)
	c.Assert(err, jc.ErrorIsNil)
}
