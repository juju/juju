# Copyright 2014-2015 Canonical Limited.
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

import glob
import json
import os
import re
import time
from base64 import b64decode
from subprocess import check_call, CalledProcessError

import six

from charmhelpers.fetch import (
    apt_install,
    filter_installed_packages,
)
from charmhelpers.core.hookenv import (
    config,
    is_relation_made,
    local_unit,
    log,
    relation_get,
    relation_ids,
    related_units,
    relation_set,
    unit_get,
    unit_private_ip,
    charm_name,
    DEBUG,
    INFO,
    WARNING,
    ERROR,
    status_set,
)

from charmhelpers.core.sysctl import create as sysctl_create
from charmhelpers.core.strutils import bool_from_string
from charmhelpers.contrib.openstack.exceptions import OSContextError

from charmhelpers.core.host import (
    get_bond_master,
    is_phy_iface,
    list_nics,
    get_nic_hwaddr,
    mkdir,
    write_file,
    pwgen,
    lsb_release,
)
from charmhelpers.contrib.hahelpers.cluster import (
    determine_apache_port,
    determine_api_port,
    https,
    is_clustered,
)
from charmhelpers.contrib.hahelpers.apache import (
    get_cert,
    get_ca_cert,
    install_ca_cert,
)
from charmhelpers.contrib.openstack.neutron import (
    neutron_plugin_attribute,
    parse_data_port_mappings,
)
from charmhelpers.contrib.openstack.ip import (
    resolve_address,
    INTERNAL,
)
from charmhelpers.contrib.network.ip import (
    get_address_in_network,
    get_ipv4_addr,
    get_ipv6_addr,
    get_netmask_for_address,
    format_ipv6_addr,
    is_address_in_network,
    is_bridge_member,
)
from charmhelpers.contrib.openstack.utils import (
    config_flags_parser,
    get_host_ip,
    enable_memcache,
)
from charmhelpers.core.unitdata import kv

try:
    import psutil
except ImportError:
    apt_install('python-psutil', fatal=True)
    import psutil

CA_CERT_PATH = '/usr/local/share/ca-certificates/keystone_juju_ca_cert.crt'
ADDRESS_TYPES = ['admin', 'internal', 'public']


def ensure_packages(packages):
    """Install but do not upgrade required plugin packages."""
    required = filter_installed_packages(packages)
    if required:
        apt_install(required, fatal=True)


def context_complete(ctxt):
    _missing = []
    for k, v in six.iteritems(ctxt):
        if v is None or v == '':
            _missing.append(k)

    if _missing:
        log('Missing required data: %s' % ' '.join(_missing), level=INFO)
        return False

    return True


class OSContextGenerator(object):
    """Base class for all context generators."""
    interfaces = []
    related = False
    complete = False
    missing_data = []

    def __call__(self):
        raise NotImplementedError

    def context_complete(self, ctxt):
        """Check for missing data for the required context data.
        Set self.missing_data if it exists and return False.
        Set self.complete if no missing data and return True.
        """
        # Fresh start
        self.complete = False
        self.missing_data = []
        for k, v in six.iteritems(ctxt):
            if v is None or v == '':
                if k not in self.missing_data:
                    self.missing_data.append(k)

        if self.missing_data:
            self.complete = False
            log('Missing required data: %s' % ' '.join(self.missing_data), level=INFO)
        else:
            self.complete = True
        return self.complete

    def get_related(self):
        """Check if any of the context interfaces have relation ids.
        Set self.related and return True if one of the interfaces
        has relation ids.
        """
        # Fresh start
        self.related = False
        try:
            for interface in self.interfaces:
                if relation_ids(interface):
                    self.related = True
            return self.related
        except AttributeError as e:
            log("{} {}"
                "".format(self, e), 'INFO')
            return self.related


class SharedDBContext(OSContextGenerator):
    interfaces = ['shared-db']

    def __init__(self,
                 database=None, user=None, relation_prefix=None, ssl_dir=None):
        """Allows inspecting relation for settings prefixed with
        relation_prefix. This is useful for parsing access for multiple
        databases returned via the shared-db interface (eg, nova_password,
        quantum_password)
        """
        self.relation_prefix = relation_prefix
        self.database = database
        self.user = user
        self.ssl_dir = ssl_dir
        self.rel_name = self.interfaces[0]

    def __call__(self):
        self.database = self.database or config('database')
        self.user = self.user or config('database-user')
        if None in [self.database, self.user]:
            log("Could not generate shared_db context. Missing required charm "
                "config options. (database name and user)", level=ERROR)
            raise OSContextError

        ctxt = {}

        # NOTE(jamespage) if mysql charm provides a network upon which
        # access to the database should be made, reconfigure relation
        # with the service units local address and defer execution
        access_network = relation_get('access-network')
        if access_network is not None:
            if self.relation_prefix is not None:
                hostname_key = "{}_hostname".format(self.relation_prefix)
            else:
                hostname_key = "hostname"
            access_hostname = get_address_in_network(access_network,
                                                     unit_get('private-address'))
            set_hostname = relation_get(attribute=hostname_key,
                                        unit=local_unit())
            if set_hostname != access_hostname:
                relation_set(relation_settings={hostname_key: access_hostname})
                return None  # Defer any further hook execution for now....

        password_setting = 'password'
        if self.relation_prefix:
            password_setting = self.relation_prefix + '_password'

        for rid in relation_ids(self.interfaces[0]):
            self.related = True
            for unit in related_units(rid):
                rdata = relation_get(rid=rid, unit=unit)
                host = rdata.get('db_host')
                host = format_ipv6_addr(host) or host
                ctxt = {
                    'database_host': host,
                    'database': self.database,
                    'database_user': self.user,
                    'database_password': rdata.get(password_setting),
                    'database_type': 'mysql'
                }
                if self.context_complete(ctxt):
                    db_ssl(rdata, ctxt, self.ssl_dir)
                    return ctxt
        return {}


