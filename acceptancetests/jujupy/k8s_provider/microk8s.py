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

import json
import logging
import os
import shutil
from pprint import pformat
from time import sleep

import dns.resolver
import yaml

from jujupy.utility import until_timeout

from .base import Base, K8sProviderType
from .factory import register_provider

logger = logging.getLogger(__name__)


@register_provider
class MicroK8s(Base):

    name = K8sProviderType.MICROK8S
    cloud_name = 'microk8s'  # built-in cloud name

    def __init__(self, bs_manager, cluster_name=None, enable_rbac=False, timeout=1800):
        super().__init__(bs_manager, cluster_name, enable_rbac, timeout)
        self.default_storage_class_name = 'microk8s-hostpath'

    def _ensure_cluster_stack(self):
        pass

    def _tear_down_substrate(self):
        # No need to tear down microk8s.
        logger.warn('skip tearing down microk8s')

    def _ensure_kube_dir(self):
        # choose to use microk8s.kubectl
        mkubectl = shutil.which('microk8s.kubectl')
        if mkubectl is None:
            raise AssertionError("microk8s.kubectl is required!")
        self.kubectl_path = mkubectl

        # export microk8s.config to kubeconfig file.
        with open(self.kube_config_path, 'w') as f:
            kubeconfig_content = self.sh('microk8s.config')
            logger.debug('writing kubeconfig to %s\n%s', self.kube_config_path, kubeconfig_content)
            f.write(kubeconfig_content)

    def _ensure_cluster_config(self):
        self.enable_microk8s_addons()

    def _node_address_getter(self, node):
        # microk8s uses the node's 'InternalIP`.
        return [addr['address'] for addr in node['status']['addresses'] if addr['type'] == 'InternalIP'][0]

    def _microk8s_status(self, wait_ready=False, timeout=None):
        timeout = timeout or 2 * 60
        args = ['microk8s.status', '--yaml']
        if wait_ready:
            args += ['--wait-ready', '--timeout', timeout]
        return yaml.load(
            self.sh(*args), Loader=yaml.Loader,
        )

    def enable_microk8s_addons(self, addons=None):
        # addons are required to be enabled.
        addons = addons or ['storage', 'dns', 'ingress']
        if self.enable_rbac:
            if 'rbac' not in addons:
                addons.append('rbac')
        else:
            addons = [addon for addon in addons if addon != 'rbac']
            logger.info('disabling rbac -> %s', self.sh('microk8s.disable', 'rbac'))

        def wait_until_ready(timeout, checker):
            for _ in until_timeout(timeout):
                if checker():
                    break
                sleep(5)

        def check_addons():
            addons_status = self._microk8s_status(True)['addons']
            not_enabled = [
                # addon can be like metallb:10.64.140.43-10.64.140.49
                addon for addon in addons if addons_status.get(addon.split(':')[0]) != 'enabled'
            ]
            if len(not_enabled) == 0:
                logger.info('addons are all ready now -> \n%s', pformat(addons_status))
                return True
            logger.info(f'addons are waiting to be enabled: {", ".join(not_enabled)}...')
            return False

        out = self.sh('microk8s.enable', *addons)
        logger.info(out)
        # wait for a bit to let all addons are fully provisoned.
        wait_until_ready(300, check_addons)
