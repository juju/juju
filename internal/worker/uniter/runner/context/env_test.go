// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"slices"
	"sort"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"

	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/semversion"
	coretrace "github.com/juju/juju/core/trace"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/api"
	apimocks "github.com/juju/juju/internal/worker/uniter/api/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type EnvSuite struct {
	testing.BaseSuite
}

func TestEnvSuite(t *stdtesting.T) {
	tc.Run(t, &EnvSuite{})
}

func (s *EnvSuite) assertVars(c *tc.C, actual []string, expect ...[]string) {
	c.Helper()

	var fullExpect []string
	for _, someExpect := range expect {
		fullExpect = append(fullExpect, someExpect...)
	}
	sort.Strings(actual)
	sort.Strings(fullExpect)
	c.Assert(actual, tc.DeepEquals, fullExpect)
}

func (s *EnvSuite) getPaths() (paths context.Paths, expectVars []string) {
	// note: path-munging is os-dependent, not included in expectVars
	return MockEnvPaths{}, []string{
		"CHARM_DIR=path-to-charm",
		"JUJU_CHARM_DIR=path-to-charm",
		"JUJU_AGENT_SOCKET_ADDRESS=path-to-jujuc.socket",
		"JUJU_AGENT_SOCKET_NETWORK=unix",
	}
}

func (s *EnvSuite) getHookContext(c *tc.C, newProxyOnly bool, uniter api.UniterClient, unit context.HookUnit) (ctx *context.HookContext, expectVars []string) {
	var (
		legacyProxy proxy.Settings
		jujuProxy   proxy.Settings
		proxy       = proxy.Settings{
			Http:    "some-http-proxy",
			Https:   "some-https-proxy",
			Ftp:     "some-ftp-proxy",
			NoProxy: "some-no-proxy",
		}
	)
	if newProxyOnly {
		jujuProxy = proxy
	} else {
		legacyProxy = proxy
	}

	expected := []string{
		"JUJU_CONTEXT_ID=some-context-id",
		"JUJU_HOOK_NAME=some-hook-name",
		"JUJU_MODEL_UUID=model-uuid-deadbeef",
		"JUJU_PRINCIPAL_UNIT=this-unit/123",
		"JUJU_MODEL_NAME=some-model-name",
		"JUJU_UNIT_NAME=this-unit/123",
		"JUJU_API_ADDRESSES=he.re:12345 the.re:23456",
		"JUJU_MACHINE_ID=42",
		"JUJU_AVAILABILITY_ZONE=some-zone",
		"JUJU_VERSION=1.2.3",
		"CLOUD_API_VERSION=6.66",

		// Tracing config is always included in the environment variables, even
		// if empty, to allow charms to write trace spans to the same OTEL
		// collector without needing to have multiple charm integrations.
		"JUJU_CHARM_TRACE_CONFIG_HTTP=",
		"JUJU_CHARM_TRACE_CONFIG_GRPC=",
		"JUJU_CHARM_TRACE_CONFIG_CA_CERT=",
	}
	if newProxyOnly {
		expected = append(expected,
			"JUJU_CHARM_HTTP_PROXY=some-http-proxy",
			"JUJU_CHARM_HTTPS_PROXY=some-https-proxy",
			"JUJU_CHARM_FTP_PROXY=some-ftp-proxy",
			"JUJU_CHARM_NO_PROXY=some-no-proxy",
		)
	} else {
		expected = append(expected,
			"http_proxy=some-http-proxy",
			"HTTP_PROXY=some-http-proxy",
			"https_proxy=some-https-proxy",
			"HTTPS_PROXY=some-https-proxy",
			"ftp_proxy=some-ftp-proxy",
			"FTP_PROXY=some-ftp-proxy",
			"no_proxy=some-no-proxy",
			"NO_PROXY=some-no-proxy",
			// JUJU_CHARM prefixed proxy values are always specified
			// even if empty.
			"JUJU_CHARM_HTTP_PROXY=",
			"JUJU_CHARM_HTTPS_PROXY=",
			"JUJU_CHARM_FTP_PROXY=",
			"JUJU_CHARM_NO_PROXY=",
		)
	}
	// It doesn't make sense that we set both legacy and juju proxy
	// settings, but by setting both to different values, we can see
	// what the environment values are.
	return context.NewModelHookContext(c, context.ModelHookContextParams{
		ID:                  "some-context-id",
		HookName:            "some-hook-name",
		ModelUUID:           "model-uuid-deadbeef",
		ModelName:           "some-model-name",
		UnitName:            "this-unit/123",
		AvailZone:           "some-zone",
		APIAddresses:        []string{"he.re:12345", "the.re:23456"},
		LegacyProxySettings: legacyProxy,
		JujuProxySettings:   jujuProxy,
		MachineTag:          names.NewMachineTag("42"),
		Uniter:              uniter,
		Unit:                unit,
	}), expected
}

