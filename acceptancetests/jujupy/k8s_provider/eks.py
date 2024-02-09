# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2020 Canonical Ltd.
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
import subprocess
from time import sleep

import yaml

from jujupy.utility import until_timeout

from .base import Base, K8sProviderType
from .factory import register_provider

logger = logging.getLogger(__name__)


@register_provider
class EKS(Base):

    name = K8sProviderType.EKS
    location = None
    parameters = None

    def __init__(self, bs_manager, cluster_name=None, enable_rbac=False, timeout=1800):
        super().__init__(bs_manager, cluster_name, enable_rbac, timeout)

        self._eksctl_bin = os.path.join(self.juju_home, 'eksctl')
        self._ensure_eksctl_bin()
        self.default_storage_class_name = ''
        self.__init_client(bs_manager.client.env)

    def __init_client(self, env):
        credential = {
            'AWS_ACCESS_KEY_ID': env._config['access-key'],
            'AWS_SECRET_ACCESS_KEY': env._config['secret-key'],
        }
        for k, v in credential.items():
            os.environ[k] = v

        self.location = env._config['location']

        # list all running clusters.
        logger.info(
            'Running eks clusters in %s: \n%s', self.location,
            yaml.dump(self.list_clusters(self.location))
        )

    def _ensure_cluster_stack(self):
        self.provision_eks()

    def _ensure_eksctl_bin(self):
        path = shutil.which('eksctl')
        if path is not None:
            self._eksctl_bin = path
        else:
            self.sh(
                '''curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp && mv /tmp/eksctl %s
''' % self._eksctl_bin, shell=True, ignore_quote=True)

    def eksctl(self, *args):
        return self.sh(self._eksctl_bin, *args)

    def _tear_down_substrate(self):
        logger.info("Deleting the EKS instance {0}".format(self.cluster_name))
        try:
            o = self.eksctl('delete', 'cluster', self.cluster_name, '--region', self.location)
            logger.info("cluster %s has been deleted -> \n%s", self.cluster_name, o)
        except Exception as e:
            if is_404(e):
                return
            logger.error(e)
            raise

    def list_clusters(self, region):
        return json.loads(
            self.eksctl('get', 'cluster', '--region', region, '-o', 'json'),
        )

    def get_lb_svc_address(self, svc_name, namespace):
        return json.loads(
            self.kubectl('-n', namespace, 'get', 'svc', svc_name, '-o', 'json')
        )['status']['loadBalancer']['ingress'][0]['hostname']

    def _ensure_kube_dir(self):
        logger.info("Writing kubeconfig to %s" % self.kube_config_path)
        self.eksctl(
            'utils', 'write-kubeconfig', '--cluster', self.cluster_name,
            '--region', self.location, '--kubeconfig', self.kube_config_path,
        )

        with open(self.kube_config_path, 'r') as f:
            self.kubeconfig_cluster_name = yaml.load(f, yaml.SafeLoader)['contexts'][0]['context']['cluster']

        # ensure kubectl
        self._ensure_kubectl_bin()

    def _ensure_cluster_config(self):
        ...

    def _node_address_getter(self, node):
        raise NotImplementedError()

    def _get_cluster(self, name):
        return self.eksctl('get', 'cluster', '--name', self.cluster_name, '--region', self.location, '-o', 'json')

    def provision_eks(self):
        def log_remaining(remaining, msg=''):
            sleep(3)
            if remaining % 30 == 0:
                msg += ' timeout in %ss...' % remaining
                logger.info(msg)

        # do pre cleanup;
        self._tear_down_substrate()

        for remaining in until_timeout(600):
            # wait for the existing cluster to be deleted.
            try:
                self._get_cluster(self.cluster_name)
            except Exception as e:
                if is_404(e):
                    break
                raise
            else:
                log_remaining(remaining)

        # provision cluster.
        logger.info('Creating cluster -> %s', self.cluster_name)
        try:
            o = self.eksctl(
                'create', 'cluster',
                '--name', self.cluster_name,
                '--version', '1.27',
                '--region', self.location,
                '--nodes', 3,
                '--nodes-min', 1,
                '--nodes-max', 3,
                '--ssh-access',
                '--ssh-public-key=' + os.path.expanduser('~/.ssh/id_ed25519.pub'),
                '--managed',
            )
            logger.info("cluster %s has been successfully provisioned -> \n%s", self.cluster_name, o)
        except subprocess.CalledProcessError as e:
            logger.error('Error attempting to create the EKS instance %s', e.__dict__)
            raise e


def is_404(err):
    try:
        err_msg = err.output.decode()
        return any([keyword in err_msg for keyword in ['404', 'does not exist', 'ResourceNotFoundException']])
    except Exception:
        return False
