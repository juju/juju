# assert_storage function will match that a given query exists in the output.
assert_storage() {
	local name query
	name=${1:?"name is missing"}
	query=${2:?"query is missing"}

	juju storage --format json | yq "${query}" | check "${name}"
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
	juju storage --format json | yq "[.. | select(kind == \"map\") | has(\"${name}\")] | any"
}

# filesystem_status emits a yq query for the status of the filesystem backing
# the given storage instance id (e.g. data/0). The filesystem is matched via
# its storage linkage rather than its id, as filesystem ids differ between
# juju versions (machine-scoped "0/0" before 4.0, global sequence numbers
# from 4.0 onwards).
filesystem_status() {
	local storage_id
	storage_id=${1}

	echo ".filesystems | to_entries | map(select(.value.storage == \"${storage_id}\")) | .[0].value.status"
}
