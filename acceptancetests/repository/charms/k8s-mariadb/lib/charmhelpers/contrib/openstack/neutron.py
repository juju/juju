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

# Various utilies for dealing with Neutron and the renaming from Quantum.

import six
from subprocess import check_output

from charmhelpers.core.hookenv import (
    config,
    log,
    ERROR,
)

from charmhelpers.contrib.openstack.utils import (
    os_release,
    CompareOpenStackReleases,
)


def headers_package():
    """Ensures correct linux-headers for running kernel are installed,
    for building DKMS package"""
    kver = check_output(['uname', '-r']).decode('UTF-8').strip()
    return 'linux-headers-%s' % kver


QUANTUM_CONF_DIR = '/etc/quantum'


def kernel_version():
    """ Retrieve the current major kernel version as a tuple e.g. (3, 13) """
    kver = check_output(['uname', '-r']).decode('UTF-8').strip()
    kver = kver.split('.')
    return (int(kver[0]), int(kver[1]))


def determine_dkms_package():
    """ Determine which DKMS package should be used based on kernel version """
    # NOTE: 3.13 kernels have support for GRE and VXLAN native
    if kernel_version() >= (3, 13):
        return []
    else:
        return [headers_package(), 'openvswitch-datapath-dkms']


# legacy


def quantum_plugins():
    return {
        'ovs': {
            'config': '/etc/quantum/plugins/openvswitch/'
                      'ovs_quantum_plugin.ini',
            'driver': 'quantum.plugins.openvswitch.ovs_quantum_plugin.'
                      'OVSQuantumPluginV2',
            'contexts': [],
            'services': ['quantum-plugin-openvswitch-agent'],
            'packages': [determine_dkms_package(),
                         ['quantum-plugin-openvswitch-agent']],
            'server_packages': ['quantum-server',
                                'quantum-plugin-openvswitch'],
            'server_services': ['quantum-server']
        },
        'nvp': {
            'config': '/etc/quantum/plugins/nicira/nvp.ini',
            'driver': 'quantum.plugins.nicira.nicira_nvp_plugin.'
                      'QuantumPlugin.NvpPluginV2',
            'contexts': [],
            'services': [],
            'packages': [],
            'server_packages': ['quantum-server',
                                'quantum-plugin-nicira'],
            'server_services': ['quantum-server']
        }
    }


NEUTRON_CONF_DIR = '/etc/neutron'


def neutron_plugins():
    release = os_release('nova-common')
    plugins = {
        'ovs': {
            'config': '/etc/neutron/plugins/openvswitch/'
                      'ovs_neutron_plugin.ini',
            'driver': 'neutron.plugins.openvswitch.ovs_neutron_plugin.'
                      'OVSNeutronPluginV2',
            'contexts': [],
            'services': ['neutron-plugin-openvswitch-agent'],
            'packages': [determine_dkms_package(),
                         ['neutron-plugin-openvswitch-agent']],
            'server_packages': ['neutron-server',
                                'neutron-plugin-openvswitch'],
            'server_services': ['neutron-server']
        },
        'nvp': {
            'config': '/etc/neutron/plugins/nicira/nvp.ini',
            'driver': 'neutron.plugins.nicira.nicira_nvp_plugin.'
                      'NeutronPlugin.NvpPluginV2',
            'contexts': [],
            'services': [],
            'packages': [],
            'server_packages': ['neutron-server',
                                'neutron-plugin-nicira'],
            'server_services': ['neutron-server']
        },
        'nsx': {
            'config': '/etc/neutron/plugins/vmware/nsx.ini',
            'driver': 'vmware',
            'contexts': [],
            'services': [],
            'packages': [],
            'server_packages': ['neutron-server',
                                'neutron-plugin-vmware'],
            'server_services': ['neutron-server']
        },
        'n1kv': {
            'config': '/etc/neutron/plugins/cisco/cisco_plugins.ini',
            'driver': 'neutron.plugins.cisco.network_plugin.PluginV2',
            'contexts': [],
            'services': [],
            'packages': [determine_dkms_package(),
                         ['neutron-plugin-cisco']],
            'server_packages': ['neutron-server',
                                'neutron-plugin-cisco'],
            'server_services': ['neutron-server']
        },
        'Calico': {
            'config': '/etc/neutron/plugins/ml2/ml2_conf.ini',
            'driver': 'neutron.plugins.ml2.plugin.Ml2Plugin',
            'contexts': [],
            'services': ['calico-felix',
                         'bird',
                         'neutron-dhcp-agent',
                         'nova-api-metadata',
                         'etcd'],
            'packages': [determine_dkms_package(),
                         ['calico-compute',
                          'bird',
                          'neutron-dhcp-agent',
                          'nova-api-metadata',
                          'etcd']],
            'server_packages': ['neutron-server', 'calico-control', 'etcd'],
            'server_services': ['neutron-server', 'etcd']
        },
        'vsp': {
            'config': '/etc/neutron/plugins/nuage/nuage_plugin.ini',
            'driver': 'neutron.plugins.nuage.plugin.NuagePlugin',
            'contexts': [],
            'services': [],
            'packages': [],
            'server_packages': ['neutron-server', 'neutron-plugin-nuage'],
            'server_services': ['neutron-server']
        },
        'plumgrid': {
            'config': '/etc/neutron/plugins/plumgrid/plumgrid.ini',
            'driver': ('neutron.plugins.plumgrid.plumgrid_plugin'
                       '.plumgrid_plugin.NeutronPluginPLUMgridV2'),
            'contexts': [],
            'services': [],
            'packages': ['plumgrid-lxc',
                         'iovisor-dkms'],
            'server_packages': ['neutron-server',
                                'neutron-plugin-plumgrid'],
            'server_services': ['neutron-server']
        },
        'midonet': {
            'config': '/etc/neutron/plugins/midonet/midonet.ini',
            'driver': 'midonet.neutron.plugin.MidonetPluginV2',
            'contexts': [],
            'services': [],
            'packages': [determine_dkms_package()],
            'server_packages': ['neutron-server',
                                'python-neutron-plugin-midonet'],
            'server_services': ['neutron-server']
        }
    }
    if CompareOpenStackReleases(release) >= 'icehouse':
        # NOTE: patch in ml2 plugin for icehouse onwards
        plugins['ovs']['config'] = '/etc/neutron/plugins/ml2/ml2_conf.ini'
        plugins['ovs']['driver'] = 'neutron.plugins.ml2.plugin.Ml2Plugin'
        plugins['ovs']['server_packages'] = ['neutron-server',
                                             'neutron-plugin-ml2']
        # NOTE: patch in vmware renames nvp->nsx for icehouse onwards
        plugins['nvp'] = plugins['nsx']
    if CompareOpenStackReleases(release) >= 'kilo':
        plugins['midonet']['driver'] = (
            'neutron.plugins.midonet.plugin.MidonetPluginV2')
    if CompareOpenStackReleases(release) >= 'liberty':
        plugins['midonet']['driver'] = (
            'midonet.neutron.plugin_v1.MidonetPluginV2')
        plugins['midonet']['server_packages'].remove(
            'python-neutron-plugin-midonet')
        plugins['midonet']['server_packages'].append(
            'python-networking-midonet')
        plugins['plumgrid']['driver'] = (
            'networking_plumgrid.neutron.plugins'
            '.plugin.NeutronPluginPLUMgridV2')
        plugins['plumgrid']['server_packages'].remove(
            'neutron-plugin-plumgrid')
    if CompareOpenStackReleases(release) >= 'mitaka':
        plugins['nsx']['server_packages'].remove('neutron-plugin-vmware')
        plugins['nsx']['server_packages'].append('python-vmware-nsx')
        plugins['nsx']['config'] = '/etc/neutron/nsx.ini'
        plugins['vsp']['driver'] = (
            'nuage_neutron.plugins.nuage.plugin.NuagePlugin')
    return plugins


