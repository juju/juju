start_server() {
	local path

	path=${1}

	(
		cd "${path}" || exit 1
		daemon python3 -m http.server 8666

		# Sleep to ensure the python server is up and running correctly, as it's
		# a daemon service (&) we can't actually see if it's up easily.
		sleep 5
	)
}