class PostgresqlDBContext(OSContextGenerator):
    interfaces = ['pgsql-db']

    def __init__(self, database=None):
        self.database = database

    def __call__(self):
        self.database = self.database or config('database')
        if self.database is None:
            log('Could not generate postgresql_db context. Missing required '
                'charm config options. (database name)', level=ERROR)
            raise OSContextError

        ctxt = {}
        for rid in relation_ids(self.interfaces[0]):
            self.related = True
            for unit in related_units(rid):
                rel_host = relation_get('host', rid=rid, unit=unit)
                rel_user = relation_get('user', rid=rid, unit=unit)
                rel_passwd = relation_get('password', rid=rid, unit=unit)
                ctxt = {'database_host': rel_host,
                        'database': self.database,
                        'database_user': rel_user,
                        'database_password': rel_passwd,
                        'database_type': 'postgresql'}
                if self.context_complete(ctxt):
                    return ctxt

        return {}


def db_ssl(rdata, ctxt, ssl_dir):
    if 'ssl_ca' in rdata and ssl_dir:
        ca_path = os.path.join(ssl_dir, 'db-client.ca')
        with open(ca_path, 'w') as fh:
            fh.write(b64decode(rdata['ssl_ca']))

        ctxt['database_ssl_ca'] = ca_path
    elif 'ssl_ca' in rdata:
        log("Charm not setup for ssl support but ssl ca found", level=INFO)
        return ctxt

    if 'ssl_cert' in rdata:
        cert_path = os.path.join(
            ssl_dir, 'db-client.cert')
        if not os.path.exists(cert_path):
            log("Waiting 1m for ssl client cert validity", level=INFO)
            time.sleep(60)

        with open(cert_path, 'w') as fh:
            fh.write(b64decode(rdata['ssl_cert']))

        ctxt['database_ssl_cert'] = cert_path
        key_path = os.path.join(ssl_dir, 'db-client.key')
        with open(key_path, 'w') as fh:
            fh.write(b64decode(rdata['ssl_key']))

        ctxt['database_ssl_key'] = key_path

    return ctxt


class IdentityServiceContext(OSContextGenerator):

    def __init__(self, service=None, service_user=None, rel_name='identity-service'):
        self.service = service
        self.service_user = service_user
        self.rel_name = rel_name
        self.interfaces = [self.rel_name]

    def __call__(self):
        log('Generating template context for ' + self.rel_name, level=DEBUG)
        ctxt = {}

        if self.service and self.service_user:
            # This is required for pki token signing if we don't want /tmp to
            # be used.
            cachedir = '/var/cache/%s' % (self.service)
            if not os.path.isdir(cachedir):
                log("Creating service cache dir %s" % (cachedir), level=DEBUG)
                mkdir(path=cachedir, owner=self.service_user,
                      group=self.service_user, perms=0o700)

            ctxt['signing_dir'] = cachedir

        for rid in relation_ids(self.rel_name):
            self.related = True
            for unit in related_units(rid):
                rdata = relation_get(rid=rid, unit=unit)
                serv_host = rdata.get('service_host')
                serv_host = format_ipv6_addr(serv_host) or serv_host
                auth_host = rdata.get('auth_host')
                auth_host = format_ipv6_addr(auth_host) or auth_host
                svc_protocol = rdata.get('service_protocol') or 'http'
                auth_protocol = rdata.get('auth_protocol') or 'http'
                api_version = rdata.get('api_version') or '2.0'
                ctxt.update({'service_port': rdata.get('service_port'),
                             'service_host': serv_host,
                             'auth_host': auth_host,
                             'auth_port': rdata.get('auth_port'),
                             'admin_tenant_name': rdata.get('service_tenant'),
                             'admin_user': rdata.get('service_username'),
                             'admin_password': rdata.get('service_password'),
                             'service_protocol': svc_protocol,
                             'auth_protocol': auth_protocol,
                             'api_version': api_version})

                if float(api_version) > 2:
                    ctxt.update({'admin_domain_name':
                                 rdata.get('service_domain')})

                if self.context_complete(ctxt):
                    # NOTE(jamespage) this is required for >= icehouse
                    # so a missing value just indicates keystone needs
                    # upgrading
                    ctxt['admin_tenant_id'] = rdata.get('service_tenant_id')
                    return ctxt

        return {}


