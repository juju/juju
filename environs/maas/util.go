package maas

import (
	"launchpad.net/juju-core/state"
	"net/url"
	"strings"
)

// extractSystemId extracts the 'system_id' part from an InstanceId.
// "/MAAS/api/1.0/nodes/system_id/" => "system_id"
func extractSystemId(instanceId state.InstanceId) string {
	trimed := strings.Trim(string(instanceId), "/")
	split := strings.Split(trimed, "/")
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
