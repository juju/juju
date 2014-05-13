// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
)

type destroyEnvSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&destroyEnvSuite{})

func (s *destroyEnvSuite) TestDestroyEnvironmentCommand(c *gc.C) {
	// Prepare the environment so we can destroy it.
	_, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)

	// check environment is mandatory
	opc, errc := runCommand(nullContext(c), new(DestroyEnvironmentCommand))
	c.Check(<-errc, gc.Equals, NoEnvironmentError)

	// normal destroy
	opc, errc = runCommand(nullContext(c), new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")

	// Verify that the environment information has been removed.
	_, err = s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandEFlag(c *gc.C) {
	// Prepare the environment so we can destroy it.
	_, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)

	// check that either environment or the flag is mandatory
	opc, errc := runCommand(nullContext(c), new(DestroyEnvironmentCommand))
	c.Check(<-errc, gc.Equals, NoEnvironmentError)

	// We don't allow them to supply both entries at the same time
	opc, errc = runCommand(nullContext(c), new(DestroyEnvironmentCommand), "-e", "dummyenv", "dummyenv", "--yes")
	c.Check(<-errc, gc.Equals, DoubleEnvironmentError)
	// We treat --environment the same way
	opc, errc = runCommand(nullContext(c), new(DestroyEnvironmentCommand), "--environment", "dummyenv", "dummyenv", "--yes")
	c.Check(<-errc, gc.Equals, DoubleEnvironmentError)

	// destroy using the -e flag
	opc, errc = runCommand(nullContext(c), new(DestroyEnvironmentCommand), "-e", "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")

	// Verify that the environment information has been removed.
	_, err = s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandEmptyJenv(c *gc.C) {
	_, err := s.ConfigStore.CreateInfo("emptyenv")
	c.Assert(err, gc.IsNil)

	context, err := coretesting.RunCommand(c, new(DestroyEnvironmentCommand), []string{"-e", "emptyenv"})
	c.Assert(err, gc.IsNil)

	c.Assert(coretesting.Stderr(context), gc.Equals, "removing empty environment file\n")
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandBroken(c *gc.C) {
	oldinfo, err := s.ConfigStore.ReadInfo("dummyenv")
	c.Assert(err, gc.IsNil)
	bootstrapConfig := oldinfo.BootstrapConfig()
	apiEndpoint := oldinfo.APIEndpoint()
	apiCredentials := oldinfo.APICredentials()
	err = oldinfo.Destroy()
	c.Assert(err, gc.IsNil)
	newinfo, err := s.ConfigStore.CreateInfo("dummyenv")
	c.Assert(err, gc.IsNil)

	bootstrapConfig["broken"] = "Destroy"
	newinfo.SetBootstrapConfig(bootstrapConfig)
	newinfo.SetAPIEndpoint(apiEndpoint)
	newinfo.SetAPICredentials(apiCredentials)
	err = newinfo.Write()
	c.Assert(err, gc.IsNil)

	// Prepare the environment so we can destroy it.
	_, err = environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)

	// destroy with broken environment
	opc, errc := runCommand(nullContext(c), new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	op, ok := (<-opc).(dummy.OpDestroy)
	c.Assert(ok, jc.IsTrue)
	c.Assert(op.Error, gc.ErrorMatches, "dummy.Destroy is broken")
	c.Check(<-errc, gc.Equals, op.Error)
	c.Check(<-opc, gc.IsNil)
}

func (*destroyEnvSuite) TestDestroyEnvironmentCommandConfirmationFlag(c *gc.C) {
	com := new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv"}), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, false)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv", "-y"}), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, true)

	com = new(DestroyEnvironmentCommand)
	c.Check(coretesting.InitCommand(com, []string{"dummyenv", "--yes"}), gc.IsNil)
	c.Check(com.assumeYes, gc.Equals, true)
}

func (s *destroyEnvSuite) TestDestroyEnvironmentCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, gc.IsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Prepare the environment so we can destroy it.
	env, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
	c.Assert(err, gc.IsNil)

	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	opc, errc := runCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
	c.Check(<-errc, gc.ErrorMatches, "environment destruction aborted")
	c.Check(<-opc, gc.IsNil)
	c.Check(stdout.String(), gc.Matches, "WARNING!.*dummyenv.*\\(type: dummy\\)(.|\n)*")
	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// EOF on stdin: equivalent to answering no.
	stdin.Reset()
	stdout.Reset()
	opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
	c.Check(<-opc, gc.IsNil)
	c.Check(<-errc, gc.ErrorMatches, "environment destruction aborted")
	assertEnvironNotDestroyed(c, env, s.ConfigStore)

	// "--yes" passed: no confirmation request.
	stdin.Reset()
	stdout.Reset()
	opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv", "--yes")
	c.Check(<-errc, gc.IsNil)
	c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")
	c.Check(stdout.String(), gc.Equals, "")
	assertEnvironDestroyed(c, env, s.ConfigStore)

	// Any of casing of "y" and "yes" will confirm.
	for _, answer := range []string{"y", "Y", "yes", "YES"} {
		// Prepare the environment so we can destroy it.
		s.Reset(c)
		env, err := environs.PrepareFromName("dummyenv", nullContext(c), s.ConfigStore)
		c.Assert(err, gc.IsNil)

		stdin.Reset()
		stdout.Reset()
		stdin.WriteString(answer)
		opc, errc = runCommand(ctx, new(DestroyEnvironmentCommand), "dummyenv")
		c.Check(<-errc, gc.IsNil)
		c.Check((<-opc).(dummy.OpDestroy).Env, gc.Equals, "dummyenv")
		c.Check(stdout.String(), gc.Matches, "WARNING!.*dummyenv.*\\(type: dummy\\)(.|\n)*")
		assertEnvironDestroyed(c, env, s.ConfigStore)
	}
}

func assertEnvironDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	_, err := store.ReadInfo(env.Name())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = env.Instances([]instance.Id{"invalid"})
	c.Assert(err, gc.ErrorMatches, "environment has been destroyed")
}

func assertEnvironNotDestroyed(c *gc.C, env environs.Environ, store configstore.Storage) {
	info, err := store.ReadInfo(env.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(info.Initialized(), jc.IsTrue)

	_, err = environs.NewFromName(env.Name(), store)
	c.Assert(err, gc.IsNil)
}