class AMQPContext(OSContextGenerator):

    def __init__(self, ssl_dir=None, rel_name='amqp', relation_prefix=None):
        self.ssl_dir = ssl_dir
        self.rel_name = rel_name
        self.relation_prefix = relation_prefix
        self.interfaces = [rel_name]

    def __call__(self):
        log('Generating template context for amqp', level=DEBUG)
        conf = config()
        if self.relation_prefix:
            user_setting = '%s-rabbit-user' % (self.relation_prefix)
            vhost_setting = '%s-rabbit-vhost' % (self.relation_prefix)
        else:
            user_setting = 'rabbit-user'
            vhost_setting = 'rabbit-vhost'

        try:
            username = conf[user_setting]
            vhost = conf[vhost_setting]
        except KeyError as e:
            log('Could not generate shared_db context. Missing required charm '
                'config options: %s.' % e, level=ERROR)
            raise OSContextError

        ctxt = {}
        for rid in relation_ids(self.rel_name):
            ha_vip_only = False
            self.related = True
            for unit in related_units(rid):
                if relation_get('clustered', rid=rid, unit=unit):
                    ctxt['clustered'] = True
                    vip = relation_get('vip', rid=rid, unit=unit)
                    vip = format_ipv6_addr(vip) or vip
                    ctxt['rabbitmq_host'] = vip
                else:
                    host = relation_get('private-address', rid=rid, unit=unit)
                    host = format_ipv6_addr(host) or host
                    ctxt['rabbitmq_host'] = host

                ctxt.update({
                    'rabbitmq_user': username,
                    'rabbitmq_password': relation_get('password', rid=rid,
                                                      unit=unit),
                    'rabbitmq_virtual_host': vhost,
                })

                ssl_port = relation_get('ssl_port', rid=rid, unit=unit)
                if ssl_port:
                    ctxt['rabbit_ssl_port'] = ssl_port

                ssl_ca = relation_get('ssl_ca', rid=rid, unit=unit)
                if ssl_ca:
                    ctxt['rabbit_ssl_ca'] = ssl_ca

                if relation_get('ha_queues', rid=rid, unit=unit) is not None:
                    ctxt['rabbitmq_ha_queues'] = True

                ha_vip_only = relation_get('ha-vip-only',
                                           rid=rid, unit=unit) is not None

                if self.context_complete(ctxt):
                    if 'rabbit_ssl_ca' in ctxt:
                        if not self.ssl_dir:
                            log("Charm not setup for ssl support but ssl ca "
                                "found", level=INFO)
                            break

                        ca_path = os.path.join(
                            self.ssl_dir, 'rabbit-client-ca.pem')
                        with open(ca_path, 'w') as fh:
                            fh.write(b64decode(ctxt['rabbit_ssl_ca']))
                            ctxt['rabbit_ssl_ca'] = ca_path

                    # Sufficient information found = break out!
                    break

            # Used for active/active rabbitmq >= grizzly
            if (('clustered' not in ctxt or ha_vip_only) and
                    len(related_units(rid)) > 1):
                rabbitmq_hosts = []
                for unit in related_units(rid):
                    host = relation_get('private-address', rid=rid, unit=unit)
                    host = format_ipv6_addr(host) or host
                    rabbitmq_hosts.append(host)

                ctxt['rabbitmq_hosts'] = ','.join(sorted(rabbitmq_hosts))

        oslo_messaging_flags = conf.get('oslo-messaging-flags', None)
        if oslo_messaging_flags:
            ctxt['oslo_messaging_flags'] = config_flags_parser(
                oslo_messaging_flags)

        if not self.complete:
            return {}

        return ctxt


class CephContext(OSContextGenerator):
    """Generates context for /etc/ceph/ceph.conf templates."""
    interfaces = ['ceph']

    def __call__(self):
        if not relation_ids('ceph'):
            return {}

        log('Generating template context for ceph', level=DEBUG)
        mon_hosts = []
        ctxt = {
            'use_syslog': str(config('use-syslog')).lower()
        }
        for rid in relation_ids('ceph'):
            for unit in related_units(rid):
                if not ctxt.get('auth'):
                    ctxt['auth'] = relation_get('auth', rid=rid, unit=unit)
                if not ctxt.get('key'):
                    ctxt['key'] = relation_get('key', rid=rid, unit=unit)
                ceph_pub_addr = relation_get('ceph-public-address', rid=rid,
                                             unit=unit)
                unit_priv_addr = relation_get('private-address', rid=rid,
                                              unit=unit)
                ceph_addr = ceph_pub_addr or unit_priv_addr
                ceph_addr = format_ipv6_addr(ceph_addr) or ceph_addr
                mon_hosts.append(ceph_addr)

        ctxt['mon_hosts'] = ' '.join(sorted(mon_hosts))

        if not os.path.isdir('/etc/ceph'):
            os.mkdir('/etc/ceph')

        if not self.context_complete(ctxt):
            return {}

        ensure_packages(['ceph-common'])
        return ctxt


class HAProxyContext(OSContextGenerator):
    """Provides half a context for the haproxy template, which describes
    all peers to be included in the cluster.  Each charm needs to include
    its own context generator that describes the port mapping.
    """
    interfaces = ['cluster']

    def __init__(self, singlenode_mode=False):
        self.singlenode_mode = singlenode_mode

    def __call__(self):
        if not relation_ids('cluster') and not self.singlenode_mode:
            return {}

        if config('prefer-ipv6'):
            addr = get_ipv6_addr(exc_list=[config('vip')])[0]
        else:
            addr = get_host_ip(unit_get('private-address'))

        l_unit = local_unit().replace('/', '-')
        cluster_hosts = {}

        # NOTE(jamespage): build out map of configured network endpoints
        # and associated backends
        for addr_type in ADDRESS_TYPES:
            cfg_opt = 'os-{}-network'.format(addr_type)
            laddr = get_address_in_network(config(cfg_opt))
            if laddr:
                netmask = get_netmask_for_address(laddr)
                cluster_hosts[laddr] = {'network': "{}/{}".format(laddr,
                                                                  netmask),
                                        'backends': {l_unit: laddr}}
                for rid in relation_ids('cluster'):
                    for unit in related_units(rid):
                        _laddr = relation_get('{}-address'.format(addr_type),
                                              rid=rid, unit=unit)
                        if _laddr:
                            _unit = unit.replace('/', '-')
                            cluster_hosts[laddr]['backends'][_unit] = _laddr

        # NOTE(jamespage) add backend based on private address - this
        # with either be the only backend or the fallback if no acls
        # match in the frontend
        cluster_hosts[addr] = {}
        netmask = get_netmask_for_address(addr)
        cluster_hosts[addr] = {'network': "{}/{}".format(addr, netmask),
                               'backends': {l_unit: addr}}
        for rid in relation_ids('cluster'):
            for unit in related_units(rid):
                _laddr = relation_get('private-address',
                                      rid=rid, unit=unit)
                if _laddr:
                    _unit = unit.replace('/', '-')
                    cluster_hosts[addr]['backends'][_unit] = _laddr

        ctxt = {
            'frontends': cluster_hosts,
            'default_backend': addr
        }

        if config('haproxy-server-timeout'):
            ctxt['haproxy_server_timeout'] = config('haproxy-server-timeout')

        if config('haproxy-client-timeout'):
            ctxt['haproxy_client_timeout'] = config('haproxy-client-timeout')

        if config('haproxy-queue-timeout'):
            ctxt['haproxy_queue_timeout'] = config('haproxy-queue-timeout')

        if config('haproxy-connect-timeout'):
            ctxt['haproxy_connect_timeout'] = config('haproxy-connect-timeout')

        if config('prefer-ipv6'):
            ctxt['ipv6'] = True
            ctxt['local_host'] = 'ip6-localhost'
            ctxt['haproxy_host'] = '::'
        else:
            ctxt['local_host'] = '127.0.0.1'
            ctxt['haproxy_host'] = '0.0.0.0'

        ctxt['stat_port'] = '8888'

        db = kv()
        ctxt['stat_password'] = db.get('stat-password')
        if not ctxt['stat_password']:
            ctxt['stat_password'] = db.set('stat-password',
                                           pwgen(32))
            db.flush()

        for frontend in cluster_hosts:
            if (len(cluster_hosts[frontend]['backends']) > 1 or
                    self.singlenode_mode):
                # Enable haproxy when we have enough peers.
                log('Ensuring haproxy enabled in /etc/default/haproxy.',
                    level=DEBUG)
                with open('/etc/default/haproxy', 'w') as out:
                    out.write('ENABLED=1\n')

                return ctxt

        log('HAProxy context is incomplete, this unit has no peers.',
            level=INFO)
        return {}


