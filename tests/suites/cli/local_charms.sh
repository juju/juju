# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

# Checks whether the cwd is used for the juju local deploy.
run_deploy_local_charm_revision() {
	echo

	file="${TEST_DIR}/local-charm-deploy-git.log"
	ensure "local-charm-deploy" "${file}"

	# Get a basic charm
	TMP=$(mktemp -d -t ci-XXXXXXXXXX)
	cp -r "$CURRENT_DIR/../testcharms/charms/ubuntu-plus" "${TMP}"
	cd "${TMP}/ubuntu-plus" || exit 1

	# Initialise a git repo to check the commit SHA is used as the charm version.
	git init
	git add . && git commit -m "commit everything"
	SHA_OF_UBUNTU_PLUS=\"$(git describe --dirty --always)\"

	# Deploy from directory.
	juju deploy .

	wait_for "ubuntu-plus" ".applications | keys[0]"
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications."ubuntu-plus"."charm-version"')

	if [ "${SHA_OF_UBUNTU_PLUS}" != "${CURRENT_CHARM_SHA}" ]; then
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

	# Get a basic charm, there should be no VCS info in this file.
	TMP=$(mktemp -d -t ci-XXXXXXXXXX)
	cp -r "$CURRENT_DIR/../testcharms/charms/ubuntu-plus" "${TMP}"
	cd "${TMP}/ubuntu-plus" || exit 1

	# Remove version file
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

	# Get a basic charm, there should be no VCS info in this file.
	TMP=$(mktemp -d -t ci-XXXXXXXXXX)
	cp -r "$CURRENT_DIR/../testcharms/charms/ubuntu-plus" "${TMP}"
	cd "${TMP}/ubuntu-plus" || exit 1

	VERSION_OUTPUT=\""$(cat version | sed 's/.* //')"\"
	CURRENT_DIRECTORY=$(pwd)

	# this is done relative because we expect that the output will be absolute in the end.
	OUTPUT=$(juju deploy --debug . 2>&1)

	wait_for "ubuntu-plus" ".applications | keys[0]"
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications."ubuntu-plus"."charm-version"')

	if [ "${VERSION_OUTPUT}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ubuntu-plus SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${VERSION_OUTPUT}"
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

	# Get a basic charm
	TMP=$(mktemp -d -t ci-XXXXXXXXXX)
	cp -r "$CURRENT_DIR/../testcharms/charms/ubuntu-plus" "${TMP}"
	cd "${TMP}/ubuntu-plus" || exit 1

	# Initialise a git repo and commit everything so that commit SHA is used as the charm version.
	git init
	git add . && git commit -m "commit everything"
	SHA_OF_UBUNTU_PLUS=\"$(git describe --dirty --always)\"

	# Create git directory outside the charm directory
	cd ..
	create_local_git_folder
	SHA_OF_TMP=\"$(git describe --dirty --always)\"

	# state: there is a git repo in the current directory, $TMP, but the correct
	# git repo is in $TMP/ubuntu-plus.
	juju deploy ./ubuntu-plus 2>&1

	cd "${TMP}/ubuntu-plus" || exit 1
	SHA_OF_UBUNTU_PLUS=\"$(git describe --dirty --always)\"

	wait_for "ubuntu-plus" ".applications | keys[0]"

	# We still expect the SHA to be the one from the place we deploy and not the CWD, which in this case has no SHA
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications."ubuntu-plus"."charm-version"')

	if [ "${SHA_OF_TMP}" = "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha should not equal the tmp SHA. Current sha: ${CURRENT_CHARM_SHA}"
		exit 1
	fi

	if [ "${SHA_OF_UBUNTU_PLUS}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ntp SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${SHA_OF_UBUNTU_PLUS}"
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

	# Get a basic charm
	cp -r "$CURRENT_DIR/../testcharms/charms/ubuntu-plus" "${TMP_CHARM_GIT}"
	cd "${TMP_CHARM_GIT}/ubuntu-plus" || exit 1

	# Initialise a git repo and commit everything so that commit SHA is used as the charm version.
	git init
	git add . && git commit -m "commit everything"
	SHA_OF_UBUNTU_PLUS=\"$(git describe --dirty --always)\"

	WANTED_CHARM_SHA=\"$(git describe --dirty --always)\"

	# We cd into a folder without git, add an unrelated repo there.
	cd "${TMP}" || exit 1
	create_local_git_folder
	# Deploy from the correct repo
	juju deploy "${TMP_CHARM_GIT}"/ubuntu-plus

	wait_for "ubuntu-plus" ".applications | keys[0]"
	# We still expect the SHA to be the one from the place we deploy and not the CWD, which in this case has no SHA.
	CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications."ubuntu-plus"."charm-version"')
	if [ "${WANTED_CHARM_SHA}" != "${CURRENT_CHARM_SHA}" ]; then
		echo "The expected sha does not equal the ubuntu-plus SHA. Current sha: ${CURRENT_CHARM_SHA} expected sha: ${WANTED_CHARM_SHA}"
		exit 1
	fi

	destroy_model "local-charm-deploy-wrong-git"
}

create_local_git_folder() {
	git init .
	if [ -z "$(git config --global user.email)" ]; then
		git config --global user.email "john@doe.com"
		git config --global user.name "John Doe"
	fi
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
