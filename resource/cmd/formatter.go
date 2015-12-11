// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/juju/resource"
)

type infoListFormatter struct {
	infos []resource.Info
}

func newInfoListFormatter(infos []resource.Info) *infoListFormatter {
	// Note that unlike the "juju status" code, we don't worry
	// about "compatVersion".
	lf := infoListFormatter{
		infos: infos,
	}
	return &lf
}

func (lf *infoListFormatter) format() []FormattedInfo {
	if lf.infos == nil {
		return nil
	}

	var formatted []FormattedInfo
	for _, info := range lf.infos {
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
		Fingerprint: info.Fingerprint,
	}
}
