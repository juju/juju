// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"
	"strconv"
	"strings"

	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
	"gopkg.in/juju/names.v2"
)

type charmResourcesFormatter struct {
	resources []charmresource.Resource
}

func newCharmResourcesFormatter(resources []charmresource.Resource) *charmResourcesFormatter {
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

// FormatSvcResource converts the resource info into a FormattedServiceResource.
func FormatSvcResource(res resource.Resource) FormattedSvcResource {
	used := !res.IsPlaceholder()
	return FormattedSvcResource{
		ID:               res.ID,
		ApplicationID:    res.ApplicationID,
		Name:             res.Name,
		Type:             res.Type.String(),
		Path:             res.Path,
		Description:      res.Description,
		Revision:         res.Revision,
		Origin:           res.Origin.String(),
		Fingerprint:      res.Fingerprint.String(),
		Size:             res.Size,
		Used:             used,
		Timestamp:        res.Timestamp,
		Username:         res.Username,
		combinedRevision: combinedRevision(res),
		combinedOrigin:   combinedOrigin(used, res),
		usedYesNo:        usedYesNo(used),
	}
}

func formatServiceResources(sr resource.ServiceResources) (FormattedServiceInfo, error) {
	var formatted FormattedServiceInfo
	updates, err := sr.Updates()
	if err != nil {
		return formatted, errors.Trace(err)
	}
	formatted = FormattedServiceInfo{
		Resources: make([]FormattedSvcResource, len(sr.Resources)),
		Updates:   make([]FormattedCharmResource, len(updates)),
	}

	for i, r := range sr.Resources {
		formatted.Resources[i] = FormatSvcResource(r)
	}
	for i, u := range updates {
		formatted.Updates[i] = FormatCharmResource(u)
	}
	return formatted, nil
}

// FormatServiceDetails converts a ServiceResources value into a formatted value
// for display on the command line.
func FormatServiceDetails(sr resource.ServiceResources) (FormattedServiceDetails, error) {
	var formatted FormattedServiceDetails
	details, err := detailedResources("", sr)
	if err != nil {
		return formatted, errors.Trace(err)
	}
	updates, err := sr.Updates()
	if err != nil {
		return formatted, errors.Trace(err)
	}
	formatted = FormattedServiceDetails{
		Resources: details,
		Updates:   make([]FormattedCharmResource, len(updates)),
	}
	for i, u := range updates {
		formatted.Updates[i] = FormatCharmResource(u)
	}
	return formatted, nil
}

// FormatDetailResource converts the arguments into a FormattedServiceResource.
func FormatDetailResource(tag names.UnitTag, svc, unit resource.Resource, progress int64) (FormattedDetailResource, error) {
	// note that the unit resource can be a zero value here, to indicate that
	// the unit has not downloaded that resource yet.

	unitNum, err := unitNum(tag)
	if err != nil {
		return FormattedDetailResource{}, errors.Trace(err)
	}
	progressStr := ""
	fUnit := FormatSvcResource(unit)
	expected := FormatSvcResource(svc)
	revProgress := expected.combinedRevision
	if progress >= 0 {
		progressStr = "100%"
		if expected.Size > 0 {
			progressStr = fmt.Sprintf("%.f%%", float64(progress)*100.0/float64(expected.Size))
		}
		if fUnit.combinedRevision != expected.combinedRevision {
			revProgress = fmt.Sprintf("%s (fetching: %s)", expected.combinedRevision, progressStr)
		}
	}
	return FormattedDetailResource{
		UnitID:      tag.Id(),
		unitNumber:  unitNum,
		Unit:        fUnit,
		Expected:    expected,
		Progress:    progress,
		progress:    progressStr,
		revProgress: revProgress,
	}, nil
}

func combinedRevision(r resource.Resource) string {
	switch r.Origin {
	case charmresource.OriginStore:
		return fmt.Sprintf("%d", r.Revision)
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

func unitNum(unit names.UnitTag) (int, error) {
	vals := strings.SplitN(unit.Id(), "/", 2)
	if len(vals) != 2 {
		return 0, errors.Errorf("%q is not a valid unit ID", unit.Id())
	}
	num, err := strconv.Atoi(vals[1])
	if err != nil {
		return 0, errors.Annotatef(err, "%q is not a valid unit ID", unit.Id())
	}
	return num, nil
}

// detailedResources shows the version of each resource on each unit, with the
// corresponding version of the resource that exists in the controller. if unit
// is non-empty, only units matching that unitID will be returned.
func detailedResources(unit string, sr resource.ServiceResources) ([]FormattedDetailResource, error) {
	var formatted []FormattedDetailResource
	for _, ur := range sr.UnitResources {
		if unit == "" || unit == ur.Tag.Id() {
			units := resourceMap(ur.Resources)
			for _, svc := range sr.Resources {
				progress, ok := ur.DownloadProgress[svc.Name]
				if !ok {
					progress = -1
				}
				f, err := FormatDetailResource(ur.Tag, svc, units[svc.Name], progress)
				if err != nil {
					return nil, errors.Trace(err)
				}
				formatted = append(formatted, f)
			}
			if unit != "" {
				break
			}
		}
	}
	return formatted, nil
}

func resourceMap(resources []resource.Resource) map[string]resource.Resource {
	m := make(map[string]resource.Resource, len(resources))
	for _, res := range resources {
		m[res.Name] = res
	}
	return m
}
