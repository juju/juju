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

from libcloud.container.types import Provider
from libcloud.container.providers import get_driver

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

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.gke_cluster_name = self.client.env.controller.name  # use controller name as cluster name
        self.default_storage_class_name = ''
        self.__init_driver(bs_manager.client.env)

    def __init_driver(self, env):
        cfg = env._config
        self.driver = get_driver(Provider.GKE)(
            user_id=cfg['client-email'],
            key=cfg['private-key'],
            datacenter=env.get_host_cloud_region()[2],
            project=cfg['project-id'],
        )
        # list all running clusters
        running_clusters = self.driver.list_clusters()
        logger.info('running gke clusters: %s', running_clusters)

    def _ensure_cluster_stack(self):
        self.provision_gke()

    def _tear_down_substrate(self):
        self.driver.destroy_cluster(self.gke_cluster_name)

    def _ensure_kube_dir(self):
        # TODO
        ...

    def _ensure_cluster_config(self):
        # TODO
        ...

    def _node_address_getter(self, node):
        # TODO
        ...

    def provision_gke(self):
        self.driver.create_cluster(self.gke_cluster_name)
