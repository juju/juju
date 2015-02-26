// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd

var RunCommand = &runCommand
var List = &list
var Reload = &reload
var Enable = &enable
var Disable = &disable
var Start = &start
var Stop = &stop

var ExtraScriptTemplate = extraScriptTemplate

func (s *Service) ServiceName() string {
	return s.serviceName()
}

func (s *Service) ServiceFilePath() string {
	return s.serviceFilePath()
}

func (s *Service) ExtraScriptPath() string {
	return s.extraScriptPath()
}

func (s *Service) Validate() error {
	return s.validate()
}

func (s *Service) Render() ([]byte, error) {
	return s.render()
}

func (s *Service) ExistsAndMatches() (bool, bool, error) {
	return s.existsAndMatches()
}

func (s *Service) Enabled() bool {
	return s.enabled()
}
