$token = config-get token
$relations = relation-ids sink
foreach ($relation_id in $relations.split()) {
	relation-set -r $relation_id token="$token"
}
juju-log.exe "Token is $token"
status-set.exe "active" "Token is $token"
