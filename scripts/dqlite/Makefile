SHELL = bash
.ONESHELL:

PROJECT_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BUILD_DIR ?= $(PROJECT_DIR)/_build

SETUP_SCRIPT ?= ${PROJECT_DIR}/scripts/dqlite/setup-ubuntu.sh
BUILD_SCRIPT ?= ${PROJECT_DIR}/scripts/dqlite/build.sh

DQLITE_BUILD_IMAGE=ubuntu:22.04
DQLITE_BUILD_CONTAINER_PREFIX=juju-dqlite-build-server
DQLITE_ARCHIVE_NAME=juju-dqlite-static-lib-deps.tar.bz2
DQLITE_UNPACKED_ARCHIVE_NAME=juju-dqlite-static-lib-deps
DQLITE_S3_BUCKET=s3://dqlite-static-libs
DQLITE_PLATFORMS ?= linux/amd64 linux/arm64

dep_platform_targets = $(addprefix dqlite-deps-build-, $(subst /,_,${1})))
dep_platform_path = $(addprefix ${BUILD_DIR}/, $(addsuffix /dep/${DQLITE_UNPACKED_ARCHIVE_NAME}, $(subst /,_, $1)))
dqlite_s3_archive_path = $(addprefix ${DQLITE_S3_BUCKET}/,$(shell date -u +"%Y-%m-%d")-dqlite-deps-$1.tar.bz2)
dqlite_s3_latest_archive_path = $(addprefix ${DQLITE_S3_BUCKET}/,latest-dqlite-deps-$1.tar.bz2)

define DQLITE_ARCHIVE_TARGETS
	$(call dep_platform_targets,${DQLITE_PLATFORMS})
endef

.PHONY: phony_explicit
phony_explicit:
## phone_explicit is a dummy target that can be added to pattern targets to
## them phony make.

.PHONY: build-dqlite
build-dqlite: ${DQLITE_ARCHIVE_TARGETS}

${BUILD_DIR}/%/dep/${DQLITE_UNPACKED_ARCHIVE_NAME}: ${BUILD_DIR}/%/dep/${DQLITE_ARCHIVE_NAME}
	@tar -xjf ${BUILD_DIR}/$*/dep/${DQLITE_ARCHIVE_NAME} -C ${BUILD_DIR}/$*/dep/

${BUILD_DIR}/%/dep/${DQLITE_ARCHIVE_NAME}:
	@$(eval s3_latest_archive_path = $(call dqlite_s3_latest_archive_path,$*))
	@mkdir -p "${BUILD_DIR}/$*/dep"
	@echo "DQLITE: Pulling ${s3_latest_archive_path} from s3"
	@aws s3 cp ${s3_latest_archive_path} ${BUILD_DIR}/$*/dep/${DQLITE_ARCHIVE_NAME}

dqlite-deps-build-%: phony_explicit
	@$(eval container = ${DQLITE_BUILD_CONTAINER_PREFIX}-$(subst _,-,$*))
	@$(eval os_arch_tuple = $(subst _,/,$*))
	@lxc delete -f ${container} &>/dev/null || true

	@echo "DQLITE: Creating build container \"${container}\" using ${DQLITE_BUILD_IMAGE}"
	@lxc launch ${DQLITE_BUILD_IMAGE} ${container}

	@echo "DQLITE: Waiting for ${container} to become ready..."
	@lxc exec ${container} -- bash -c 'while [ "$$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

	@echo "DQLITE: Pushing setup script to ${container}"
	@lxc file push ${SETUP_SCRIPT} ${container}/tmp/setup.sh

	@echo "DQLITE: Executing setup script"
	@lxc exec -T ${container} bash /tmp/setup.sh

	@echo "DQLITE: Pushing build script to ${container}"
	@lxc file push ${BUILD_SCRIPT} ${container}/tmp/build.sh

	@echo "DQLITE: Executing build script"
	@lxc exec -T ${container} --env BUILD_DIR="/tmp/build_$*" bash /tmp/build.sh ${os_arch_tuple}

	@echo "DQLITE: Pulling lib archive"
	@mkdir -p "${BUILD_DIR}/$*/dep"
	@lxc file pull ${container}/tmp/build_$*/juju-dqlite-static-lib-deps.tar.bz2 $@

dqlite-deps-push-%: phony_explicit ${BUILD_DIR}/%/dep/${DQLITE_ARCHIVE_NAME}
	@$(eval archive_path = ${BUILD_DIR}/$*/dep/${DQLITE_ARCHIVE_NAME})
	@$(eval s3_archive_path = $(call dqlite_s3_archive_path,$*))
	@echo "DQLITE: Pushing ${archive_path} to s3"
	@aws s3 cp --acl public-read ${archive_path} ${s3_archive_path}
	@echo "DQLITE: making ${s3_archive_path} latest in s3"
	@aws s3 cp --acl public-read ${s3_archive_path} $(call dqlite_s3_latest_archive_path,$*)

dqlite-deps-push: phony_explicit $(addprefix dqlite-deps-push-,$(subst /,_,${DQLITE_PLATFORMS}))
