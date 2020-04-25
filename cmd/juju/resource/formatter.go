// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"fmt"
	"sort"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/resource"
)

type charmResourcesFormatter struct {
	resources []charmresource.Resource
}

func newCharmResourcesFormatter(resources []charmresource.Resource) *charmResourcesFormatter {
	// It's a lot easier to read and to digest a list of resources
	// when  they are ordered.
	sort.Sort(charmResourceList(resources))

	// Note that unlike the "juju status" code, we don't worry
	// about "compatVersion".
	crf := charmResourcesFormatter{
		resources: resources,
	}
	return &crf
}

func (crf *charmResourcesFormatter) format() []FormattedCharmResource {
	if crf.resources == nil {
		return nil
	}

	var formatted []FormattedCharmResource
	for _, res := range crf.resources {
		formatted = append(formatted, FormatCharmResource(res))
	}
	return formatted
}

// FormatCharmResource converts the resource info into a FormattedCharmResource.
func FormatCharmResource(res charmresource.Resource) FormattedCharmResource {
	return FormattedCharmResource{
		Name:        res.Name,
		Type:        res.Type.String(),
		Path:        res.Path,
		Description: res.Description,
		Revision:    res.Revision,
		Origin:      res.Origin.String(),
		Fingerprint: res.Fingerprint.String(), // ...the hex string.
		Size:        res.Size,
	}
}

// FormatAppResource converts the resource info into a FormattedAppResource.
func FormatAppResource(res resource.Resource) FormattedAppResource {
	used := !res.IsPlaceholder()
	result := FormattedAppResource{
		ID:               res.ID,
		ApplicationID:    res.ApplicationID,
		Name:             res.Name,
		Type:             res.Type.String(),
		Path:             res.Path,
		Description:      res.Description,
		Origin:           res.Origin.String(),
		Fingerprint:      res.Fingerprint.String(),
		Size:             res.Size,
		Used:             used,
		Timestamp:        res.Timestamp,
		Username:         res.Username,
		CombinedRevision: combinedRevision(res),
		CombinedOrigin:   combinedOrigin(used, res),
		UsedYesNo:        usedYesNo(used),
	}
	// Have to check since revision 0 is still a valid revision.
	if res.Revision >= 0 {
		result.Revision = fmt.Sprintf("%v", res.Revision)
	} else {
		result.Revision = "-"
	}
	return result
}

func formatApplicationResources(sr resource.ApplicationResources) (FormattedApplicationInfo, error) {
	var formatted FormattedApplicationInfo
	updates, err := sr.Updates()
	if err != nil {
		return formatted, errors.Trace(err)
	}
	formatted = FormattedApplicationInfo{
		Resources: make([]FormattedAppResource, len(sr.Resources)),
		Updates:   make([]FormattedCharmResource, len(updates)),
	}

	for i, r := range sr.Resources {
		formatted.Resources[i] = FormatAppResource(r)
	}
	for i, u := range updates {
		formatted.Updates[i] = FormatCharmResource(u)
	}
	return formatted, nil
}

// FormatApplicationDetails converts a ApplicationResources value into a formatted value
// for display on the command line.
func FormatApplicationDetails(sr resource.ApplicationResources) (FormattedApplicationDetails, error) {
	var formatted FormattedApplicationDetails
	details := detailedResources("", sr)
	updates, err := sr.Updates()
	if err != nil {
		return formatted, errors.Trace(err)
	}
	formatted = FormattedApplicationDetails{
		Resources: details,
		Updates:   make([]FormattedCharmResource, len(updates)),
	}
	for i, u := range updates {
		formatted.Updates[i] = FormatCharmResource(u)
	}
	return formatted, nil
}

// FormatDetailResource converts the arguments into a FormattedApplicationResource.
func FormatDetailResource(tag names.UnitTag, svc, unit resource.Resource, progress int64) FormattedDetailResource {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	unitNum := tag.Number()
	progressStr := ""
	fUnit := FormatAppResource(unit)
	expected := FormatAppResource(svc)
	revProgress := expected.CombinedRevision
	if progress >= 0 {
		progressStr = "100%"
		if expected.Size > 0 {
			progressStr = fmt.Sprintf("%.f%%", float64(progress)*100.0/float64(expected.Size))
		}
		if fUnit.CombinedRevision != expected.CombinedRevision {
			revProgress = fmt.Sprintf("%s (fetching: %s)", expected.CombinedRevision, progressStr)
		}
	}
	return FormattedDetailResource{
		UnitID:      tag.Id(),
		UnitNumber:  unitNum,
		Unit:        fUnit,
		Expected:    expected,
		Progress:    progress,
		RevProgress: revProgress,
	}
}

func combinedRevision(r resource.Resource) string {
	switch r.Origin {
	case charmresource.OriginStore:
		// Have to check since 0+ is a valid revision number
		if r.Revision >= 0 {
			return fmt.Sprintf("%d", r.Revision)
		}
	case charmresource.OriginUpload:
		if !r.Timestamp.IsZero() {
			return r.Timestamp.Format("2006-02-01T15:04")
		}
	}
	return "-"
}

func combinedOrigin(used bool, r resource.Resource) string {
	if r.Origin == charmresource.OriginUpload && used && r.Username != "" {
		return r.Username
	}
	if r.Origin == charmresource.OriginStore {
		return "charmstore"
	}
	return r.Origin.String()
}

func usedYesNo(used bool) string {
	if used {
		return "yes"
	}
	return "no"
}

// detailedResources shows the version of each resource on each unit, with the
// corresponding version of the resource that exists in the controller. if unit
// is non-empty, only units matching that unitID will be returned.
func detailedResources(unit string, sr resource.ApplicationResources) []FormattedDetailResource {
	var formatted []FormattedDetailResource
	for _, ur := range sr.UnitResources {
		if unit == "" || unit == ur.Tag.Id() {
			units := resourceMap(ur.Resources)
			for _, svc := range sr.Resources {
				progress, ok := ur.DownloadProgress[svc.Name]
				if !ok {
					progress = -1
				}
				f := FormatDetailResource(ur.Tag, svc, units[svc.Name], progress)
				formatted = append(formatted, f)
			}
			if unit != "" {
				break
			}
		}
	}
	return formatted
}

func resourceMap(resources []resource.Resource) map[string]resource.Resource {
	m := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		m[res.Name] = res
	}
	return m
}

// charmResourceList is a convenience type enabling to sort
// a collection of charmresource.Resource by Name.
type charmResourceList []charmresource.Resource

// Len implements sort.Interface
func (m charmResourceList) Len() int {
	return len(m)
}

// Less implements sort.Interface and sorts resources by Name.
func (m charmResourceList) Less(i, j int) bool {
	return m[i].Name < m[j].Name
}

// Swap implements sort.Interface
func (m charmResourceList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

// resourceList is a convenience type enabling to sort
// a collection of resource.Resource by Name.
type resourceList []resource.Resource

// Len implements sort.Interface
func (m resourceList) Len() int {
	return len(m)
}

// Less implements sort.Interface and sorts resources by Name.
func (m resourceList) Less(i, j int) bool {
	return m[i].Name < m[j].Name
}

// Swap implements sort.Interface
func (m resourceList) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}
