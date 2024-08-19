// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/kr/pretty"
)

func main() {
	if len(os.Args) <= 2 {
		panic("must supply two Juju versions as command line args")
	}

	facadeCompatibilityInfo := compareFacadeVersions(os.Args[1], os.Args[2])
	printFacadeCompatibility(facadeCompatibilityInfo)
}

func printFacadeCompatibility(info sharedFacadeVersions) {
	fmt.Println("Compatible facades:")
	iterateAlphabetical(info.compatibleVersions, func(facadeName string, versions set.Ints) {
		fmt.Printf(" - %s (common versions: %s)\n", facadeName, printSetAsList(versions))
	})
	fmt.Println()

	fmt.Println("Incompatible facades:")
	iterateAlphabetical(info.incompatibleFacades, func(facadeName string, versionInfo map[string]facadeInfo) {
		fmt.Printf(" - %s\n", facadeName)
		iterateAlphabetical(versionInfo, func(v string, vInfo facadeInfo) {
			fmt.Printf("   - %s: %s\n", v, vInfo.String())
		})
	})
}

// sharedFacadeVersions contains information about which facade versions are
// compatible or incompatible between two different versions of Juju.
type sharedFacadeVersions struct {
	// compatibleVersions maps each facade name to a list of commonly supported
	// versions.
	compatibleVersions map[string]set.Ints
	// incompatibleFacades maps facadeName -> version -> info about what
	// versions are supported.
	incompatibleFacades map[string]map[string]facadeInfo
}

// facadeInfo contains information about a facade in a given version of Juju -
// either the supported versions or the fact it doesn't exist in that version.
type facadeInfo interface {
	String() string
}

type facadeIsSupported struct {
	supportedVersions set.Ints
}

func (f facadeIsSupported) String() string {
	return fmt.Sprintf("supported versions: %s", printSetAsList(f.supportedVersions))
}

type facadeDoesntExist struct{}

func (facadeDoesntExist) String() string {
	return "doesn't exist"
}

func compareFacadeVersions(v1, v2 string) sharedFacadeVersions {
	comparison := sharedFacadeVersions{
		compatibleVersions:  map[string]set.Ints{},
		incompatibleFacades: map[string]map[string]facadeInfo{},
	}

	supportedFacadeVersions1 := getSupportedFacadeVersions(v1)
	supportedFacadeVersions2 := getSupportedFacadeVersions(v2)

	for facadeName, supportedVersions1 := range supportedFacadeVersions1 {
		supportedVersions2, ok := supportedFacadeVersions2[facadeName]
		if !ok {
			comparison.incompatibleFacades[facadeName] = map[string]facadeInfo{
				v1: facadeIsSupported{supportedVersions1},
				v2: facadeDoesntExist{},
			}
			continue
		}

		intersection := supportedVersions1.Intersection(supportedVersions2)
		if intersection.Size() == 0 {
			comparison.incompatibleFacades[facadeName] = map[string]facadeInfo{
				v1: facadeIsSupported{supportedVersions1},
				v2: facadeIsSupported{supportedVersions2},
			}
		} else {
			comparison.compatibleVersions[facadeName] = intersection
		}
	}

	// Check facades supported by v2 but not v1
	for facadeName, supportedVersions2 := range supportedFacadeVersions2 {
		_, ok := supportedFacadeVersions1[facadeName]
		if !ok {
			comparison.incompatibleFacades[facadeName] = map[string]facadeInfo{
				v1: facadeDoesntExist{},
				v2: facadeIsSupported{supportedVersions2},
			}
		}
		// If the facade exists in both versions, it should have been caught in
		// the loop above.
	}

	return comparison
}

// supportedFacadeVersions maps a facade name to a set of supported versions.
type supportedFacadeVersions map[string]set.Ints

func getSupportedFacadeVersions(ver string) supportedFacadeVersions {
	resp, err := http.Get(fmt.Sprintf("https://raw.githubusercontent.com/juju/juju/v%s/api/facadeversions.go", ver))
	check(err)
	if resp.StatusCode == http.StatusNotFound {
		resp, err = http.Get(fmt.Sprintf("https://raw.githubusercontent.com/juju/juju/juju-%s/api/facadeversions.go", ver))
		check(err)
		if resp.StatusCode == http.StatusNotFound {
			panic(fmt.Sprintf("could not find version %q on GitHub", ver))
		}
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		panic(fmt.Sprintf("GET returned %d: %s", resp.StatusCode, body))
	}

	defer resp.Body.Close()
	parsed, err := parser.ParseFile(token.NewFileSet(), "", resp.Body, 0)
	check(err)

	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl: // skip
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ImportSpec: // skip
				case *ast.ValueSpec:
					if s.Names[0].Name == "facadeVersions" {
						return getSupportedFacadeVersionsFromValueSpec(s)
					}
				default:
					panic(fmt.Sprintf("unexpected spec type %T", s))
				}
			}
		default:
			panic(fmt.Sprintf("unexpected decl type %T\n%s", d, pretty.Sprint(d)))
		}
	}

	return nil
}

func getSupportedFacadeVersionsFromValueSpec(s *ast.ValueSpec) supportedFacadeVersions {
	allSupportedVersions := supportedFacadeVersions{}

	entries := s.Values[0].(*ast.CompositeLit).Elts
	for _, elt := range entries {
		kv := elt.(*ast.KeyValueExpr)

		// Parse facade name
		facadeName := strings.Trim(kv.Key.(*ast.BasicLit).Value, `"`)

		// Parse supported facade versions
		supportedVersions := set.NewInts()
		switch v := kv.Value.(type) {
		case *ast.BasicLit: // older-style
			maxSupportedVersion, err := strconv.Atoi(v.Value)
			check(err)
			for i := 0; i <= maxSupportedVersion; i++ {
				supportedVersions.Add(i)
			}

		case *ast.CompositeLit: // newer-style
			for _, elt := range v.Elts {
				ver, err := strconv.Atoi(elt.(*ast.BasicLit).Value)
				check(err)
				supportedVersions.Add(ver)
			}

		default:
			panic(fmt.Sprintf("unexpected expr type %T", v))
		}

		allSupportedVersions[facadeName] = supportedVersions
	}
	return allSupportedVersions
}

// UTILITY FUNCTIONS

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func iterateAlphabetical[T any](m map[string]T, f func(k string, v T)) {
	// Sort keys
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Iterate in order
	for _, k := range keys {
		f(k, m[k])
	}
}

func printSetAsList(s set.Ints) string {
	var strs []string
	for _, v := range s.SortedValues() {
		strs = append(strs, fmt.Sprint(v))
	}
	return strings.Join(strs, ", ")
}