func (s *EnvSuite) setSecret(ctx *context.HookContext) (expectVars []string) {
	url := secrets.NewURI()
	context.SetEnvironmentHookContextSecret(ctx, url.String(), nil, nil, nil)
	return []string{
		"JUJU_SECRET_ID=" + url.String(),
		"JUJU_SECRET_LABEL=label-" + url.String(),
		"JUJU_SECRET_REVISION=666",
	}
}

func (s *EnvSuite) setWorkload(ctx *context.HookContext) (expectVars []string) {
	workload := "wrk"
	context.SetEnvironmentHookContextWorkload(ctx, workload)
	return []string{
		"JUJU_WORKLOAD_NAME=" + workload,
	}
}

func (s *EnvSuite) setNotice(ctx *context.HookContext) (expectVars []string) {
	id := "1"
	typ := "custom"
	key := "a.com/b"
	context.SetEnvironmentHookContextNotice(ctx, id, typ, key)
	return []string{
		"JUJU_NOTICE_ID=" + id,
		"JUJU_NOTICE_TYPE=" + typ,
		"JUJU_NOTICE_KEY=" + key,
	}
}

// setCheck sets the context for a check hook.
func (s *EnvSuite) setCheck(ctx *context.HookContext) (expectVars []string) {
	name := "http-check"
	context.SetEnvironmentHookContextCheck(ctx, name)
	return []string{
		"JUJU_PEBBLE_CHECK_NAME=" + name,
	}
}

func (s *EnvSuite) setDepartingRelation(ctx *context.HookContext) (expectVars []string) {
	context.SetEnvironmentHookContextRelation(ctx, 22, "an-endpoint", "that-unit/456", "that-app", "that-unit/456")
	return []string{
		"JUJU_RELATION=an-endpoint",
		"JUJU_RELATION_ID=an-endpoint:22",
		"JUJU_REMOTE_UNIT=that-unit/456",
		"JUJU_REMOTE_APP=that-app",
		"JUJU_DEPARTING_UNIT=that-unit/456",
	}
}

func (s *EnvSuite) setStorage(ctx *context.HookContext) (expectVars []string) {
	tag := names.NewStorageTag("data/0")
	context.SetEnvironmentHookContextStorage(ctx, tag)
	return []string{
		"JUJU_STORAGE_ID=data/0",
		"JUJU_STORAGE_KIND=block",
		"JUJU_STORAGE_LOCATION=/dev/sdb",
	}
}

func (s *EnvSuite) TestHostEnv(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := apimocks.NewMockUniterClient(ctrl)
	state.EXPECT().StorageAttachment(gomock.Any(), names.NewStorageTag("data/0"), names.NewUnitTag("this-unit/123")).Return(params.StorageAttachment{
		Kind:     params.StorageKindBlock,
		Location: "/dev/sdb",
	}, nil).AnyTimes()
	unit := apimocks.NewMockUnit(ctrl)
	unit.EXPECT().Tag().Return(names.NewUnitTag("this-unit/123")).AnyTimes()

	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Ubuntu })
	s.PatchValue(&jujuversion.Current, semversion.MustParse("1.2.3"))

	ubuntuVars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"LANG=C.UTF-8",
		"PATH=path-to-tools:foo:bar",
		"TERM=tmux-256color",
	}

	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(k string) string {
			switch k {
			case "PATH":
				return "foo:bar"
			}
			return ""
		},
		func(k string) (string, bool) {
			switch k {
			case "PATH":
				return "foo:bar", true
			}
			return "", false
		},
	)

	//ctx, contextVars := s.getContext(false, state, unit)
	hookContext, contextVars := s.getHookContext(c, false, state, unit)
	paths, pathsVars := s.getPaths()
	actualVars, err := hookContext.HookVars(c.Context(), paths, environmenter)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVars(c, actualVars, contextVars, pathsVars, ubuntuVars)

	relationVars := s.setDepartingRelation(hookContext)
	secretVars := s.setSecret(hookContext)
	storageVars := s.setStorage(hookContext)
	workloadVars := s.setWorkload(hookContext)
	noticeVars := s.setNotice(hookContext)
	checkVars := s.setCheck(hookContext)
	actualVars, err = hookContext.HookVars(c.Context(), paths, environmenter)
	c.Assert(err, tc.ErrorIsNil)
	s.assertVars(c,
		actualVars,
		contextVars,
		pathsVars,
		ubuntuVars,
		relationVars,
		secretVars,
		storageVars,
		workloadVars,
		noticeVars,
		checkVars,
	)
}

