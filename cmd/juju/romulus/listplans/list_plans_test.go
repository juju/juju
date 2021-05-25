// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listplans_test

import (
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	api "github.com/juju/romulus/api/plan"
	wireformat "github.com/juju/romulus/wireformat/plan"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/juju/romulus/listplans"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

var (
	testPlan1 = `
    description:
        text: |
            Lorem ipsum dolor sit amet,
            consectetur adipiscing elit.
            Nunc pretium purus nec magna faucibus, sed eleifend dui fermentum. Nulla nec ornare lorem, sed imperdiet turpis. Nam auctor quis massa et commodo. Maecenas in magna erat. Duis non iaculis risus, a malesuada quam. Sed quis commodo sapien. Suspendisse laoreet diam eu interdum tristique. Class aptent taciti sociosqu ad litora torquent per conubia nostra, per inceptos himenaeos.
            Donec eu nunc quis eros fermentum porta non ut justo. Donec ut tempus sapien. Suspendisse bibendum fermentum eros, id feugiat justo elementum quis. Quisque vel volutpat risus. Aenean pellentesque ultrices consequat. Maecenas luctus, augue vitae ullamcorper vulputate, purus ligula accumsan diam, ut efficitur diam tellus ac nibh. Cras eros ligula, mattis in ex quis, porta efficitur quam. Donec porta, est ut interdum blandit, enim est elementum sapien, quis congue orci dui et nulla. Maecenas vehicula malesuada vehicula. Phasellus sapien ante, semper eu ornare sed, vulputate id nunc. Maecenas in orci mollis, sagittis lorem quis, ultrices metus. Integer molestie tempor augue, pulvinar blandit sapien ultricies eget.
            Fusce sed tellus sit amet tortor mollis pellentesque. Nulla tempus sem tellus, vitae tempor ipsum scelerisque eu. Cras tempor, tellus nec pretium egestas, felis massa luctus velit, vitae feugiat nunc velit ac tellus. Maecenas quis nisi diam. Sed pulvinar suscipit nibh sit amet cursus. Ut sem orci, consequat id pretium id, lacinia id nisl. Maecenas id quam at nisi eleifend porta. Vestibulum at ligula arcu. Quisque tincidunt pulvinar egestas. Ut suscipit ornare ligula a fermentum. Morbi ante justo, condimentum ut risus vitae, molestie elementum elit. Curabitur malesuada commodo diam sed ultrices. Vestibulum tincidunt turpis at ultricies fermentum. Morbi ipsum felis, laoreet quis risus id, ornare elementum urna. Morbi ultrices porttitor pulvinar. Maecenas facilisis velit sit amet tellus feugiat iaculis.
    metrics:
      pings:
        unit:
          transform: max
          period: hour
          gaps: zero
`
	testPlan2 = `
    metrics:
      pongs:
        unit:
          transform: max
          period: hour
          gaps: zero
`
)

type ListPlansCommandSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	mockAPI *mockapi
	stub    *testing.Stub
}

var _ = gc.Suite(&ListPlansCommandSuite{})

func (s *ListPlansCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.mockAPI = newMockAPI(s.stub)
	s.PatchValue(listplans.NewClient, listplans.APIClientFnc(s.mockAPI))
	s.PatchValue(&rcmd.GetMeteringURLForControllerCmd, func(c *modelcmd.ControllerCommandBase) (string, error) {
		return "http://example.com", nil
	})
}

func (s *ListPlansCommandSuite) TestTabularOutput(c *gc.C) {
	ctx, err := s.runCommand(c, &mockCharmResolver{
		ResolvedURL: "cs:series/some-charm-url",
		Stub:        s.stub,
	}, "cs:some-charm-url")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals,
		`Plan             	Price	Description                                       
bob/test-plan-1  	     	Lorem ipsum dolor sit amet,                       
                 	     	consectetur adipiscing elit.                      
                 	     	Nunc pretium purus nec magna faucibus, sed        
                 	     	eleifend dui fermentum. Nulla nec ornare lorem,   
                 	     	sed imperdiet turpis. Nam auctor quis massa et    
                 	     	commodo. Maecenas in magna erat. Duis non iaculis 
                 	     	risus, a malesuada quam. Sed quis commodo sapien. 
                 	     	Suspendisse laoreet diam eu interdum tristique.   
                 	     	Class aptent taciti sociosqu ad litora torquent   
                 	     	per conubia nostra, per inceptos himenaeos.       
                 	     	Donec eu nunc quis eros fermentum porta non ut    
                 	     	justo. Donec ut tempus sapien. Suspendisse        
                 	     	bibendum fermentum eros, id feugiat justo         
                 	     	elementum quis. Quisque vel volutpat risus. Aenean
                 	     	pellentesque ultrices consequat. Maecenas luctus, 
                 	     	augue vitae ullamcorper vulputate, purus ligula   
                 	     	accumsan diam, ut efficitur diam tellus ac nibh.  
                 	     	Cras eros ligula, mattis in ex quis, porta        
                 	     	efficitur quam. Donec porta, est ut interdum      
                 	     	blandit, enim est elementum sapien, quis congue   
                 	     	orci dui et nulla. Maecenas vehicula malesuada    
                 	     	vehicula. Phasellus sapien ante, semper eu ornare 
                 	     	sed, vulputate id nunc. Maecenas in orci mollis,  
                 	     	sagittis lorem quis, ultrices metus. Integer      
                 	     	molestie tempor augue, pulvinar blandit sapien    
                 	     	ultricies eget.                                   
                 	     	Fusce sed tellus sit amet tortor mollis           
                 	     	pellentesque. Nulla tempus sem tellus, vitae      
                 	     	tempor ipsum scelerisque eu. Cras tempor, tellus  
                 	     	nec pretium egestas, felis massa luctus velit,    
                 	     	vitae feugiat nunc velit ac tellus. Maecenas quis 
                 	     	nisi diam. Sed pulvinar suscipit nibh sit amet    
                 	     	cursus. Ut sem orci, consequat id pretium id,     
                 	     	lacinia id nisl. Maecenas id quam at nisi eleifend
                 	     	porta. Vestibulum at ligula arcu. Quisque         
                 	     	tincidunt pulvinar egestas. Ut suscipit ornare    
                 	     	ligula a fermentum. Morbi ante justo, condimentum 
                 	     	ut risus vitae, molestie elementum elit. Curabitur
                 	     	malesuada commodo diam sed ultrices. Vestibulum   
                 	     	tincidunt turpis at ultricies fermentum. Morbi    
                 	     	ipsum felis, laoreet quis risus id, ornare        
                 	     	elementum urna. Morbi ultrices porttitor pulvinar.
                 	     	Maecenas facilisis velit sit amet tellus feugiat  
                 	     	iaculis.                                          
                 	     	                                                  
carol/test-plan-2	     	                                                  
`)
}