class ImageServiceContext(OSContextGenerator):
    interfaces = ['image-service']

    def __call__(self):
        """Obtains the glance API server from the image-service relation.
        Useful in nova and cinder (currently).
        """
        log('Generating template context for image-service.', level=DEBUG)
        rids = relation_ids('image-service')
        if not rids:
            return {}

        for rid in rids:
            for unit in related_units(rid):
                api_server = relation_get('glance-api-server',
                                          rid=rid, unit=unit)
                if api_server:
                    return {'glance_api_servers': api_server}

        log("ImageService context is incomplete. Missing required relation "
            "data.", level=INFO)
        return {}


class ApacheSSLContext(OSContextGenerator):
    """Generates a context for an apache vhost configuration that configures
    HTTPS reverse proxying for one or many endpoints.  Generated context
    looks something like::

        {
            'namespace': 'cinder',
            'private_address': 'iscsi.mycinderhost.com',
            'endpoints': [(8776, 8766), (8777, 8767)]
        }

    The endpoints list consists of a tuples mapping external ports
    to internal ports.
    """
    interfaces = ['https']

    # charms should inherit this context and set external ports
    # and service namespace accordingly.
    external_ports = []
    service_namespace = None

    def enable_modules(self):
        cmd = ['a2enmod', 'ssl', 'proxy', 'proxy_http', 'headers']
        check_call(cmd)

    def configure_cert(self, cn=None):
        ssl_dir = os.path.join('/etc/apache2/ssl/', self.service_namespace)
        mkdir(path=ssl_dir)
        cert, key = get_cert(cn)
        if cn:
            cert_filename = 'cert_{}'.format(cn)
            key_filename = 'key_{}'.format(cn)
        else:
            cert_filename = 'cert'
            key_filename = 'key'

        write_file(path=os.path.join(ssl_dir, cert_filename),
                   content=b64decode(cert))
        write_file(path=os.path.join(ssl_dir, key_filename),
                   content=b64decode(key))

    def configure_ca(self):
        ca_cert = get_ca_cert()
        if ca_cert:
            install_ca_cert(b64decode(ca_cert))

    def canonical_names(self):
        """Figure out which canonical names clients will access this service.
        """
        cns = []
        for r_id in relation_ids('identity-service'):
            for unit in related_units(r_id):
                rdata = relation_get(rid=r_id, unit=unit)
                for k in rdata:
                    if k.startswith('ssl_key_'):
                        cns.append(k.lstrip('ssl_key_'))

        return sorted(list(set(cns)))

    def get_network_addresses(self):
        """For each network configured, return corresponding address and vip
           (if available).

        Returns a list of tuples of the form:

            [(address_in_net_a, vip_in_net_a),
             (address_in_net_b, vip_in_net_b),
             ...]

            or, if no vip(s) available:

            [(address_in_net_a, address_in_net_a),
             (address_in_net_b, address_in_net_b),
             ...]
        """
        addresses = []
        if config('vip'):
            vips = config('vip').split()
        else:
            vips = []

        for net_type in ['os-internal-network', 'os-admin-network',
                         'os-public-network']:
            addr = get_address_in_network(config(net_type),
                                          unit_get('private-address'))
            if len(vips) > 1 and is_clustered():
                if not config(net_type):
                    log("Multiple networks configured but net_type "
                        "is None (%s)." % net_type, level=WARNING)
                    continue

                for vip in vips:
                    if is_address_in_network(config(net_type), vip):
                        addresses.append((addr, vip))
                        break

            elif is_clustered() and config('vip'):
                addresses.append((addr, config('vip')))
            else:
                addresses.append((addr, addr))

        return sorted(addresses)

    def __call__(self):
        if isinstance(self.external_ports, six.string_types):
            self.external_ports = [self.external_ports]

        if not self.external_ports or not https():
            return {}

        self.configure_ca()
        self.enable_modules()

        ctxt = {'namespace': self.service_namespace,
                'endpoints': [],
                'ext_ports': []}

        cns = self.canonical_names()
        if cns:
            for cn in cns:
                self.configure_cert(cn)
        else:
            # Expect cert/key provided in config (currently assumed that ca
            # uses ip for cn)
            cn = resolve_address(endpoint_type=INTERNAL)
            self.configure_cert(cn)

        addresses = self.get_network_addresses()
        for address, endpoint in sorted(set(addresses)):
            for api_port in self.external_ports:
                ext_port = determine_apache_port(api_port,
                                                 singlenode_mode=True)
                int_port = determine_api_port(api_port, singlenode_mode=True)
                portmap = (address, endpoint, int(ext_port), int(int_port))
                ctxt['endpoints'].append(portmap)
                ctxt['ext_ports'].append(int(ext_port))

        ctxt['ext_ports'] = sorted(list(set(ctxt['ext_ports'])))
        return ctxt


