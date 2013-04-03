package maas

import (
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
	cloudcfg, err := cloudinit.New(cfg)
	for _, script := range scripts {
		cloudcfg.AddRunCmd(script)
	}
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
