# Copyright 2018 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import json
import os

import charmhelpers.contrib.openstack.alternatives as alternatives
import charmhelpers.contrib.openstack.context as context

import charmhelpers.core.hookenv as hookenv
import charmhelpers.core.host as host
import charmhelpers.core.templating as templating
import charmhelpers.core.unitdata as unitdata

VAULTLOCKER_BACKEND = 'charm-vaultlocker'


class VaultKVContext(context.OSContextGenerator):
    """Vault KV context for interaction with vault-kv interfaces"""
    interfaces = ['secrets-storage']

    def __init__(self, secret_backend=None):
        super(context.OSContextGenerator, self).__init__()
        self.secret_backend = (
            secret_backend or 'charm-{}'.format(hookenv.service_name())
        )

    def __call__(self):
        db = unitdata.kv()
        last_token = db.get('last-token')
        secret_id = db.get('secret-id')
        for relation_id in hookenv.relation_ids(self.interfaces[0]):
            for unit in hookenv.related_units(relation_id):
                data = hookenv.relation_get(unit=unit,
                                            rid=relation_id)
                vault_url = data.get('vault_url')
                role_id = data.get('{}_role_id'.format(hookenv.local_unit()))
                token = data.get('{}_token'.format(hookenv.local_unit()))

                if all([vault_url, role_id, token]):
                    token = json.loads(token)
                    vault_url = json.loads(vault_url)

                    # Tokens may change when secret_id's are being
                    # reissued - if so use token to get new secret_id
                    if token != last_token:
                        secret_id = retrieve_secret_id(
                            url=vault_url,
                            token=token
                        )
                        db.set('secret-id', secret_id)
                        db.set('last-token', token)
                        db.flush()

                    ctxt = {
                        'vault_url': vault_url,
                        'role_id': json.loads(role_id),
                        'secret_id': secret_id,
                        'secret_backend': self.secret_backend,
                    }
                    vault_ca = data.get('vault_ca')
                    if vault_ca:
                        ctxt['vault_ca'] = json.loads(vault_ca)
                    self.complete = True
                    return ctxt
        return {}


def write_vaultlocker_conf(context, priority=100):
    """Write vaultlocker configuration to disk and install alternative

    :param context: Dict of data from vault-kv relation
    :ptype: context: dict
    :param priority: Priority of alternative configuration
    :ptype: priority: int"""
    charm_vl_path = "/var/lib/charm/{}/vaultlocker.conf".format(
        hookenv.service_name()
    )
    host.mkdir(os.path.dirname(charm_vl_path), perms=0o700)
    templating.render(source='vaultlocker.conf.j2',
                      target=charm_vl_path,
                      context=context, perms=0o600),
    alternatives.install_alternative('vaultlocker.conf',
                                     '/etc/vaultlocker/vaultlocker.conf',
                                     charm_vl_path, priority)


def vault_relation_complete(backend=None):
    """Determine whether vault relation is complete

    :param backend: Name of secrets backend requested
    :ptype backend: string
    :returns: whether the relation to vault is complete
    :rtype: bool"""
    vault_kv = VaultKVContext(secret_backend=backend or VAULTLOCKER_BACKEND)
    vault_kv()
    return vault_kv.complete


# TODO: contrib a high level unwrap method to hvac that works
def retrieve_secret_id(url, token):
    """Retrieve a response-wrapped secret_id from Vault

    :param url: URL to Vault Server
    :ptype url: str
    :param token: One shot Token to use
    :ptype token: str
    :returns: secret_id to use for Vault Access
    :rtype: str"""
    import hvac
    client = hvac.Client(url=url, token=token)
    response = client._post('/v1/sys/wrapping/unwrap')
    if response.status_code == 200:
        data = response.json()
        return data['data']['secret_id']
