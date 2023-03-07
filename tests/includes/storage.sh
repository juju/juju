# assert_storage function will match that a given query exists in the output.
assert_storage() {
	local name query
	name=${1:?"name is missing"}
	query=${2:?"query is missing"}

	juju storage --format json | jq "${query}" | check "${name}"
}

# life_status checks for the life status for a given application storage. Uses a combination of the storage name and its unit index to query.
life_status() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"life\"]"
}

# kind_name checks for the storage kind using the combination of the storage name and its unit index to query.
kind_name() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"kind\"]"
}

# label checks for the storage label for a given application. The key's index is the application index.
label() {
	local app_index
	app_index=${1}

	echo ".storage | keys[$app_index]"
}

# used to query for a storage's attached unit using the combination of the storage application name and its storage unit index.
unit_attachment() {
	local name app_index unit_index
	name=${1}
	app_index=${2}
	unit_index=${3}

	echo ".storage[\"$name/$app_index\"] | .attachments | .units | keys[$unit_index]"
}

# unit_state queries for a storage application's attached unit life status using a combination of the storage application name and application index together with
# the storage unit name and storage unit index to filter.
unit_state() {
	local app_name app_index unit_name unit_index
	app_name=${1}
	app_index=${2}
	unit_name=${3}
	unit_index=${4}

	echo ".storage[\"$app_name/$app_index\"] | .attachments | .units[\"$unit_name/$unit_index\"][\"life\"]"
}

## checks if the given storage unit exists.
unit_exist() {
	local name
	name=${1}
	juju storage --format json | jq "any(paths; .[-1] == \"${name}\")"
}

# filesystem_status used to check for the current status of the given volume for a filesystem matched by the volume number and volume index combination e.g 0/0, 2/1, 3/1
filesystem_status() {
	local name volume_num volume_index
	volume_num=${1}
	volume_index=${2}

	if [ -z "$volume_index" ]; then
		name="$volume_num"
	else
		name="$volume_num/$volume_index"
	fi
	echo ".filesystems | .[\"$name\"] | .status"
}
