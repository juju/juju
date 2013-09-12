// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"

	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
)

type instanceTypeSuite struct{}

var _ = gc.Suite(&instanceTypeSuite{})

func (*instanceTypeSuite) TestNewPreferredTypesAcceptsNil(c *gc.C) {
	types := newPreferredTypes(nil)

	c.Check(types, gc.HasLen, 0)
	c.Check(types.Len(), gc.Equals, 0)
}

func (*instanceTypeSuite) TestNewPreferredTypesRepresentsInput(c *gc.C) {
	availableTypes := []gwacl.RoleSize{{Name: "Humongous", Cost: 123}}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	c.Check(types[0], gc.Equals, &availableTypes[0])
	c.Check(types.Len(), gc.Equals, len(availableTypes))
}

func (*instanceTypeSuite) TestNewPreferredTypesSortsByCost(c *gc.C) {
	availableTypes := []gwacl.RoleSize{
		{Name: "Excessive", Cost: 12},
		{Name: "Ridiculous", Cost: 99},
		{Name: "Modest", Cost: 3},
	}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	// We end up with machine types sorted by ascending cost.
	c.Check(types[0].Name, gc.Equals, "Modest")
	c.Check(types[1].Name, gc.Equals, "Excessive")
	c.Check(types[2].Name, gc.Equals, "Ridiculous")
}

func (*instanceTypeSuite) TestLessComparesCost(c *gc.C) {
	types := preferredTypes{
		{Name: "Cheap", Cost: 1},
		{Name: "Posh", Cost: 200},
	}

	c.Check(types.Less(0, 1), gc.Equals, true)
	c.Check(types.Less(1, 0), gc.Equals, false)
}

func (*instanceTypeSuite) TestSwapSwitchesEntries(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(0, 1)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*instanceTypeSuite) TestSwapIsCommutative(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(1, 0)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*instanceTypeSuite) TestSwapLeavesOtherEntriesIntact(c *gc.C) {
	types := preferredTypes{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
		{Name: "D"},
	}

	types.Swap(1, 2)

	c.Check(types[0].Name, gc.Equals, "A")
	c.Check(types[1].Name, gc.Equals, "C")
	c.Check(types[2].Name, gc.Equals, "B")
	c.Check(types[3].Name, gc.Equals, "D")
}

func (*instanceTypeSuite) TestSufficesAcceptsNilRequirement(c *gc.C) {
	types := preferredTypes{}
	c.Check(types.suffices(0, nil), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesAcceptsMetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesAcceptsExactRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesRejectsUnmetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation-1, &expectation), gc.Equals, false)
}

