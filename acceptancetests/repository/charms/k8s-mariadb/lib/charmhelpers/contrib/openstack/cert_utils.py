# Copyright 2014-2018 Canonical Limited.
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

# Common python helper functions used for OpenStack charm certificats.

import os
import json

from charmhelpers.contrib.network.ip import (
    get_hostname,
    resolve_network_cidr,
)
from charmhelpers.core.hookenv import (
    local_unit,
    network_get_primary_address,
    config,
    related_units,
    relation_get,
    relation_ids,
    unit_get,
    NoNetworkBinding,
    log,
    WARNING,
)
from charmhelpers.contrib.openstack.ip import (
    ADMIN,
    resolve_address,
    get_vip_in_network,
    INTERNAL,
    PUBLIC,
    ADDRESS_MAP)

from charmhelpers.core.host import (
    mkdir,
    write_file,
)

from charmhelpers.contrib.hahelpers.apache import (
    install_ca_cert
)


class CertRequest(object):

    """Create a request for certificates to be generated
    """

    def __init__(self, json_encode=True):
        self.entries = []
        self.hostname_entry = None
        self.json_encode = json_encode

    def add_entry(self, net_type, cn, addresses):
        """Add a request to the batch

        :param net_type: str netwrok space name request is for
        :param cn: str Canonical Name for certificate
        :param addresses: [] List of addresses to be used as SANs
        """
        self.entries.append({
            'cn': cn,
            'addresses': addresses})

    def add_hostname_cn(self):
        """Add a request for the hostname of the machine"""
        ip = unit_get('private-address')
        addresses = [ip]
        # If a vip is being used without os-hostname config or
        # network spaces then we need to ensure the local units
        # cert has the approriate vip in the SAN list
        vip = get_vip_in_network(resolve_network_cidr(ip))
        if vip:
            addresses.append(vip)
        self.hostname_entry = {
            'cn': get_hostname(ip),
            'addresses': addresses}

    def add_hostname_cn_ip(self, addresses):
        """Add an address to the SAN list for the hostname request

        :param addr: [] List of address to be added
        """
        for addr in addresses:
            if addr not in self.hostname_entry['addresses']:
                self.hostname_entry['addresses'].append(addr)

    def get_request(self):
        """Generate request from the batched up entries

        """
        if self.hostname_entry:
            self.entries.append(self.hostname_entry)
        request = {}
        for entry in self.entries:
            sans = sorted(list(set(entry['addresses'])))
            request[entry['cn']] = {'sans': sans}
        if self.json_encode:
            return {'cert_requests': json.dumps(request, sort_keys=True)}
        else:
            return {'cert_requests': request}


def get_certificate_request(json_encode=True):
    """Generate a certificatee requests based on the network confioguration

    """
    req = CertRequest(json_encode=json_encode)
    req.add_hostname_cn()
    # Add os-hostname entries
    for net_type in [INTERNAL, ADMIN, PUBLIC]:
        net_config = config(ADDRESS_MAP[net_type]['override'])
        try:
            net_addr = resolve_address(endpoint_type=net_type)
            ip = network_get_primary_address(
                ADDRESS_MAP[net_type]['binding'])
            addresses = [net_addr, ip]
            vip = get_vip_in_network(resolve_network_cidr(ip))
            if vip:
                addresses.append(vip)
            if net_config:
                req.add_entry(
                    net_type,
                    net_config,
                    addresses)
            else:
                # There is network address with no corresponding hostname.
                # Add the ip to the hostname cert to allow for this.
                req.add_hostname_cn_ip(addresses)
        except NoNetworkBinding:
            log("Skipping request for certificate for ip in {} space, no "
                "local address found".format(net_type), WARNING)
    return req.get_request()