func (s *ListPlansCommandSuite) runCommand(c *gc.C, resolver rcmd.CharmResolver, args ...string) (*cmd.Context, error) {
	cleanup := testing.PatchValue(&rcmd.NewCharmStoreResolverForControllerCmd, func(c *modelcmd.ControllerCommandBase) (rcmd.CharmResolver, error) {
		return resolver, nil
	})
	defer cleanup()
	cmd := listplans.NewListPlansCommand()
	cmd.SetClientStore(newMockStore())
	return cmdtesting.RunCommand(c, cmd, args...)
}

func newMockStore() *jujuclient.MemStore {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	return store
}

func (s *ListPlansCommandSuite) TestGetCommands(c *gc.C) {
	tests := []struct {
		about            string
		args             []string
		err              string
		resolvedCharmURL string
		apiCall          []interface{}
	}{{
		about:            "charm url is resolved",
		args:             []string{"cs:some-charm-url"},
		resolvedCharmURL: "cs:series/some-charm-url-1",
		apiCall:          []interface{}{"cs:series/some-charm-url-1"},
	}, {
		about:   "everything works - default format",
		args:    []string{"cs:some-charm-url"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "everything works - yaml",
		args:    []string{"cs:some-charm-url", "--format", "yaml"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "everything works - smart",
		args:    []string{"cs:some-charm-url", "--format", "smart"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "everything works - json",
		args:    []string{"cs:some-charm-url", "--format", "json"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "everything works - summary",
		args:    []string{"cs:some-charm-url", "--format", "summary"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "everything works - tabular",
		args:    []string{"cs:some-charm-url", "--format", "tabular"},
		apiCall: []interface{}{"cs:some-charm-url"},
	}, {
		about:   "missing argument",
		args:    []string{},
		err:     `missing arguments`,
		apiCall: []interface{}{},
	}, {
		about:   "invalid charm url",
		args:    []string{"some-url"},
		err:     `charm-store charm URLs are only supported`,
		apiCall: []interface{}{},
	}, {
		about:   "unknown arguments",
		args:    []string{"some-charm-url", "extra", "arguments"},
		err:     `unknown command line arguments: extra,arguments`,
		apiCall: []interface{}{},
	},
	}

	for i, t := range tests {
		c.Logf("Running test %d %s", i, t.about)
		s.mockAPI.reset()

		_, err := s.runCommand(c, &mockCharmResolver{
			ResolvedURL: t.resolvedCharmURL,
			Stub:        s.stub,
		}, t.args...)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			s.mockAPI.CheckCall(c, 0, "Resolve", t.args[0])
			s.mockAPI.CheckCall(c, 1, "GetAssociatedPlans", t.apiCall...)
		}
	}
}

// mockapi mocks the plan service api
type mockapi struct {
	*testing.Stub
	api.Client
}

func newMockAPI(s *testing.Stub) *mockapi {
	return &mockapi{Stub: s}
}

// Get implements the Get function of the api.PlanClient interface.
// TODO (domas) : fix once querying by charm url is in place
func (m *mockapi) GetAssociatedPlans(charmURL string) ([]wireformat.Plan, error) {
	m.AddCall("GetAssociatedPlans", charmURL)
	p1 := wireformat.Plan{
		URL:        "bob/test-plan-1",
		Definition: testPlan1,
		CreatedOn:  time.Date(2015, 0, 0, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	p2 := wireformat.Plan{
		URL:        "carol/test-plan-2",
		Definition: testPlan2,
		CreatedOn:  time.Date(2015, 0, 0, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
	return []wireformat.Plan{p1, p2}, m.NextErr()
}

func (m *mockapi) reset() {
	m.ResetCalls()
}

// mockCharmResolver is a mock implementation of cmd.CharmResolver.
type mockCharmResolver struct {
	*testing.Stub
	ResolvedURL string
}

// Resolve implements cmd.CharmResolver.
func (r *mockCharmResolver) Resolve(_ *httpbakery.Client, charmURL string) (string, error) {
	r.AddCall("Resolve", charmURL)
	if r.ResolvedURL != "" {
		return r.ResolvedURL, r.NextErr()
	}
	return charmURL, r.NextErr()
}
