// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/juju/resource"
)

type infoListFormatter struct {
	infoList []resource.Info
}

func newInfoListFormatter(infoList []resource.Info) *infoListFormatter {
	// Note that unlike the "juju status" code, we don't worry
	// about "compatVersion".
	lf := infoListFormatter{
		infoList: infoList,
	}
	return &lf
}

func (lf *infoListFormatter) format() []FormattedInfo {
	if lf.infoList == nil {
		return nil
	}

	var formatted []FormattedInfo
	for _, info := range lf.infoList {
		formatted = append(formatted, FormatInfo(info))
	}
	return formatted
}

// FormatInfo converts the resource info into a FormattedInfo.
func FormatInfo(info resource.Info) FormattedInfo {
	return FormattedInfo{
		Name:        info.Name,
		Type:        info.Type.String(),
		Path:        info.Path,
		Comment:     info.Comment,
		Revision:    info.Revision,
		Origin:      info.Origin.String(),
		Fingerprint: info.Fingerprint.String(),
	}
}