def create_ip_cert_links(ssl_dir, custom_hostname_link=None):
    """Create symlinks for SAN records

    :param ssl_dir: str Directory to create symlinks in
    :param custom_hostname_link: str Additional link to be created
    """
    hostname = get_hostname(unit_get('private-address'))
    hostname_cert = os.path.join(
        ssl_dir,
        'cert_{}'.format(hostname))
    hostname_key = os.path.join(
        ssl_dir,
        'key_{}'.format(hostname))
    # Add links to hostname cert, used if os-hostname vars not set
    for net_type in [INTERNAL, ADMIN, PUBLIC]:
        try:
            addr = resolve_address(endpoint_type=net_type)
            cert = os.path.join(ssl_dir, 'cert_{}'.format(addr))
            key = os.path.join(ssl_dir, 'key_{}'.format(addr))
            if os.path.isfile(hostname_cert) and not os.path.isfile(cert):
                os.symlink(hostname_cert, cert)
                os.symlink(hostname_key, key)
        except NoNetworkBinding:
            log("Skipping creating cert symlink for ip in {} space, no "
                "local address found".format(net_type), WARNING)
    if custom_hostname_link:
        custom_cert = os.path.join(
            ssl_dir,
            'cert_{}'.format(custom_hostname_link))
        custom_key = os.path.join(
            ssl_dir,
            'key_{}'.format(custom_hostname_link))
        if os.path.isfile(hostname_cert) and not os.path.isfile(custom_cert):
            os.symlink(hostname_cert, custom_cert)
            os.symlink(hostname_key, custom_key)


def install_certs(ssl_dir, certs, chain=None):
    """Install the certs passed into the ssl dir and append the chain if
       provided.

    :param ssl_dir: str Directory to create symlinks in
    :param certs: {} {'cn': {'cert': 'CERT', 'key': 'KEY'}}
    :param chain: str Chain to be appended to certs
    """
    for cn, bundle in certs.items():
        cert_filename = 'cert_{}'.format(cn)
        key_filename = 'key_{}'.format(cn)
        cert_data = bundle['cert']
        if chain:
            # Append chain file so that clients that trust the root CA will
            # trust certs signed by an intermediate in the chain
            cert_data = cert_data + chain
        write_file(
            path=os.path.join(ssl_dir, cert_filename),
            content=cert_data, perms=0o640)
        write_file(
            path=os.path.join(ssl_dir, key_filename),
            content=bundle['key'], perms=0o640)


def process_certificates(service_name, relation_id, unit,
                         custom_hostname_link=None):
    """Process the certificates supplied down the relation

    :param service_name: str Name of service the certifcates are for.
    :param relation_id: str Relation id providing the certs
    :param unit: str Unit providing the certs
    :param custom_hostname_link: str Name of custom link to create
    """
    data = relation_get(rid=relation_id, unit=unit)
    ssl_dir = os.path.join('/etc/apache2/ssl/', service_name)
    mkdir(path=ssl_dir)
    name = local_unit().replace('/', '_')
    certs = data.get('{}.processed_requests'.format(name))
    chain = data.get('chain')
    ca = data.get('ca')
    if certs:
        certs = json.loads(certs)
        install_ca_cert(ca.encode())
        install_certs(ssl_dir, certs, chain)
        create_ip_cert_links(
            ssl_dir,
            custom_hostname_link=custom_hostname_link)


def get_requests_for_local_unit(relation_name=None):
    """Extract any certificates data targeted at this unit down relation_name.

    :param relation_name: str Name of relation to check for data.
    :returns: List of bundles of certificates.
    :rtype: List of dicts
    """
    local_name = local_unit().replace('/', '_')
    raw_certs_key = '{}.processed_requests'.format(local_name)
    relation_name = relation_name or 'certificates'
    bundles = []
    for rid in relation_ids(relation_name):
        for unit in related_units(rid):
            data = relation_get(rid=rid, unit=unit)
            if data.get(raw_certs_key):
                bundles.append({
                    'ca': data['ca'],
                    'chain': data.get('chain'),
                    'certs': json.loads(data[raw_certs_key])})
    return bundles


def get_bundle_for_cn(cn, relation_name=None):
    """Extract certificates for the given cn.

    :param cn: str Canonical Name on certificate.
    :param relation_name: str Relation to check for certificates down.
    :returns: Dictionary of certificate data,
    :rtype: dict.
    """
    entries = get_requests_for_local_unit(relation_name)
    cert_bundle = {}
    for entry in entries:
        for _cn, bundle in entry['certs'].items():
            if _cn == cn:
                cert_bundle = {
                    'cert': bundle['cert'],
                    'key': bundle['key'],
                    'chain': entry['chain'],
                    'ca': entry['ca']}
                break
        if cert_bundle:
            break
    return cert_bundle