func (*instanceTypeSuite) TestSatisfiesComparesCPUCores(c *gc.C) {
	types := preferredTypes{}
	var desiredCores uint64 = 5
	constraint := constraints.Value{CpuCores: &desiredCores}

	// A machine with fewer cores than required does not satisfy...
	machine := gwacl.RoleSize{CpuCores: desiredCores - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more cores.
	machine.CpuCores = desiredCores
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*instanceTypeSuite) TestSatisfiesComparesMem(c *gc.C) {
	types := preferredTypes{}
	var desiredMem uint64 = 37
	constraint := constraints.Value{Mem: &desiredMem}

	// A machine with less memory than required does not satisfy...
	machine := gwacl.RoleSize{Mem: desiredMem - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more memory.
	machine.Mem = desiredMem
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecSetsMimimumMem(c *gc.C) {
	c.Check(
		*defaultToBaselineSpec(constraints.Value{}).Mem,
		gc.Equals,
		uint64(defaultMem))
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesOriginalIntact(c *gc.C) {
	original := constraints.Value{}
	defaultToBaselineSpec(original)
	c.Check(original.Mem, gc.IsNil)
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesLowerMemIntact(c *gc.C) {
	const low = 100 * gwacl.MB
	var value uint64 = low
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(low))
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesHigherMemIntact(c *gc.C) {
	const high = 100 * gwacl.MB
	var value uint64 = high
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(high))
}

func (*instanceTypeSuite) TestSelectMachineTypeReturnsErrorIfNoMatch(c *gc.C) {
	var lots uint64 = 1000000000000
	_, err := selectMachineType(nil, constraints.Value{Mem: &lots})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no machine type matches constraints mem=100000*[MGT]")
}

func (*instanceTypeSuite) TestSelectMachineTypeReturnsCheapestMatch(c *gc.C) {
	var desiredCores uint64 = 50
	availableTypes := []gwacl.RoleSize{
		// Cheap, but not up to our requirements.
		{Name: "Panda", CpuCores: desiredCores / 2, Cost: 10},
		// Exactly what we need, but not the cheapest match.
		{Name: "LFA", CpuCores: desiredCores, Cost: 200},
		// Much more power than we need, but actually cheaper.
		{Name: "Lambo", CpuCores: 2 * desiredCores, Cost: 100},
		// Way out of our league.
		{Name: "Veyron", CpuCores: 10 * desiredCores, Cost: 500},
	}

	choice, err := selectMachineType(availableTypes, constraints.Value{CpuCores: &desiredCores})
	c.Assert(err, gc.IsNil)

	// Out of these options, selectMachineType picks not the first; not
	// the cheapest; not the biggest; not the last; but the cheapest type
	// of machine that meets requirements.
	c.Check(choice.Name, gc.Equals, "Lambo")
}

// fakeSimpleStreamsScheme is a fake protocol which tests can use for their
// simplestreams base URLs.
const fakeSimpleStreamsScheme = "azure-simplestreams-test"

// testRoundTripper is a fake http-like transport for injecting fake
// simplestream responses into these tests.
var testRoundTripper = jujutest.ProxyRoundTripper{}

func init() {
	// Route any request for a URL on the fakeSimpleStreamsScheme protocol
	// to testRoundTripper.
	testRoundTripper.RegisterForScheme(fakeSimpleStreamsScheme)
}

// prepareSimpleStreamsResponse sets up a fake response for our query to
// SimpleStreams.
//
// It returns a cleanup function, which you must call to reset things when
// done.
func prepareSimpleStreamsResponse(stream, location, series, release, arch, json string) func() {
	fakeURL := fakeSimpleStreamsScheme + "://"
	originalAzureURLs := baseURLs
	originalDefaultURL := imagemetadata.DefaultBaseURL
	baseURLs = []string{fakeURL}
	imagemetadata.DefaultBaseURL = ""

	originalSignedOnly := signedImageDataOnly
	signedImageDataOnly = false

	azureName := fmt.Sprintf("com.ubuntu.cloud:%s:azure", stream)
	streamSuffix := ""
	if stream != "released" {
		streamSuffix = "." + stream
	}

	// Generate an index.  It will point to an Azure index with the
	// caller's json.
	index := fmt.Sprintf(`
		{
		 "index": {
		   %q: {
		    "updated": "Thu, 08 Aug 2013 07:55:58 +0000",
		    "clouds": [
			{
			 "region": %q,
			 "endpoint": "https://management.core.windows.net/"
			}
		    ],
		    "format": "products:1.0",
		    "datatype": "image-ids",
		    "cloudname": "azure",
		    "products": [
			"com.ubuntu.cloud%s:server:%s:%s"
		    ],
		    "path": "/v1/%s.json"
		   }
		 },
		 "updated": "Thu, 08 Aug 2013 07:55:58 +0000",
		 "format": "index:1.0"
		}
		`, azureName, location, streamSuffix, release, arch, azureName)
	files := map[string]string{
		"/v1/index.json":             index,
		"/v1/" + azureName + ".json": json,
	}
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
	return func() {
		baseURLs = originalAzureURLs
		imagemetadata.DefaultBaseURL = originalDefaultURL
		signedImageDataOnly = originalSignedOnly
		testRoundTripper.Sub = nil
	}
}

func (*environSuite) TestGetEndpoint(c *gc.C) {
	c.Check(
		getEndpoint("West US"),
		gc.Equals,
		"https://management.core.windows.net/")
	c.Check(
		getEndpoint("China East"),
		gc.Equals,
		"https://management.core.chinacloudapi.cn/")
}

func (*instanceTypeSuite) TestFindMatchingImagesReturnsErrorIfNoneFound(c *gc.C) {
	emptyResponse := `
		{
		 "format": "products:1.0"
		}
		`
	cleanup := prepareSimpleStreamsResponse("released", "West US", "precise", "12.04", "amd64", emptyResponse)
	defer cleanup()

	env := makeEnviron(c)
	_, err := findMatchingImages(env, "West US", "saucy", "", []string{"amd64"})
	c.Assert(err, gc.NotNil)

	c.Check(err, gc.ErrorMatches, "no OS images found for location .*")
}

func (*instanceTypeSuite) TestFindMatchingImagesReturnsReleasedImages(c *gc.C) {
	// Based on real-world simplestreams data, pared down to a minimum:
	response := `
	{
	 "updated": "Tue, 09 Jul 2013 22:35:10 +0000",
	 "datatype": "image-ids",
	 "content_id": "com.ubuntu.cloud:released",
	 "products": {
	  "com.ubuntu.cloud:server:12.04:amd64": {
	   "release": "precise",
	   "version": "12.04",
	   "arch": "amd64",
	   "versions": {
	    "20130603": {
	     "items": {
	      "euww1i3": {
	       "virt": "Hyper-V",
	       "crsn": "West Europe",
	       "root_size": "30GB",
	       "id": "MATCHING-IMAGE"
	      }
	     },
	     "pub_name": "b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_04_2-LTS-amd64-server-20130603-en-us-30GB",
	     "publabel": "Ubuntu Server 12.04.2 LTS",
	     "label": "release"
	    }
	   }
	  }
	 },
	 "format": "products:1.0",
	 "_aliases": {
	  "crsn": {
	   "West Europe": {
	    "region": "West Europe",
	    "endpoint": "https://management.core.windows.net/"
	   }
	  }
	 }
	}
	`
	cleanup := prepareSimpleStreamsResponse("released", "West Europe", "precise", "12.04", "amd64", response)
	defer cleanup()

	env := makeEnviron(c)
	images, err := findMatchingImages(env, "West Europe", "precise", "", []string{"amd64"})
	c.Assert(err, gc.IsNil)

	c.Assert(images, gc.HasLen, 1)
	c.Check(images[0].Id, gc.Equals, "MATCHING-IMAGE")
}

func (*instanceTypeSuite) TestFindMatchingImagesReturnsDailyImages(c *gc.C) {
	// Based on real-world simplestreams data, pared down to a minimum:
	response := `
	{
	 "updated": "Tue, 09 Jul 2013 22:35:10 +0000",
	 "datatype": "image-ids",
	 "content_id": "com.ubuntu.cloud:daily:azure",
	 "products": {
	  "com.ubuntu.cloud.daily:server:12.04:amd64": {
	   "release": "precise",
	   "version": "12.04",
	   "arch": "amd64",
	   "versions": {
	    "20130603": {
	     "items": {
	      "euww1i3": {
	       "virt": "Hyper-V",
	       "crsn": "West Europe",
	       "root_size": "30GB",
	       "id": "MATCHING-IMAGE"
	      }
	     },
	     "pub_name": "b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-12_04_2-LTS-amd64-server-20130603-en-us-30GB",
	     "publabel": "Ubuntu Server 12.04.2 LTS",
	     "label": "release"
	    }
	   }
	  }
	 },
	 "format": "products:1.0",
	 "_aliases": {
	  "crsn": {
	   "West Europe": {
	    "region": "West Europe",
	    "endpoint": "https://management.core.windows.net/"
	   }
	  }
	 }
	}
	`
	cleanup := prepareSimpleStreamsResponse("daily", "West Europe", "precise", "12.04", "amd64", response)
	defer cleanup()

	env := makeEnviron(c)
	images, err := findMatchingImages(env, "West Europe", "precise", "daily", []string{"amd64"})
	c.Assert(err, gc.IsNil)

	c.Assert(images, gc.HasLen, 1)
	c.Check(images[0].Id, gc.Equals, "MATCHING-IMAGE")
}

func (*instanceTypeSuite) TestNewInstanceTypeConvertsRoleSize(c *gc.C) {
	roleSize := gwacl.RoleSize{
		Name:             "Outrageous",
		CpuCores:         128,
		Mem:              4 * gwacl.TB,
		OSDiskSpaceCloud: 48 * gwacl.TB,
		OSDiskSpaceVirt:  50 * gwacl.TB,
		MaxDataDisks:     20,
		Cost:             999999500,
	}
	vtype := "Hyper-V"
	var cpupower uint64 = 100
	expectation := instances.InstanceType{
		Id:       roleSize.Name,
		Name:     roleSize.Name,
		Arches:   []string{"amd64", "i386"},
		CpuCores: roleSize.CpuCores,
		Mem:      roleSize.Mem,
		RootDisk: roleSize.OSDiskSpaceVirt,
		Cost:     roleSize.Cost,
		VType:    &vtype,
		CpuPower: &cpupower,
	}
	c.Check(newInstanceType(roleSize), gc.DeepEquals, expectation)
}

func (*instanceTypeSuite) TestListInstanceTypesAcceptsNil(c *gc.C) {
	c.Check(listInstanceTypes(nil), gc.HasLen, 0)
}

func (*instanceTypeSuite) TestListInstanceTypesMaintainsOrder(c *gc.C) {
	roleSizes := []gwacl.RoleSize{
		{Name: "Biggish"},
		{Name: "Tiny"},
		{Name: "Huge"},
		{Name: "Miniscule"},
	}

	expectation := make([]instances.InstanceType, len(roleSizes))
	for index, roleSize := range roleSizes {
		expectation[index] = newInstanceType(roleSize)
	}

	c.Check(listInstanceTypes(roleSizes), gc.DeepEquals, expectation)
}

func (*instanceTypeSuite) TestFindInstanceSpecFailsImpossibleRequest(c *gc.C) {
	impossibleConstraint := instances.InstanceConstraint{
		Series: "precise",
		Arches: []string{"axp"},
	}

	env := makeEnviron(c)
	_, err := findInstanceSpec(env, "daily", impossibleConstraint)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no OS images found for .*")
}

// patchFetchImageMetadata temporarily replaces imagemetadata.Fetch() with a
// fake that returns the given canned answer.
// It returns a cleanup function, which you must call when done.
func patchFetchImageMetadata(cannedResponse []*imagemetadata.ImageMetadata, cannedError error) func() {
	original := fetchImageMetadata
	fetchImageMetadata = func([]simplestreams.DataSource, string, *imagemetadata.ImageConstraint, bool) ([]*imagemetadata.ImageMetadata, error) {
		return cannedResponse, cannedError
	}
	return func() { fetchImageMetadata = original }
}

func (*instanceTypeSuite) TestFindInstanceSpecFindsMatch(c *gc.C) {
	// We have one OS image.
	images := []*imagemetadata.ImageMetadata{
		{
			Id:          "image-id",
			VType:       "Hyper-V",
			Arch:        "amd64",
			RegionAlias: "West US",
			RegionName:  "West US",
			Endpoint:    "http://localhost/",
		},
	}
	cleanup := patchFetchImageMetadata(images, nil)
	defer cleanup()

	// We'll tailor our constraints to describe one particular Azure
	// instance type:
	aim := gwacl.RoleNameMap["Large"]
	constraints := instances.InstanceConstraint{
		Region: "West US",
		Series: "precise",
		Arches: []string{"amd64"},
		Constraints: constraints.Value{
			CpuCores: &aim.CpuCores,
			Mem:      &aim.Mem,
		},
	}

	// Find a matching instance type and image.
	env := makeEnviron(c)
	spec, err := findInstanceSpec(env, "released", constraints)
	c.Assert(err, gc.IsNil)

	// We got the instance type we described in our constraints, and
	// the image returned by (the fake) simplestreams.
	c.Check(spec.InstanceType.Name, gc.Equals, aim.Name)
	c.Check(spec.Image.Id, gc.Equals, "image-id")
}

func (*instanceTypeSuite) TestFindInstanceSpecSetsBaseline(c *gc.C) {
	images := []*imagemetadata.ImageMetadata{
		{
			Id:          "image-id",
			VType:       "Hyper-V",
			Arch:        "amd64",
			RegionAlias: "West US",
			RegionName:  "West US",
			Endpoint:    "http://localhost/",
		},
	}
	cleanup := patchFetchImageMetadata(images, nil)
	defer cleanup()

	// findInstanceSpec sets baseline constraints, so that it won't pick
	// ExtraSmall (which is too small for routine tasks) if you fail to
	// set sufficient hardware constraints.
	anyInstanceType := instances.InstanceConstraint{
		Region: "West US",
		Series: "precise",
		Arches: []string{"amd64"},
	}

	env := makeEnviron(c)
	spec, err := findInstanceSpec(env, "", anyInstanceType)
	c.Assert(err, gc.IsNil)

	c.Check(spec.InstanceType.Name, gc.Equals, "Small")
}