class NeutronContext(OSContextGenerator):
    interfaces = []

    @property
    def plugin(self):
        return None

    @property
    def network_manager(self):
        return None

    @property
    def packages(self):
        return neutron_plugin_attribute(self.plugin, 'packages',
                                        self.network_manager)

    @property
    def neutron_security_groups(self):
        return None

    def _ensure_packages(self):
        for pkgs in self.packages:
            ensure_packages(pkgs)

    def _save_flag_file(self):
        if self.network_manager == 'quantum':
            _file = '/etc/nova/quantum_plugin.conf'
        else:
            _file = '/etc/nova/neutron_plugin.conf'

        with open(_file, 'wb') as out:
            out.write(self.plugin + '\n')

    def ovs_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        config = neutron_plugin_attribute(self.plugin, 'config',
                                          self.network_manager)
        ovs_ctxt = {'core_plugin': driver,
                    'neutron_plugin': 'ovs',
                    'neutron_security_groups': self.neutron_security_groups,
                    'local_ip': unit_private_ip(),
                    'config': config}

        return ovs_ctxt

    def nuage_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        config = neutron_plugin_attribute(self.plugin, 'config',
                                          self.network_manager)
        nuage_ctxt = {'core_plugin': driver,
                      'neutron_plugin': 'vsp',
                      'neutron_security_groups': self.neutron_security_groups,
                      'local_ip': unit_private_ip(),
                      'config': config}

        return nuage_ctxt

    def nvp_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        config = neutron_plugin_attribute(self.plugin, 'config',
                                          self.network_manager)
        nvp_ctxt = {'core_plugin': driver,
                    'neutron_plugin': 'nvp',
                    'neutron_security_groups': self.neutron_security_groups,
                    'local_ip': unit_private_ip(),
                    'config': config}

        return nvp_ctxt

    def n1kv_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        n1kv_config = neutron_plugin_attribute(self.plugin, 'config',
                                               self.network_manager)
        n1kv_user_config_flags = config('n1kv-config-flags')
        restrict_policy_profiles = config('n1kv-restrict-policy-profiles')
        n1kv_ctxt = {'core_plugin': driver,
                     'neutron_plugin': 'n1kv',
                     'neutron_security_groups': self.neutron_security_groups,
                     'local_ip': unit_private_ip(),
                     'config': n1kv_config,
                     'vsm_ip': config('n1kv-vsm-ip'),
                     'vsm_username': config('n1kv-vsm-username'),
                     'vsm_password': config('n1kv-vsm-password'),
                     'restrict_policy_profiles': restrict_policy_profiles}

        if n1kv_user_config_flags:
            flags = config_flags_parser(n1kv_user_config_flags)
            n1kv_ctxt['user_config_flags'] = flags

        return n1kv_ctxt

    def calico_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        config = neutron_plugin_attribute(self.plugin, 'config',
                                          self.network_manager)
        calico_ctxt = {'core_plugin': driver,
                       'neutron_plugin': 'Calico',
                       'neutron_security_groups': self.neutron_security_groups,
                       'local_ip': unit_private_ip(),
                       'config': config}

        return calico_ctxt

    def neutron_ctxt(self):
        if https():
            proto = 'https'
        else:
            proto = 'http'

        if is_clustered():
            host = config('vip')
        else:
            host = unit_get('private-address')

        ctxt = {'network_manager': self.network_manager,
                'neutron_url': '%s://%s:%s' % (proto, host, '9696')}
        return ctxt

    def pg_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        config = neutron_plugin_attribute(self.plugin, 'config',
                                          self.network_manager)
        ovs_ctxt = {'core_plugin': driver,
                    'neutron_plugin': 'plumgrid',
                    'neutron_security_groups': self.neutron_security_groups,
                    'local_ip': unit_private_ip(),
                    'config': config}
        return ovs_ctxt

    def midonet_ctxt(self):
        driver = neutron_plugin_attribute(self.plugin, 'driver',
                                          self.network_manager)
        midonet_config = neutron_plugin_attribute(self.plugin, 'config',
                                                  self.network_manager)
        mido_ctxt = {'core_plugin': driver,
                     'neutron_plugin': 'midonet',
                     'neutron_security_groups': self.neutron_security_groups,
                     'local_ip': unit_private_ip(),
                     'config': midonet_config}

        return mido_ctxt

    def __call__(self):
        if self.network_manager not in ['quantum', 'neutron']:
            return {}

        if not self.plugin:
            return {}

        ctxt = self.neutron_ctxt()

        if self.plugin == 'ovs':
            ctxt.update(self.ovs_ctxt())
        elif self.plugin in ['nvp', 'nsx']:
            ctxt.update(self.nvp_ctxt())
        elif self.plugin == 'n1kv':
            ctxt.update(self.n1kv_ctxt())
        elif self.plugin == 'Calico':
            ctxt.update(self.calico_ctxt())
        elif self.plugin == 'vsp':
            ctxt.update(self.nuage_ctxt())
        elif self.plugin == 'plumgrid':
            ctxt.update(self.pg_ctxt())
        elif self.plugin == 'midonet':
            ctxt.update(self.midonet_ctxt())

        alchemy_flags = config('neutron-alchemy-flags')
        if alchemy_flags:
            flags = config_flags_parser(alchemy_flags)
            ctxt['neutron_alchemy_flags'] = flags

        self._save_flag_file()
        return ctxt