func (s *EnvSuite) TestHookVarsIncludeW3CTraceContext(c *tc.C) {
	hookContext, _ := s.getHookContext(c, false, nil, nil)
	paths, _ := s.getPaths()
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return nil },
		func(string) string { return "" },
		func(string) (string, bool) { return "", false },
	)
	ctx := coretrace.WithSpan(c.Context(), traceSpan{scope: traceScope{
		traceID:    "80f198ee56343ba864fe8b2a57d3eff7",
		spanID:     "ff00000000000000",
		traceFlags: 1,
	}})

	vars, err := hookContext.HookVars(ctx, paths, environmenter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(slices.Contains(vars, "TRACEPARENT=00-80f198ee56343ba864fe8b2a57d3eff7-ff00000000000000-01"), tc.IsTrue)
	c.Check(slices.Contains(vars, "TRACESTATE="), tc.IsTrue)
	c.Check(slices.Contains(vars, "JUJU_TRACE_ID=80f198ee56343ba864fe8b2a57d3eff7"), tc.IsTrue)
	c.Check(slices.Contains(vars, "JUJU_SPAN_ID=ff00000000000000"), tc.IsTrue)
	c.Check(slices.Contains(vars, "JUJU_TRACE_FLAGS=1"), tc.IsTrue)
}

func (s *EnvSuite) TestContextDependentDoesNotIncludeUnSet(c *tc.C) {
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) { return "", false },
	)

	c.Assert(len(context.ContextDependentEnvVars(environmenter)), tc.Equals, 0)
}

func (s *EnvSuite) TestContextDependentDoesIncludeAll(c *tc.C) {
	counter := 0
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) {
			counter = counter + 1
			return "dummy-val", true
		},
	)
	c.Assert(len(context.ContextDependentEnvVars(environmenter)), tc.Equals, counter)
}

func (s *EnvSuite) TestContextDependentPartialInclude(c *tc.C) {
	counter := 0
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(_ string) (string, bool) {
			// We are just going to include the first two env vars here to make
			// sure that both true and false statements work
			if counter < 2 {
				counter = counter + 1
				return "dummy-val", true
			}
			return "", false
		},
	)

	c.Assert(len(context.ContextDependentEnvVars(environmenter)), tc.Equals, counter)
	c.Assert(counter, tc.Equals, 2)
}

func (s *EnvSuite) TestContextDependentCallsAllVarKeys(c *tc.C) {
	queriedVars := map[string]bool{}
	environmenter := context.NewRemoteEnvironmenter(
		func() []string { return []string{} },
		func(_ string) string { return "" },
		func(k string) (string, bool) {
			for _, envKey := range context.ContextAllowedEnvVars {
				if envKey == k && queriedVars[k] == false {
					queriedVars[k] = true
					return "dummy-val", true
				} else if envKey == k && queriedVars[envKey] == true {
					c.Errorf("key %q has been queried more than once", k)
					return "", false
				}
			}
			c.Errorf("unexpected key %q has been queried for", k)
			return "", false
		},
	)

	rval := context.ContextDependentEnvVars(environmenter)
	c.Assert(len(rval), tc.Equals, len(queriedVars))
	c.Assert(len(queriedVars), tc.Equals, len(context.ContextAllowedEnvVars))
}

type traceSpan struct {
	scope coretrace.Scope
}

func (s traceSpan) Scope() coretrace.Scope { return s.scope }

func (traceSpan) AddEvent(string, ...coretrace.Attribute) {}

func (traceSpan) RecordError(error, ...coretrace.Attribute) {}

func (traceSpan) End(...coretrace.Attribute) {}

type traceScope struct {
	traceID    string
	spanID     string
	traceFlags int
}

func (s traceScope) TraceID() string { return s.traceID }

func (s traceScope) SpanID() string { return s.spanID }

func (s traceScope) TraceFlags() int { return s.traceFlags }

func (s traceScope) IsSampled() bool { return s.traceFlags&1 == 1 }
