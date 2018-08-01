package jujusvg // import "gopkg.in/juju/jujusvg.v3"

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
)

// NewFromBundle returns a new Canvas that can be used
// to generate a graphical representation of the given bundle
// data. The iconURL function is used to generate a URL
// that refers to an SVG for the supplied charm URL.
// If fetcher is non-nil, it will be used to fetch icon
// contents for any icons embedded within the charm,
// allowing the generated bundle to be self-contained. If fetcher
// is nil, a default fetcher which refers to icons by their
// URLs as svg <image> tags will be used.
func NewFromBundle(b *charm.BundleData, iconURL func(*charm.URL) string, fetcher IconFetcher) (*Canvas, error) {
	if fetcher == nil {
		fetcher = &LinkFetcher{
			IconURL: iconURL,
		}
	}
	iconMap, err := fetcher.FetchIcons(b)
	if err != nil {
		return nil, err
	}

	var canvas Canvas

	// Verify the bundle to make sure that all the invariants
	// that we depend on below actually hold true.
	if err := b.Verify(nil, nil, nil); err != nil {
		return nil, errgo.Notef(err, "cannot verify bundle")
	}
	// Go through all applications in alphabetical order so that
	// we get consistent results.
	applicationNames := make([]string, 0, len(b.Applications))
	for name := range b.Applications {
		applicationNames = append(applicationNames, name)
	}
	sort.Strings(applicationNames)
	applications := make(map[string]*application)
	applicationsNeedingPlacement := make(map[string]bool)
	for _, name := range applicationNames {
		applicationData := b.Applications[name]
		x, xerr := strconv.ParseFloat(applicationData.Annotations["gui-x"], 64)
		y, yerr := strconv.ParseFloat(applicationData.Annotations["gui-y"], 64)
		if xerr != nil || yerr != nil {
			if applicationData.Annotations["gui-x"] == "" && applicationData.Annotations["gui-y"] == "" {
				applicationsNeedingPlacement[name] = true
				x = 0
				y = 0
			} else {
				return nil, errgo.Newf("application %q does not have a valid position", name)
			}
		}
		charmID, err := charm.ParseURL(applicationData.Charm)
		if err != nil {
			// cannot actually happen, as we've verified it.
			return nil, errgo.Notef(err, "cannot parse charm %q", applicationData.Charm)
		}
		icon := iconMap[charmID.Path()]
		svc := &application{
			name:      name,
			charmPath: charmID.Path(),
			point:     image.Point{int(x), int(y)},
			iconUrl:   iconURL(charmID),
			iconSrc:   icon,
		}
		applications[name] = svc
	}
	padding := image.Point{int(math.Floor(applicationBlockSize * 1.5)), int(math.Floor(applicationBlockSize * 0.5))}
	for name := range applicationsNeedingPlacement {
		vertices := []image.Point{}
		for n, svc := range applications {
			if !applicationsNeedingPlacement[n] {
				vertices = append(vertices, svc.point)
			}
		}
		applications[name].point = getPointOutside(vertices, padding)
		applicationsNeedingPlacement[name] = false
	}
	for _, name := range applicationNames {
		canvas.addApplication(applications[name])
	}
	for _, relation := range b.Relations {
		canvas.addRelation(&applicationRelation{
			name:         fmt.Sprintf("%s %s", relation[0], relation[1]),
			applicationA: applications[strings.Split(relation[0], ":")[0]],
			applicationB: applications[strings.Split(relation[1], ":")[0]],
		})
	}
	return &canvas, nil
}