class NeutronPortContext(OSContextGenerator):

    def resolve_ports(self, ports):
        """Resolve NICs not yet bound to bridge(s)

        If hwaddress provided then returns resolved hwaddress otherwise NIC.
        """
        if not ports:
            return None

        hwaddr_to_nic = {}
        hwaddr_to_ip = {}
        for nic in list_nics():
            # Ignore virtual interfaces (bond masters will be identified from
            # their slaves)
            if not is_phy_iface(nic):
                continue

            _nic = get_bond_master(nic)
            if _nic:
                log("Replacing iface '%s' with bond master '%s'" % (nic, _nic),
                    level=DEBUG)
                nic = _nic

            hwaddr = get_nic_hwaddr(nic)
            hwaddr_to_nic[hwaddr] = nic
            addresses = get_ipv4_addr(nic, fatal=False)
            addresses += get_ipv6_addr(iface=nic, fatal=False)
            hwaddr_to_ip[hwaddr] = addresses

        resolved = []
        mac_regex = re.compile(r'([0-9A-F]{2}[:-]){5}([0-9A-F]{2})', re.I)
        for entry in ports:
            if re.match(mac_regex, entry):
                # NIC is in known NICs and does NOT hace an IP address
                if entry in hwaddr_to_nic and not hwaddr_to_ip[entry]:
                    # If the nic is part of a bridge then don't use it
                    if is_bridge_member(hwaddr_to_nic[entry]):
                        continue

                    # Entry is a MAC address for a valid interface that doesn't
                    # have an IP address assigned yet.
                    resolved.append(hwaddr_to_nic[entry])
            else:
                # If the passed entry is not a MAC address, assume it's a valid
                # interface, and that the user put it there on purpose (we can
                # trust it to be the real external network).
                resolved.append(entry)

        # Ensure no duplicates
        return list(set(resolved))


class OSConfigFlagContext(OSContextGenerator):
    """Provides support for user-defined config flags.

    Users can define a comma-seperated list of key=value pairs
    in the charm configuration and apply them at any point in
    any file by using a template flag.

    Sometimes users might want config flags inserted within a
    specific section so this class allows users to specify the
    template flag name, allowing for multiple template flags
    (sections) within the same context.

    NOTE: the value of config-flags may be a comma-separated list of
          key=value pairs and some Openstack config files support
          comma-separated lists as values.
    """

    def __init__(self, charm_flag='config-flags',
                 template_flag='user_config_flags'):
        """
        :param charm_flag: config flags in charm configuration.
        :param template_flag: insert point for user-defined flags in template
                              file.
        """
        super(OSConfigFlagContext, self).__init__()
        self._charm_flag = charm_flag
        self._template_flag = template_flag

    def __call__(self):
        config_flags = config(self._charm_flag)
        if not config_flags:
            return {}

        return {self._template_flag:
                config_flags_parser(config_flags)}


class LibvirtConfigFlagsContext(OSContextGenerator):
    """
    This context provides support for extending
    the libvirt section through user-defined flags.
    """
    def __call__(self):
        ctxt = {}
        libvirt_flags = config('libvirt-flags')
        if libvirt_flags:
            ctxt['libvirt_flags'] = config_flags_parser(
                libvirt_flags)
        return ctxt


class SubordinateConfigContext(OSContextGenerator):

    """
    Responsible for inspecting relations to subordinates that
    may be exporting required config via a json blob.

    The subordinate interface allows subordinates to export their
    configuration requirements to the principle for multiple config
    files and multiple serivces.  Ie, a subordinate that has interfaces
    to both glance and nova may export to following yaml blob as json::

        glance:
            /etc/glance/glance-api.conf:
                sections:
                    DEFAULT:
                        - [key1, value1]
            /etc/glance/glance-registry.conf:
                    MYSECTION:
                        - [key2, value2]
        nova:
            /etc/nova/nova.conf:
                sections:
                    DEFAULT:
                        - [key3, value3]


    It is then up to the principle charms to subscribe this context to
    the service+config file it is interestd in.  Configuration data will
    be available in the template context, in glance's case, as::

        ctxt = {
            ... other context ...
            'subordinate_configuration': {
                'DEFAULT': {
                    'key1': 'value1',
                },
                'MYSECTION': {
                    'key2': 'value2',
                },
            }
        }
    """

    def __init__(self, service, config_file, interface):
        """
        :param service     : Service name key to query in any subordinate
                             data found
        :param config_file : Service's config file to query sections
        :param interface   : Subordinate interface to inspect
        """
        self.config_file = config_file
        if isinstance(service, list):
            self.services = service
        else:
            self.services = [service]
        if isinstance(interface, list):
            self.interfaces = interface
        else:
            self.interfaces = [interface]

    def __call__(self):
        ctxt = {'sections': {}}
        rids = []
        for interface in self.interfaces:
            rids.extend(relation_ids(interface))
        for rid in rids:
            for unit in related_units(rid):
                sub_config = relation_get('subordinate_configuration',
                                          rid=rid, unit=unit)
                if sub_config and sub_config != '':
                    try:
                        sub_config = json.loads(sub_config)
                    except:
                        log('Could not parse JSON from '
                            'subordinate_configuration setting from %s'
                            % rid, level=ERROR)
                        continue

                    for service in self.services:
                        if service not in sub_config:
                            log('Found subordinate_configuration on %s but it '
                                'contained nothing for %s service'
                                % (rid, service), level=INFO)
                            continue

                        sub_config = sub_config[service]
                        if self.config_file not in sub_config:
                            log('Found subordinate_configuration on %s but it '
                                'contained nothing for %s'
                                % (rid, self.config_file), level=INFO)
                            continue

                        sub_config = sub_config[self.config_file]
                        for k, v in six.iteritems(sub_config):
                            if k == 'sections':
                                for section, config_list in six.iteritems(v):
                                    log("adding section '%s'" % (section),
                                        level=DEBUG)
                                    if ctxt[k].get(section):
                                        ctxt[k][section].extend(config_list)
                                    else:
                                        ctxt[k][section] = config_list
                            else:
                                ctxt[k] = v
        log("%d section(s) found" % (len(ctxt['sections'])), level=DEBUG)
        return ctxt


