# Checks whether the cwd is used for the juju local deploy.
run_deploy_local_charm_revision() {
	echo

	file="${TEST_DIR}/local-charm-deploy-git.log"
	ensure "local-charm-deploy" "${file}"

	TMP=$(mktemp -d -t ci-XXXXXXXXXX)

	cd "${TMP}" || exit 1
	git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
	cd "${TMP}/ntp" || exit 1
	SHA_OF_NTP=\"$(git describe --dirty --always)\"

	OUTPUT=$(juju deploy . 2>&1)

	wait_for "ntp" ".applications | keys[0]"
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')

	# If a error happens it means it could not use the git sha of the CWD.
	check_not_contains "${OUTPUT}" "exit status 128"

	if [ "${SHA_OF_NTP}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ntp SHA"
		exit 1
	fi

	destroy_model "local-charm-deploy"
}

# Checks the cwd has no vcs of any kind.
run_deploy_local_charm_revision_no_vcs() {
	echo

	file="${TEST_DIR}/local-charm-deploy-no-vcs.log"
	ensure "local-charm-deploy-no-vcs" "${file}"

	TMP=$(mktemp -d -t ci-XXXXXXXXXX)

	cd "${TMP}" || exit 1
	git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
	cd "${TMP}/ntp" || exit 1
	rm -rf .git
	# make sure that no version file exists.
	rm version

	OUTPUT=$(juju deploy --debug . 2>&1)

	check_contains "${OUTPUT}" "charm is not versioned"

	destroy_model "local-charm-deploy-no-vcs"
}

# Checks the cwd has no vcs but a version file.
run_deploy_local_charm_revision_no_vcs_but_version_file() {
	echo

	file="${TEST_DIR}/local-charm-deploy-version-file.log"
	ensure "local-charm-deploy-version-file" "${file}"

	TMP=$(mktemp -d -t ci-XXXXXXXXXX)

	cd "${TMP}" || exit 1
	git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
	cd "${TMP}/ntp" || exit 1
	rm -rf .git
	touch version
	echo 123 >version
	VERSION_OUTPUT=\""$(cat version)"\"

	CURRENT_DIRECTORY=$(pwd)

	# this is done relative because we expect that the output will be absolute in the end.
	OUTPUT=$(juju deploy --debug . 2>&1)

	wait_for "ntp" ".applications | keys[0]"
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')

	if [ "${VERSION_OUTPUT}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ntp SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${VERSION_OUTPUT}"
		exit 1
	fi

	# we expect the debug output to be absolute and not relative.
	check_contains "${OUTPUT}" "${CURRENT_DIRECTORY}"

	destroy_model "local-charm-deploy-version-file"
}

# Checks whether the cwd is used for the juju local deploy.
run_deploy_local_charm_revision_relative_path() {
	echo

	file="${TEST_DIR}/local-charm-deploy-relative-path.log"

	ensure "relative-path" "${file}"

	TMP=$(mktemp -d -t ci-XXXXXXXXXX)

	cd "${TMP}" || exit 1
	create_local_git_folder
	SHA_OF_TMP=\"$(git describe --dirty --always)\"
	# create ${TMP}/ntp git folder
	git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp

	# state: ${TMP} is wrong git ${TMP}/ntp is correct git
	juju deploy ./ntp 2>&1

	cd "${TMP}/ntp" || exit 1
	SHA_OF_NTP=\"$(git describe --dirty --always)\"

	wait_for "ntp" ".applications | keys[0]"

	# We still expect the SHA to be the one from the place we deploy and not the CWD, which in this case has no SHA
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')

	if [ "${SHA_OF_TMP}" = "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha should not equal the tmp SHA. Current sha: ${CURRENT_CHARM_SHA}"
		exit 1
	fi

	if [ "${SHA_OF_NTP}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ntp SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${SHA_OF_NTP}"
		exit 1
	fi

	destroy_model "relative-path"
}

# CWD with git, deploy charm with git, but -> check that git describe is correct.
run_deploy_local_charm_revision_invalid_git() {
	echo

	file="${TEST_DIR}/local-charm-deploy-wrong-git.log"
	ensure "local-charm-deploy-wrong-git" "${file}"

	TMP_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
	TMP=$(mktemp -d -t ci-XXXXXXXXXX)

	cd "${TMP_CHARM_GIT}" || exit 1
	git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp

	cd "${TMP_CHARM_GIT}/ntp" || exit 1
	WANTED_CHARM_SHA=\"$(git describe --dirty --always)\"

	# We cd into a folder without git and deploy from the folder without git.
	cd "${TMP}" || exit 1

	create_local_git_folder

	juju deploy "${TMP_CHARM_GIT}"/ntp ntp

	wait_for "ntp" ".applications | keys[0]"
	# We still expect the SHA to be the one from the place we deploy and not the CWD, which in this case has no SHA.
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')
	if [ "${WANTED_CHARM_SHA}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ntp SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${WANTED_CHARM_SHA}"
		exit 1
	fi

	destroy_model "local-charm-deploy-wrong-git"
}

create_local_git_folder() {
	git init .
	touch rand_file
	git add rand_file
	git commit -am "rand_file"
}

test_local_charms() {
	if [ "$(skip 'test_local_charms')" ]; then
		echo "==> TEST SKIPPED: deploy local charm tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_local_charm_revision"
		run "run_deploy_local_charm_revision_no_vcs"
		run "run_deploy_local_charm_revision_no_vcs_but_version_file"
		run "run_deploy_local_charm_revision_relative_path"
		run "run_deploy_local_charm_revision_invalid_git"
	)
}
