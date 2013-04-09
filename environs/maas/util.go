package maas

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	cloudinit_core "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	"net/url"
	"strings"
)

// extractSystemId extracts the 'system_id' part from an InstanceId.
// "/MAAS/api/1.0/nodes/system_id/" => "system_id"
func extractSystemId(instanceId state.InstanceId) string {
	trimmed := strings.TrimRight(string(instanceId), "/")
	split := strings.Split(trimmed, "/")
	return split[len(split)-1]
}

// getSystemIdValues returns a url.Values object with all the 'system_ids'
// from the given instanceIds stored under the key 'id'.  This is used
// to filter out instances when listing the nodes objects.
func getSystemIdValues(instanceIds []state.InstanceId) url.Values {
	values := url.Values{}
	for _, instanceId := range instanceIds {
		values.Add("id", extractSystemId(instanceId))
	}
	return values
}

// userData returns a zipped cloudinit config.
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
	cdata := trivial.Gzip(data)
	log.Debugf("environs/maas: maas user data; %d bytes", len(cdata))
	return cdata, nil
}

type machineInfo struct {
	InstanceId string
	Hostname   string
}

var _MAASInstanceFilename = jujuDataDir + "/MAASmachine.txt"

// serializeYAML serializes the info into YAML format.
func (info *machineInfo) serializeYAML() ([]byte, error) {
	return goyaml.Marshal(info)
}

// cloudinitRunCmd returns the shell command that, when run, will create the
// "machine info" file containing the instanceId and the hostname of a machine.
// That command is destined to be used by cloudinit.
func (info *machineInfo) cloudinitRunCmd() (string, error) {
	yaml, err := info.serializeYAML()
	if err != nil {
		return "", err
	}
	script := fmt.Sprintf(`mkdir -p %s; echo -n %s > %s`, trivial.ShQuote(jujuDataDir), trivial.ShQuote(string(yaml)), trivial.ShQuote(_MAASInstanceFilename))
	return script, nil
}

// load loads the "machine info" file and parse the content into the info
// object.
func (info *machineInfo) load() error {
	content, err := ioutil.ReadFile(_MAASInstanceFilename)
	if err != nil {
		return err
	}
	return goyaml.Unmarshal(content, info)
}