class LogLevelContext(OSContextGenerator):

    def __call__(self):
        ctxt = {}
        ctxt['debug'] = \
            False if config('debug') is None else config('debug')
        ctxt['verbose'] = \
            False if config('verbose') is None else config('verbose')

        return ctxt


class SyslogContext(OSContextGenerator):

    def __call__(self):
        ctxt = {'use_syslog': config('use-syslog')}
        return ctxt


class BindHostContext(OSContextGenerator):

    def __call__(self):
        if config('prefer-ipv6'):
            return {'bind_host': '::'}
        else:
            return {'bind_host': '0.0.0.0'}


class WorkerConfigContext(OSContextGenerator):

    @property
    def num_cpus(self):
        # NOTE: use cpu_count if present (16.04 support)
        if hasattr(psutil, 'cpu_count'):
            return psutil.cpu_count()
        else:
            return psutil.NUM_CPUS

    def __call__(self):
        multiplier = config('worker-multiplier') or 0
        count = int(self.num_cpus * multiplier)
        if multiplier > 0 and count == 0:
            count = 1
        ctxt = {"workers": count}
        return ctxt


class ZeroMQContext(OSContextGenerator):
    interfaces = ['zeromq-configuration']

    def __call__(self):
        ctxt = {}
        if is_relation_made('zeromq-configuration', 'host'):
            for rid in relation_ids('zeromq-configuration'):
                    for unit in related_units(rid):
                        ctxt['zmq_nonce'] = relation_get('nonce', unit, rid)
                        ctxt['zmq_host'] = relation_get('host', unit, rid)
                        ctxt['zmq_redis_address'] = relation_get(
                            'zmq_redis_address', unit, rid)

        return ctxt


class NotificationDriverContext(OSContextGenerator):

    def __init__(self, zmq_relation='zeromq-configuration',
                 amqp_relation='amqp'):
        """
        :param zmq_relation: Name of Zeromq relation to check
        """
        self.zmq_relation = zmq_relation
        self.amqp_relation = amqp_relation

    def __call__(self):
        ctxt = {'notifications': 'False'}
        if is_relation_made(self.amqp_relation):
            ctxt['notifications'] = "True"

        return ctxt


class SysctlContext(OSContextGenerator):
    """This context check if the 'sysctl' option exists on configuration
    then creates a file with the loaded contents"""
    def __call__(self):
        sysctl_dict = config('sysctl')
        if sysctl_dict:
            sysctl_create(sysctl_dict,
                          '/etc/sysctl.d/50-{0}.conf'.format(charm_name()))
        return {'sysctl': sysctl_dict}


class NeutronAPIContext(OSContextGenerator):
    '''
    Inspects current neutron-plugin-api relation for neutron settings. Return
    defaults if it is not present.
    '''
    interfaces = ['neutron-plugin-api']

    def __call__(self):
        self.neutron_defaults = {
            'l2_population': {
                'rel_key': 'l2-population',
                'default': False,
            },
            'overlay_network_type': {
                'rel_key': 'overlay-network-type',
                'default': 'gre',
            },
            'neutron_security_groups': {
                'rel_key': 'neutron-security-groups',
                'default': False,
            },
            'network_device_mtu': {
                'rel_key': 'network-device-mtu',
                'default': None,
            },
            'enable_dvr': {
                'rel_key': 'enable-dvr',
                'default': False,
            },
            'enable_l3ha': {
                'rel_key': 'enable-l3ha',
                'default': False,
            },
        }
        ctxt = self.get_neutron_options({})
        for rid in relation_ids('neutron-plugin-api'):
            for unit in related_units(rid):
                rdata = relation_get(rid=rid, unit=unit)
                if 'l2-population' in rdata:
                    ctxt.update(self.get_neutron_options(rdata))

        return ctxt

    def get_neutron_options(self, rdata):
        settings = {}
        for nkey in self.neutron_defaults.keys():
            defv = self.neutron_defaults[nkey]['default']
            rkey = self.neutron_defaults[nkey]['rel_key']
            if rkey in rdata.keys():
                if type(defv) is bool:
                    settings[nkey] = bool_from_string(rdata[rkey])
                else:
                    settings[nkey] = rdata[rkey]
            else:
                settings[nkey] = defv
        return settings


class ExternalPortContext(NeutronPortContext):

    def __call__(self):
        ctxt = {}
        ports = config('ext-port')
        if ports:
            ports = [p.strip() for p in ports.split()]
            ports = self.resolve_ports(ports)
            if ports:
                ctxt = {"ext_port": ports[0]}
                napi_settings = NeutronAPIContext()()
                mtu = napi_settings.get('network_device_mtu')
                if mtu:
                    ctxt['ext_port_mtu'] = mtu

        return ctxt


class DataPortContext(NeutronPortContext):

    def __call__(self):
        ports = config('data-port')
        if ports:
            # Map of {port/mac:bridge}
            portmap = parse_data_port_mappings(ports)
            ports = portmap.keys()
            # Resolve provided ports or mac addresses and filter out those
            # already attached to a bridge.
            resolved = self.resolve_ports(ports)
            # FIXME: is this necessary?
            normalized = {get_nic_hwaddr(port): port for port in resolved
                          if port not in ports}
            normalized.update({port: port for port in resolved
                               if port in ports})
            if resolved:
                return {normalized[port]: bridge for port, bridge in
                        six.iteritems(portmap) if port in normalized.keys()}

        return None


