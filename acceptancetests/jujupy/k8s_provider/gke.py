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

# from libcloud.container.types import Provider
# from libcloud.container.providers import get_driver
from google.oauth2 import service_account
from apiclient.discovery import build
from jujupy.utility import until_timeout

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)


@register_provider
class GKE(Base):

    name = K8sProviderType.GKE

    driver = None
    gke_cluster_name = None
    gke_cluster = None

    project_id = None
    zone = 'asia-southeast1-b'

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.gke_cluster_name = self.client.env.controller.name  # use controller name as cluster name
        self.default_storage_class_name = ''
        self.__init_driver(bs_manager.client.env)

    def __init_driver(self, env):
        cfg = env._config
        self.project_id = cfg['project_id']

        self.driver = build(
            'container', 'v1',
            credentials=service_account.Credentials.from_service_account_info(cfg),
        ).projects().zones().clusters()

        # list all running clusters
        running_clusters = self.driver.list(
            projectId=self.project_id, zone=self.zone,
        ).execute()
        logger.info('running gke clusters: %s', running_clusters)

    def _ensure_cluster_stack(self):
        self.provision_gke()

    def _tear_down_substrate(self):
        try:
            self.driver.delete(
                projectId=self.project_id,
                zone=self.zone,
                clusterId=self.gke_cluster_name,
            ).execute()
        except Exception as e:
            logger.warn(e)
            if '404' in str(e):
                return
            raise

    def _ensure_kube_dir(self):
        # TODO
        ...

    def _ensure_cluster_config(self):
        # TODO
        ...

    def _node_address_getter(self, node):
        # TODO
        ...

    def _get_cluster(self):
        return self.driver.get(
            projectId=self.project_id,
            zone=self.zone,
            clusterId=self.gke_cluster_name,
        ).execute()

    def provision_gke(self):
        def log_remaining(remaining):
            sleep(3)
            if remaining % 30 == 0:
                logger.info('tearing down existing cluster... timeout in %ss', remaining)

        for remaining in until_timeout(600):
            # do pre cleanup;
            try:
                self._tear_down_substrate()
                break
            except Exception as e: # noqa
                logger.warn(e)
            finally:
                log_remaining(remaining)

        # provision cluster.
        self.gke_cluster = self.driver.create(
            projectId=self.project_id,
            zone=self.zone,
            body=dict(cluster={'name': self.gke_cluster_name, 'initial_node_count': 1}),
        ).execute()
        # wait until cluster fully provisioned.
        for remaining in until_timeout(600):
            try:
                if self._get_cluster().get('status') == 'RUNNING':
                    return
            except Exception as e: # noqa
                logger.warn(e)
            finally:
                log_remaining(remaining)
