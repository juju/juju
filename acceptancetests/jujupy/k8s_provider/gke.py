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
import yaml
import json
from time import sleep
import shutil
import tempfile
import os

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
    cluster_name = None

    driver = None
    default_params = None

    def __init__(self, bs_manager, timeout=1800):
        super().__init__(bs_manager, timeout)

        self.cluster_name = self.client.env.controller.name  # use controller name for cluster name
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

        self.__ensure_gcloud(cfg)

        # list all running clusters
        running_clusters = self.driver.list_clusters(**self.default_params)
        logger.info('running gke clusters: %s', running_clusters)

    def _ensure_cluster_stack(self):
        self.provision_gke()

    def __ensure_gcloud(self, cfg):
        gcloud_bin = shutil.which('gcloud')
        if gcloud_bin is None:
            # ensure gcloud installed.
            self.sh('sudo', 'snap', 'install', 'google-cloud-sdk', '--classic')
            logger.debug("gcloud installed successfully")
            gcloud_bin = shutil.which('gcloud')

        cred = {
            k: cfg[k] for k in (
                'project_id',
                'private_key_id',
                'private_key',
                'client_email',
                'client_id',
                'auth_uri',
                'token_uri',
                'auth_provider_x509_cert_url',
                'client_x509_cert_url',
            )
        }
        cred['type'] = 'service_account'
        cred_file = tempfile.NamedTemporaryFile(suffix='.json', delete=False)
        try:
            with open(cred_file.name, 'w') as f:
                json.dump(cred, f)

            # set credential;
            o = self.sh(gcloud_bin, 'auth', 'activate-service-account', '--key-file', cred_file.name)
            logger.debug(o)
            # set project;
            o = self.sh(gcloud_bin, 'config', 'set', 'project', cfg['project_id'])
            logger.debug(o)
        finally:
            os.unlink(cred_file.name)

    def _tear_down_substrate(self):
        try:
            self.driver.delete_cluster(
                cluster_id=self.cluster_name, **self.default_params,
            )
        except exceptions.NotFound:
            pass

    def _ensure_kube_dir(self):
        cluster = self._get_cluster(self.cluster_name)
        cluster_config = ClusterConfig(
            project_id=self.default_params['project_id'],
            cluster=cluster,
        )
        self.kubeconfig_cluster_name = cluster_config.context_name
        kubeconfig_content = cluster_config.dump()
        with open(self.kube_config_path, 'w') as f:
            logger.debug('writing kubeconfig to %s\n%s', self.kube_config_path, kubeconfig_content)
            f.write(kubeconfig_content)

        # ensure kubectl
        self._ensure_kubectl_bin()

    def _ensure_cluster_config(self):
        ...

    def _node_address_getter(self, node):
        return [addr['address'] for addr in node['status']['addresses'] if addr['type'] == 'ExternalIP'][0]

    def _get_cluster(self, name):
        return self.driver.get_cluster(
            cluster_id=self.cluster_name, **self.default_params,
        )

    def provision_gke(self):
        def log_remaining(remaining, msg=''):
            sleep(3)
            if remaining % 30 == 0:
                msg += ' timeout in %ss...' % remaining
                logger.info(msg)

        # do pre cleanup;
        self._tear_down_substrate()
        for remaining in until_timeout(600):
            # wait for the old cluster to be deleted.
            try:
                self._get_cluster(self.cluster_name)
            except exceptions.NotFound:
                break
            finally:
                log_remaining(remaining)

        # provision cluster.
        cluster = dict(
            name=self.cluster_name,
            node_pools=[dict(
                name='default-pool',
                initial_node_count=1,
                config=dict(
                    machine_type='n1-standard-2',
                ),
                autoscaling=dict(
                    enabled=True,
                    min_node_count=1,
                    max_node_count=3,
                ),
            )],
        )
        logger.info('creating cluster -> %s', cluster)
        r = self.driver.create_cluster(
            cluster=cluster,
            **self.default_params,
        )
        logger.info('created cluster -> %s', r)
        # wait until cluster fully provisioned.
        logger.info('waiting for cluster fully provisioned.')
        for remaining in until_timeout(600):
            try:
                cluster = self._get_cluster(self.cluster_name)
                if cluster.status == CLUSTER_STATUS.RUNNING:
                    return
            except Exception as e:  # noqa
                logger.info(e)
            finally:
                log_remaining(remaining)


class ClusterConfig(object):

    def __init__(self, project_id, cluster):
        self.cluster_name = cluster.name
        self.zone_id = cluster.zone
        self.project_id = project_id
        self.server = 'https://' + cluster.endpoint
        self.ca_data = cluster.master_auth.cluster_ca_certificate
        self.client_key_data = cluster.master_auth.client_key
        self.client_cert_data = cluster.master_auth.client_certificate
        self.context_name = 'gke_{project_id}_{zone_id}_{cluster_name}'.format(
            project_id=self.project_id,
            zone_id=self.zone_id,
            cluster_name=self.cluster_name,
        )

    def user(self, auth_provider='gcp'):
        if auth_provider is None:
            return {
                'name': self.context_name,
                'user': {
                    'client-certificate-data': self.client_cert_data,
                    'client-key-data': self.client_key_data
                },
            }
        # TODO(ycliuhw): remove gcloud dependency once 'google-cloud-container' supports defining master-auth type.
        gcloud_bin_path = shutil.which('gcloud')
        if gcloud_bin_path is None:
            raise AssertionError("gcloud bin is required!")
        return {
            'name': self.context_name,
            'user': {
                'auth-provider': {
                    'name': auth_provider,
                    'config': {
                        # Command for gcloud credential helper
                        'cmd-path': gcloud_bin_path,
                        # Args for gcloud credential helper
                        'cmd-args': 'config config-helper --format=json',
                        # JSONpath to the field that is the raw access token
                        'token-key': '{.credential.access_token}',
                        # JSONpath to the field that is the expiration timestamp
                        'expiry-key': '{.credential.token_expiry}',
                    }
                },
            },
        }

    @property
    def cluster(self):
        return {
            'name': self.context_name,
            'cluster': {
                'server': self.server,
                'certificate-authority-data': self.ca_data,
            },
        }

    @property
    def context(self):
        return {
            'name': self.context_name,
            'context': {
                'cluster': self.context_name,
                'user': self.context_name,
            },
        }

    def dump(self):
        d = {
            'apiVersion': 'v1',
            'kind': 'Config',
            'contexts': [self.context],
            'clusters': [self.cluster],
            'current-context': self.context_name,
            'preferences': {},
            'users': [self.user()],
        }
        return yaml.dump(d)