class PhyNICMTUContext(DataPortContext):

    def __call__(self):
        ctxt = {}
        mappings = super(PhyNICMTUContext, self).__call__()
        if mappings and mappings.keys():
            ports = sorted(mappings.keys())
            napi_settings = NeutronAPIContext()()
            mtu = napi_settings.get('network_device_mtu')
            all_ports = set()
            # If any of ports is a vlan device, its underlying device must have
            # mtu applied first.
            for port in ports:
                for lport in glob.glob("/sys/class/net/%s/lower_*" % port):
                    lport = os.path.basename(lport)
                    all_ports.add(lport.split('_')[1])

            all_ports = list(all_ports)
            all_ports.extend(ports)
            if mtu:
                ctxt["devs"] = '\\n'.join(all_ports)
                ctxt['mtu'] = mtu

        return ctxt


class NetworkServiceContext(OSContextGenerator):

    def __init__(self, rel_name='quantum-network-service'):
        self.rel_name = rel_name
        self.interfaces = [rel_name]

    def __call__(self):
        for rid in relation_ids(self.rel_name):
            for unit in related_units(rid):
                rdata = relation_get(rid=rid, unit=unit)
                ctxt = {
                    'keystone_host': rdata.get('keystone_host'),
                    'service_port': rdata.get('service_port'),
                    'auth_port': rdata.get('auth_port'),
                    'service_tenant': rdata.get('service_tenant'),
                    'service_username': rdata.get('service_username'),
                    'service_password': rdata.get('service_password'),
                    'quantum_host': rdata.get('quantum_host'),
                    'quantum_port': rdata.get('quantum_port'),
                    'quantum_url': rdata.get('quantum_url'),
                    'region': rdata.get('region'),
                    'service_protocol':
                    rdata.get('service_protocol') or 'http',
                    'auth_protocol':
                    rdata.get('auth_protocol') or 'http',
                    'api_version':
                    rdata.get('api_version') or '2.0',
                }
                if self.context_complete(ctxt):
                    return ctxt
        return {}


class InternalEndpointContext(OSContextGenerator):
    """Internal endpoint context.

    This context provides the endpoint type used for communication between
    services e.g. between Nova and Cinder internally. Openstack uses Public
    endpoints by default so this allows admins to optionally use internal
    endpoints.
    """
    def __call__(self):
        return {'use_internal_endpoints': config('use-internal-endpoints')}


class AppArmorContext(OSContextGenerator):
    """Base class for apparmor contexts."""

    def __init__(self, profile_name=None):
        self._ctxt = None
        self.aa_profile = profile_name
        self.aa_utils_packages = ['apparmor-utils']

    @property
    def ctxt(self):
        if self._ctxt is not None:
            return self._ctxt
        self._ctxt = self._determine_ctxt()
        return self._ctxt

    def _determine_ctxt(self):
        """
        Validate aa-profile-mode settings is disable, enforce, or complain.

        :return ctxt: Dictionary of the apparmor profile or None
        """
        if config('aa-profile-mode') in ['disable', 'enforce', 'complain']:
            ctxt = {'aa_profile_mode': config('aa-profile-mode'),
                    'ubuntu_release': lsb_release()['DISTRIB_RELEASE']}
            if self.aa_profile:
                ctxt['aa_profile'] = self.aa_profile
        else:
            ctxt = None
        return ctxt

    def __call__(self):
        return self.ctxt

    def install_aa_utils(self):
        """
        Install packages required for apparmor configuration.
        """
        log("Installing apparmor utils.")
        ensure_packages(self.aa_utils_packages)

    def manually_disable_aa_profile(self):
        """
        Manually disable an apparmor profile.

        If aa-profile-mode is set to disabled (default) this is required as the
        template has been written but apparmor is yet unaware of the profile
        and aa-disable aa-profile fails. Without this the profile would kick
        into enforce mode on the next service restart.

        """
        profile_path = '/etc/apparmor.d'
        disable_path = '/etc/apparmor.d/disable'
        if not os.path.lexists(os.path.join(disable_path, self.aa_profile)):
            os.symlink(os.path.join(profile_path, self.aa_profile),
                       os.path.join(disable_path, self.aa_profile))

    def setup_aa_profile(self):
        """
        Setup an apparmor profile.
        The ctxt dictionary will contain the apparmor profile mode and
        the apparmor profile name.
        Makes calls out to aa-disable, aa-complain, or aa-enforce to setup
        the apparmor profile.
        """
        self()
        if not self.ctxt:
            log("Not enabling apparmor Profile")
            return
        self.install_aa_utils()
        cmd = ['aa-{}'.format(self.ctxt['aa_profile_mode'])]
        cmd.append(self.ctxt['aa_profile'])
        log("Setting up the apparmor profile for {} in {} mode."
            "".format(self.ctxt['aa_profile'], self.ctxt['aa_profile_mode']))
        try:
            check_call(cmd)
        except CalledProcessError as e:
            # If aa-profile-mode is set to disabled (default) manual
            # disabling is required as the template has been written but
            # apparmor is yet unaware of the profile and aa-disable aa-profile
            # fails. If aa-disable learns to read profile files first this can
            # be removed.
            if self.ctxt['aa_profile_mode'] == 'disable':
                log("Manually disabling the apparmor profile for {}."
                    "".format(self.ctxt['aa_profile']))
                self.manually_disable_aa_profile()
                return
            status_set('blocked', "Apparmor profile {} failed to be set to {}."
                                  "".format(self.ctxt['aa_profile'],
                                            self.ctxt['aa_profile_mode']))
            raise e


class MemcacheContext(OSContextGenerator):
    """Memcache context

    This context provides options for configuring a local memcache client and
    server
    """
    def __call__(self):
        ctxt = {}
        ctxt['use_memcache'] = enable_memcache(config('openstack-origin'))
        if ctxt['use_memcache']:
            ctxt['memcache_server'] = '::1'
            ctxt['memcache_server_formatted'] = '[::1]'
            ctxt['memcache_port'] = '11211'
            ctxt['memcache_url'] = 'inet6:{}:{}'.format(
                ctxt['memcache_server_formatted'],
                ctxt['memcache_port'])

        return ctxt
