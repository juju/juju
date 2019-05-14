# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2019 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Functionality for handling installed or other juju binaries
# (including paths etc.)


from __future__ import print_function

import logging
# import shutil
from time import sleep

from google.cloud import container_v1
from google.oauth2 import service_account
from google.api_core import exceptions
from jujupy.utility import until_timeout

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)
CLUSTER_STATUS = container_v1.enums.Cluster.Status


@register_provider
class GKE(Base):

    name = K8sProviderType.GKE

    driver = None
    gke_cluster_name = None
    gke_cluster = None

    default_params = None

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.gke_cluster_name = self.client.env.controller.name  # use controller name as cluster name
        self.default_storage_class_name = ''
        self.__init_driver(bs_manager.client.env)

    def __init_driver(self, env):
        zone = env.get_host_cloud_region()[2] + '-b'
        cfg = env._config
        self.default_params = dict(
            project_id=cfg['project_id'],
            zone=zone,
        )

        self.driver = container_v1.ClusterManagerClient(
            credentials=service_account.Credentials.from_service_account_info(cfg)
        )

        # list all running clusters
        running_clusters = self.driver.list_clusters(**self.default_params)
        logger.warn('running gke clusters: %s', running_clusters)

    def _ensure_cluster_stack(self):
        self.provision_gke()

    def _tear_down_substrate(self):
        try:
            self.driver.delete_cluster(
                cluster_id=self.gke_cluster_name, **self.default_params,
            )
        except exceptions.NotFound as e:
            logger.warn(e)

    def _ensure_kube_dir(self):
        # TODO
        ...

    def _ensure_cluster_config(self):
        # TODO
        ...

    def _node_address_getter(self, node):
        # TODO
        ...

    def _get_cluster(self, name):
        return self.driver.get_cluster(
            cluster_id=self.gke_cluster_name, **self.default_params,
        )

    def provision_gke(self):
        def log_remaining(remaining, msg=''):
            sleep(3)
            if remaining % 30 == 0:
                msg += ' timeout in %ss...' % remaining
                logger.warn(msg)

        # do pre cleanup;
        self._tear_down_substrate()
        for remaining in until_timeout(600):
            # wait for the old cluster to be deleted.
            try:
                self._get_cluster(self.gke_cluster_name)
            except exceptions.NotFound:
                break
            finally:
                log_remaining(remaining)

        # provision cluster.
        cluster = dict(name=self.gke_cluster_name, initial_node_count=1)
        logger.info('creating cluster -> %s', cluster)
        r = self.driver.create_cluster(
            cluster=cluster,
            **self.default_params,
        )
        logger.warn('created cluster -> %s', r)
        # wait until cluster fully provisioned.
        logger.info('waiting for cluster fully provisioned.')
        for remaining in until_timeout(600):
            try:
                if self._get_cluster(self.gke_cluster_name).status == CLUSTER_STATUS.RUNNING:
                    return
            except Exception as e: # noqa
                logger.warn(e)
            finally:
                log_remaining(remaining)
