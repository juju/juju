assert_storage() {
	local name query
	name=${1:?"name is missing"}
	query=${2:?"query is missing"}

	juju storage --format json | jq "${query}" | check "${name}"
}

life_status() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"life\"]"
}

kind_name() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"kind\"]"
}

label() {
	local app_index
	app_index=${1}

	echo ".storage | keys[$app_index]"
}

unit_attachment() {
	local name app_index unit_index
	name=${1}
	app_index=${2}
	unit_index=${3}

	echo ".storage[\"$name/$app_index\"] | .attachments | .units | keys[$unit_index]"
}

unit_state() {
	local app_name app_index unit_name unit_index
	app_name=${1}
	app_index=${2}
	unit_name=${3}
	unit_index=${4}

	echo ".storage[\"$app_name/$app_index\"] | .attachments | .units[\"$unit_name/$unit_index\"][\"life\"]"
}
