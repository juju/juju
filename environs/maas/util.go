// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	"launchpad.net/goyaml"
	cloudinit_core "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
	"net/url"
	"strings"
)

// extractSystemId extracts the 'system_id' part from an InstanceId.
// "/MAAS/api/1.0/nodes/system_id/" => "system_id"
func extractSystemId(instanceId instance.Id) string {
	trimmed := strings.TrimRight(string(instanceId), "/")
	split := strings.Split(trimmed, "/")
	return split[len(split)-1]
}

// getSystemIdValues returns a url.Values object with all the 'system_ids'
// from the given instanceIds stored under the key 'id'.  This is used
// to filter out instances when listing the nodes objects.
func getSystemIdValues(instanceIds []instance.Id) url.Values {
	values := url.Values{}
	for _, instanceId := range instanceIds {
		values.Add("id", extractSystemId(instanceId))
	}
	return values
}

// userData returns a zipped cloudinit config.
// TODO(bug 1199847): Some of this work can be shared between providers.
func userData(cfg *cloudinit.MachineConfig, scripts ...string) ([]byte, error) {
	cloudcfg := cloudinit_core.New()
	for _, script := range scripts {
		cloudcfg.AddRunCmd(script)
	}
	cloudcfg, err := cloudinit.Configure(cfg, cloudcfg)
	if err != nil {
		return nil, err
	}
	data, err := cloudcfg.Render()
	if err != nil {
		return nil, err
	}
	cdata := utils.Gzip(data)
	log.Debugf("environs/maas: maas user data; %d bytes", len(cdata))
	return cdata, nil
}

// machineInfo is the structure used to pass information between the provider
// and the agent running on a node.
// When a node is started, the provider code creates a machineInfo object
// containing information about the node being started and configures
// cloudinit to get a YAML representation of that object written on the node's
// filesystem during its first startup.  That file is then read by the juju
// agent running on the node and converted back into a machineInfo object.
type machineInfo struct {
	InstanceId string `yaml:,omitempty`
	Hostname   string `yaml:,omitempty`
}

var _MAASInstanceFilename = jujuDataDir + "/MAASmachine.txt"

// cloudinitRunCmd returns the shell command that, when run, will create the
// "machine info" file containing the instanceId and the hostname of a machine.
// That command is destined to be used by cloudinit.
func (info *machineInfo) cloudinitRunCmd() (string, error) {
	yaml, err := goyaml.Marshal(info)
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(`mkdir -p %s; echo -n %s > %s`, utils.ShQuote(jujuDataDir), utils.ShQuote(string(yaml)), utils.ShQuote(_MAASInstanceFilename))
	return script, nil
}

// load loads the "machine info" file and parse the content into the info
// object.
func (info *machineInfo) load() error {
	return utils.ReadYaml(_MAASInstanceFilename, info)
}
