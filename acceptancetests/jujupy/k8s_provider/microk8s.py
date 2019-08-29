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
import shutil
import os
import json

import dns.resolver

from .base import (
    Base,
    K8sProviderType,
)
from .factory import register_provider


logger = logging.getLogger(__name__)


@register_provider
class MicroK8s(Base):

    name = K8sProviderType.MICROK8S
    cloud_name = 'microk8s'  # built-in cloud name

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)
        self.default_storage_class_name = 'microk8s-hostpath'

    def _ensure_cluster_stack(self):
        self.__ensure_microk8s_installed()

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
        self.__enable_addons()
        try:
            self.__tmp_fix_patch_coredns()
        except Exception as e:
            logger.error(e)

    def _node_address_getter(self, node):
        # microk8s uses the node's 'InternalIP`.
        return [addr['address'] for addr in node['status']['addresses'] if addr['type'] == 'InternalIP'][0]

    def __enable_addons(self):
        out = self.sh(
            'microk8s.enable',
            'storage', 'dns', 'dashboard', 'ingress',  # addons are required to enable.
        )
        logger.debug(out)

    def __ensure_microk8s_installed(self):
        # unfortunately, we needs sudo!
        if shutil.which('microk8s.kubectl') is None:
            self.sh('sudo', 'snap', 'install', 'microk8s', '--classic', '--stable')
            logger.debug("microk8s installed successfully")
        logger.debug(
            "microk8s status \n%s",
            self.sh('microk8s.status', '--wait-ready', '--timeout', self.timeout, '--yaml'),
        )

    def __tmp_fix_patch_coredns(self):
        # patch nameservers of coredns because the google one used in microk8s is blocked in our network.
        def ping(addr):
            return os.system('ping -c 1 ' + addr) == 0

        def get_nameserver():
            nameservers = dns.resolver.Resolver().nameservers
            for ns in nameservers:
                if ping(ns):
                    return ns
            raise Exception('No working nameservers found from %s to use for patching coredns' % nameservers)

        coredns_cm = self.get_configmap('kube-system', 'coredns')
        data = coredns_cm['data']
        data['Corefile'] = data['Corefile'].replace('8.8.8.8 8.8.4.4', get_nameserver())
        coredns_cm['data'] = data
        self.kubectl_apply(json.dumps(coredns_cm))
