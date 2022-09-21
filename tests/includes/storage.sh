# this function will match that a given query exists in the output.
assert_storage() {
	local name query
	name=${1:?"name is missing"}
	query=${2:?"query is missing"}

	juju storage --format json | jq "${query}" | check "${name}"
}

# used to check for the life status for a given application storage. Uses a combination of the storage name and its unit index to query.
life_status() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"life\"]"
}

# used to check for the storage kind using the combination of the storage name and its unit index to query.
kind_name() {
	local name unit_index
	name=${1}
	unit_index=${2}

	echo ".storage[\"$name/$unit_index\"][\"kind\"]"
}

# used to check for the storage label for a given application. The key's index is the application index.
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

# used to query for a storage application's attached unit life status using a combination of the storage application name and applicaiton index together with
# the storage unit name and storage unit index to filter.
unit_state() {
	local app_name app_index unit_name unit_index
	app_name=${1}
	app_index=${2}
	unit_name=${3}
	unit_index=${4}

	echo ".storage[\"$app_name/$app_index\"] | .attachments | .units[\"$unit_name/$unit_index\"][\"life\"]"
}

# like wait_for but for storage formats. Used to wait for a certain condition in charm storage.
wait_for_storage(){
  local name query timeout

  	name=${1}
  	query=${2}
  	timeout=${3:-600} # default timeout: 600s = 10m

  	attempt=0
  	start_time="$(date -u +%s)"
  	# shellcheck disable=SC2046,SC2143
  	until [[ "$(juju storage --format=json 2>/dev/null | jq -S "${query}" | grep "${name}")" ]]; do
  		echo "[+] (attempt ${attempt}) polling status for" "${query} => ${name}"
  		juju storage  2>&1 | sed 's/^/    | /g'
  		sleep "${SHORT_TIMEOUT}"

  		elapsed=$(date -u +%s)-$start_time
  		if [[ ${elapsed} -ge ${timeout} ]]; then
  			echo "[-] $(red 'timed out waiting for')" "$(red "${name}")"
  			exit 1
  		fi

  		attempt=$((attempt + 1))
  	done

  	if [[ ${attempt} -gt 0 ]]; then
  		echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
  		juju storage 2>&1 | sed 's/^/    | /g'
  		# Although juju reports as an idle condition, some charms require a
  		# breathe period to ensure things have actually settled.
  		sleep "${SHORT_TIMEOUT}"
  	fi
}

# used to check for the current status of the given volume for a filesystem matched by the volume number and volume index combination e.g 0/0, 2/1, 3/1
filesystem_status(){
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