def neutron_plugin_attribute(plugin, attr, net_manager=None):
    manager = net_manager or network_manager()
    if manager == 'quantum':
        plugins = quantum_plugins()
    elif manager == 'neutron':
        plugins = neutron_plugins()
    else:
        log("Network manager '%s' does not support plugins." % (manager),
            level=ERROR)
        raise Exception

    try:
        _plugin = plugins[plugin]
    except KeyError:
        log('Unrecognised plugin for %s: %s' % (manager, plugin), level=ERROR)
        raise Exception

    try:
        return _plugin[attr]
    except KeyError:
        return None


def network_manager():
    '''
    Deals with the renaming of Quantum to Neutron in H and any situations
    that require compatability (eg, deploying H with network-manager=quantum,
    upgrading from G).
    '''
    release = os_release('nova-common')
    manager = config('network-manager').lower()

    if manager not in ['quantum', 'neutron']:
        return manager

    if release in ['essex']:
        # E does not support neutron
        log('Neutron networking not supported in Essex.', level=ERROR)
        raise Exception
    elif release in ['folsom', 'grizzly']:
        # neutron is named quantum in F and G
        return 'quantum'
    else:
        # ensure accurate naming for all releases post-H
        return 'neutron'


def parse_mappings(mappings, key_rvalue=False):
    """By default mappings are lvalue keyed.

    If key_rvalue is True, the mapping will be reversed to allow multiple
    configs for the same lvalue.
    """
    parsed = {}
    if mappings:
        mappings = mappings.split()
        for m in mappings:
            p = m.partition(':')

            if key_rvalue:
                key_index = 2
                val_index = 0
                # if there is no rvalue skip to next
                if not p[1]:
                    continue
            else:
                key_index = 0
                val_index = 2

            key = p[key_index].strip()
            parsed[key] = p[val_index].strip()

    return parsed


def parse_bridge_mappings(mappings):
    """Parse bridge mappings.

    Mappings must be a space-delimited list of provider:bridge mappings.

    Returns dict of the form {provider:bridge}.
    """
    return parse_mappings(mappings)


def parse_data_port_mappings(mappings, default_bridge='br-data'):
    """Parse data port mappings.

    Mappings must be a space-delimited list of bridge:port.

    Returns dict of the form {port:bridge} where ports may be mac addresses or
    interface names.
    """

    # NOTE(dosaboy): we use rvalue for key to allow multiple values to be
    # proposed for <port> since it may be a mac address which will differ
    # across units this allowing first-known-good to be chosen.
    _mappings = parse_mappings(mappings, key_rvalue=True)
    if not _mappings or list(_mappings.values()) == ['']:
        if not mappings:
            return {}

        # For backwards-compatibility we need to support port-only provided in
        # config.
        _mappings = {mappings.split()[0]: default_bridge}

    ports = _mappings.keys()
    if len(set(ports)) != len(ports):
        raise Exception("It is not allowed to have the same port configured "
                        "on more than one bridge")

    return _mappings


def parse_vlan_range_mappings(mappings):
    """Parse vlan range mappings.

    Mappings must be a space-delimited list of provider:start:end mappings.

    The start:end range is optional and may be omitted.

    Returns dict of the form {provider: (start, end)}.
    """
    _mappings = parse_mappings(mappings)
    if not _mappings:
        return {}

    mappings = {}
    for p, r in six.iteritems(_mappings):
        mappings[p] = tuple(r.split(':'))

    return mappings
