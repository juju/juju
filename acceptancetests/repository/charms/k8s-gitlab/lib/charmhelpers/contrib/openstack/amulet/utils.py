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

import amulet
import json
import logging
import os
import re
import six
import time
import urllib
import urlparse

import cinderclient.v1.client as cinder_client
import cinderclient.v2.client as cinder_clientv2
import glanceclient.v1 as glance_client
import glanceclient.v2 as glance_clientv2
import heatclient.v1.client as heat_client
from keystoneclient.v2_0 import client as keystone_client
from keystoneauth1.identity import (
    v3,
    v2,
)
from keystoneauth1 import session as keystone_session
from keystoneclient.v3 import client as keystone_client_v3
from novaclient import exceptions

import novaclient.client as nova_client
import novaclient
import pika
import swiftclient

from charmhelpers.core.decorators import retry_on_exception
from charmhelpers.contrib.amulet.utils import (
    AmuletUtils
)
from charmhelpers.core.host import CompareHostReleases

DEBUG = logging.DEBUG
ERROR = logging.ERROR

NOVA_CLIENT_VERSION = "2"

OPENSTACK_RELEASES_PAIRS = [
    'trusty_icehouse', 'trusty_kilo', 'trusty_liberty',
    'trusty_mitaka', 'xenial_mitaka', 'xenial_newton',
    'yakkety_newton', 'xenial_ocata', 'zesty_ocata',
    'xenial_pike', 'artful_pike', 'xenial_queens',
    'bionic_queens', 'bionic_rocky', 'cosmic_rocky']


class OpenStackAmuletUtils(AmuletUtils):
    """OpenStack amulet utilities.

       This class inherits from AmuletUtils and has additional support
       that is specifically for use by OpenStack charm tests.
       """

    def __init__(self, log_level=ERROR):
        """Initialize the deployment environment."""
        super(OpenStackAmuletUtils, self).__init__(log_level)

    def validate_endpoint_data(self, endpoints, admin_port, internal_port,
                               public_port, expected, openstack_release=None):
        """Validate endpoint data. Pick the correct validator based on
           OpenStack release. Expected data should be in the v2 format:
           {
               'id': id,
               'region': region,
               'adminurl': adminurl,
               'internalurl': internalurl,
               'publicurl': publicurl,
               'service_id': service_id}

           """
        validation_function = self.validate_v2_endpoint_data
        xenial_queens = OPENSTACK_RELEASES_PAIRS.index('xenial_queens')
        if openstack_release and openstack_release >= xenial_queens:
                validation_function = self.validate_v3_endpoint_data
                expected = {
                    'id': expected['id'],
                    'region': expected['region'],
                    'region_id': 'RegionOne',
                    'url': self.valid_url,
                    'interface': self.not_null,
                    'service_id': expected['service_id']}
        return validation_function(endpoints, admin_port, internal_port,
                                   public_port, expected)

    def validate_v2_endpoint_data(self, endpoints, admin_port, internal_port,
                                  public_port, expected):
        """Validate endpoint data.

           Validate actual endpoint data vs expected endpoint data. The ports
           are used to find the matching endpoint.
           """
        self.log.debug('Validating endpoint data...')
        self.log.debug('actual: {}'.format(repr(endpoints)))
        found = False
        for ep in endpoints:
            self.log.debug('endpoint: {}'.format(repr(ep)))
            if (admin_port in ep.adminurl and
                    internal_port in ep.internalurl and
                    public_port in ep.publicurl):
                found = True
                actual = {'id': ep.id,
                          'region': ep.region,
                          'adminurl': ep.adminurl,
                          'internalurl': ep.internalurl,
                          'publicurl': ep.publicurl,
                          'service_id': ep.service_id}
                ret = self._validate_dict_data(expected, actual)
                if ret:
                    return 'unexpected endpoint data - {}'.format(ret)

        if not found:
            return 'endpoint not found'

    def validate_v3_endpoint_data(self, endpoints, admin_port, internal_port,
                                  public_port, expected, expected_num_eps=3):
        """Validate keystone v3 endpoint data.

        Validate the v3 endpoint data which has changed from v2.  The
        ports are used to find the matching endpoint.

        The new v3 endpoint data looks like:

        [<Endpoint enabled=True,
                   id=0432655fc2f74d1e9fa17bdaa6f6e60b,
                   interface=admin,
                   links={u'self': u'<RESTful URL of this endpoint>'},
                   region=RegionOne,
                   region_id=RegionOne,
                   service_id=17f842a0dc084b928e476fafe67e4095,
                   url=http://10.5.6.5:9312>,
         <Endpoint enabled=True,
                   id=6536cb6cb92f4f41bf22b079935c7707,
                   interface=admin,
                   links={u'self': u'<RESTful url of this endpoint>'},
                   region=RegionOne,
                   region_id=RegionOne,
                   service_id=72fc8736fb41435e8b3584205bb2cfa3,
                   url=http://10.5.6.6:35357/v3>,
                   ... ]
        """
        self.log.debug('Validating v3 endpoint data...')
        self.log.debug('actual: {}'.format(repr(endpoints)))
        found = []
        for ep in endpoints:
            self.log.debug('endpoint: {}'.format(repr(ep)))
            if ((admin_port in ep.url and ep.interface == 'admin') or
                    (internal_port in ep.url and ep.interface == 'internal') or
                    (public_port in ep.url and ep.interface == 'public')):
                found.append(ep.interface)
                # note we ignore the links member.
                actual = {'id': ep.id,
                          'region': ep.region,
                          'region_id': ep.region_id,
                          'interface': self.not_null,
                          'url': ep.url,
                          'service_id': ep.service_id, }
                ret = self._validate_dict_data(expected, actual)
                if ret:
                    return 'unexpected endpoint data - {}'.format(ret)

        if len(found) != expected_num_eps:
            return 'Unexpected number of endpoints found'

    def convert_svc_catalog_endpoint_data_to_v3(self, ep_data):
        """Convert v2 endpoint data into v3.

           {
               'service_name1': [
                   {
                       'adminURL': adminURL,
                       'id': id,
                       'region': region.
                       'publicURL': publicURL,
                       'internalURL': internalURL
                   }],
               'service_name2': [
                   {
                       'adminURL': adminURL,
                       'id': id,
                       'region': region.
                       'publicURL': publicURL,
                       'internalURL': internalURL
                   }],
           }
          """
        self.log.warn("Endpoint ID and Region ID validation is limited to not "
                      "null checks after v2 to v3 conversion")
        for svc in ep_data.keys():
            assert len(ep_data[svc]) == 1, "Unknown data format"
            svc_ep_data = ep_data[svc][0]
            ep_data[svc] = [
                {
                    'url': svc_ep_data['adminURL'],
                    'interface': 'admin',
                    'region': svc_ep_data['region'],
                    'region_id': self.not_null,
                    'id': self.not_null},
                {
                    'url': svc_ep_data['publicURL'],
                    'interface': 'public',
                    'region': svc_ep_data['region'],
                    'region_id': self.not_null,
                    'id': self.not_null},
                {
                    'url': svc_ep_data['internalURL'],
                    'interface': 'internal',
                    'region': svc_ep_data['region'],
                    'region_id': self.not_null,
                    'id': self.not_null}]
        return ep_data

    def validate_svc_catalog_endpoint_data(self, expected, actual,
                                           openstack_release=None):
        """Validate service catalog endpoint data. Pick the correct validator
           for the OpenStack version. Expected data should be in the v2 format:
           {
               'service_name1': [
                   {
                       'adminURL': adminURL,
                       'id': id,
                       'region': region.
                       'publicURL': publicURL,
                       'internalURL': internalURL
                   }],
               'service_name2': [
                   {
                       'adminURL': adminURL,
                       'id': id,
                       'region': region.
                       'publicURL': publicURL,
                       'internalURL': internalURL
                   }],
           }

           """
        validation_function = self.validate_v2_svc_catalog_endpoint_data
        xenial_queens = OPENSTACK_RELEASES_PAIRS.index('xenial_queens')
        if openstack_release and openstack_release >= xenial_queens:
            validation_function = self.validate_v3_svc_catalog_endpoint_data
            expected = self.convert_svc_catalog_endpoint_data_to_v3(expected)
        return validation_function(expected, actual)

    def validate_v2_svc_catalog_endpoint_data(self, expected, actual):
        """Validate service catalog endpoint data.

           Validate a list of actual service catalog endpoints vs a list of
           expected service catalog endpoints.
           """
        self.log.debug('Validating service catalog endpoint data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        for k, v in six.iteritems(expected):
            if k in actual:
                ret = self._validate_dict_data(expected[k][0], actual[k][0])
                if ret:
                    return self.endpoint_error(k, ret)
            else:
                return "endpoint {} does not exist".format(k)
        return ret

    def validate_v3_svc_catalog_endpoint_data(self, expected, actual):
        """Validate the keystone v3 catalog endpoint data.

        Validate a list of dictinaries that make up the keystone v3 service
        catalogue.

        It is in the form of:


        {u'identity': [{u'id': u'48346b01c6804b298cdd7349aadb732e',
                        u'interface': u'admin',
                        u'region': u'RegionOne',
                        u'region_id': u'RegionOne',
                        u'url': u'http://10.5.5.224:35357/v3'},
                       {u'id': u'8414f7352a4b47a69fddd9dbd2aef5cf',
                        u'interface': u'public',
                        u'region': u'RegionOne',
                        u'region_id': u'RegionOne',
                        u'url': u'http://10.5.5.224:5000/v3'},
                       {u'id': u'd5ca31440cc24ee1bf625e2996fb6a5b',
                        u'interface': u'internal',
                        u'region': u'RegionOne',
                        u'region_id': u'RegionOne',
                        u'url': u'http://10.5.5.224:5000/v3'}],
         u'key-manager': [{u'id': u'68ebc17df0b045fcb8a8a433ebea9e62',
                           u'interface': u'public',
                           u'region': u'RegionOne',
                           u'region_id': u'RegionOne',
                           u'url': u'http://10.5.5.223:9311'},
                          {u'id': u'9cdfe2a893c34afd8f504eb218cd2f9d',
                           u'interface': u'internal',
                           u'region': u'RegionOne',
                           u'region_id': u'RegionOne',
                           u'url': u'http://10.5.5.223:9311'},
                          {u'id': u'f629388955bc407f8b11d8b7ca168086',
                           u'interface': u'admin',
                           u'region': u'RegionOne',
                           u'region_id': u'RegionOne',
                           u'url': u'http://10.5.5.223:9312'}]}

        Note, that an added complication is that the order of admin, public,
        internal against 'interface' in each region.

        Thus, the function sorts the expected and actual lists using the
        interface key as a sort key, prior to the comparison.
        """
        self.log.debug('Validating v3 service catalog endpoint data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        for k, v in six.iteritems(expected):
            if k in actual:
                l_expected = sorted(v, key=lambda x: x['interface'])
                l_actual = sorted(actual[k], key=lambda x: x['interface'])
                if len(l_actual) != len(l_expected):
                    return ("endpoint {} has differing number of interfaces "
                            " - expected({}), actual({})"
                            .format(k, len(l_expected), len(l_actual)))
                for i_expected, i_actual in zip(l_expected, l_actual):
                    self.log.debug("checking interface {}"
                                   .format(i_expected['interface']))
                    ret = self._validate_dict_data(i_expected, i_actual)
                    if ret:
                        return self.endpoint_error(k, ret)
            else:
                return "endpoint {} does not exist".format(k)
        return ret

    def validate_tenant_data(self, expected, actual):
        """Validate tenant data.

           Validate a list of actual tenant data vs list of expected tenant
           data.
           """
        self.log.debug('Validating tenant data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        for e in expected:
            found = False
            for act in actual:
                a = {'enabled': act.enabled, 'description': act.description,
                     'name': act.name, 'id': act.id}
                if e['name'] == a['name']:
                    found = True
                    ret = self._validate_dict_data(e, a)
                    if ret:
                        return "unexpected tenant data - {}".format(ret)
            if not found:
                return "tenant {} does not exist".format(e['name'])
        return ret

    def validate_role_data(self, expected, actual):
        """Validate role data.

           Validate a list of actual role data vs a list of expected role
           data.
           """
        self.log.debug('Validating role data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        for e in expected:
            found = False
            for act in actual:
                a = {'name': act.name, 'id': act.id}
                if e['name'] == a['name']:
                    found = True
                    ret = self._validate_dict_data(e, a)
                    if ret:
                        return "unexpected role data - {}".format(ret)
            if not found:
                return "role {} does not exist".format(e['name'])
        return ret

    def validate_user_data(self, expected, actual, api_version=None):
        """Validate user data.

           Validate a list of actual user data vs a list of expected user
           data.
           """
        self.log.debug('Validating user data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        for e in expected:
            found = False
            for act in actual:
                if e['name'] == act.name:
                    a = {'enabled': act.enabled, 'name': act.name,
                         'email': act.email, 'id': act.id}
                    if api_version == 3:
                        a['default_project_id'] = getattr(act,
                                                          'default_project_id',
                                                          'none')
                    else:
                        a['tenantId'] = act.tenantId
                    found = True
                    ret = self._validate_dict_data(e, a)
                    if ret:
                        return "unexpected user data - {}".format(ret)
            if not found:
                return "user {} does not exist".format(e['name'])
        return ret

    def validate_flavor_data(self, expected, actual):
        """Validate flavor data.

           Validate a list of actual flavors vs a list of expected flavors.
           """
        self.log.debug('Validating flavor data...')
        self.log.debug('actual: {}'.format(repr(actual)))
        act = [a.name for a in actual]
        return self._validate_list_data(expected, act)

    def tenant_exists(self, keystone, tenant):
        """Return True if tenant exists."""
        self.log.debug('Checking if tenant exists ({})...'.format(tenant))
        return tenant in [t.name for t in keystone.tenants.list()]

    @retry_on_exception(num_retries=5, base_delay=1)
    def keystone_wait_for_propagation(self, sentry_relation_pairs,
                                      api_version):
        """Iterate over list of sentry and relation tuples and verify that
           api_version has the expected value.

        :param sentry_relation_pairs: list of sentry, relation name tuples used
                                      for monitoring propagation of relation
                                      data
        :param api_version: api_version to expect in relation data
        :returns: None if successful.  Raise on error.
        """
        for (sentry, relation_name) in sentry_relation_pairs:
            rel = sentry.relation('identity-service',
                                  relation_name)
            self.log.debug('keystone relation data: {}'.format(rel))
            if rel.get('api_version') != str(api_version):
                raise Exception("api_version not propagated through relation"
                                " data yet ('{}' != '{}')."
                                "".format(rel.get('api_version'), api_version))

    def keystone_configure_api_version(self, sentry_relation_pairs, deployment,
                                       api_version):
        """Configure preferred-api-version of keystone in deployment and
           monitor provided list of relation objects for propagation
           before returning to caller.

        :param sentry_relation_pairs: list of sentry, relation tuples used for
                                      monitoring propagation of relation data
        :param deployment: deployment to configure
        :param api_version: value preferred-api-version will be set to
        :returns: None if successful.  Raise on error.
        """
        self.log.debug("Setting keystone preferred-api-version: '{}'"
                       "".format(api_version))

        config = {'preferred-api-version': api_version}
        deployment.d.configure('keystone', config)
        deployment._auto_wait_for_status()
        self.keystone_wait_for_propagation(sentry_relation_pairs, api_version)

    def authenticate_cinder_admin(self, keystone, api_version=2):
        """Authenticates admin user with cinder."""
        self.log.debug('Authenticating cinder admin...')
        _clients = {
            1: cinder_client.Client,
            2: cinder_clientv2.Client}
        return _clients[api_version](session=keystone.session)

    def authenticate_keystone(self, keystone_ip, username, password,
                              api_version=False, admin_port=False,
                              user_domain_name=None, domain_name=None,
                              project_domain_name=None, project_name=None):
        """Authenticate with Keystone"""
        self.log.debug('Authenticating with keystone...')
        if not api_version:
            api_version = 2
        sess, auth = self.get_keystone_session(
            keystone_ip=keystone_ip,
            username=username,
            password=password,
            api_version=api_version,
            admin_port=admin_port,
            user_domain_name=user_domain_name,
            domain_name=domain_name,
            project_domain_name=project_domain_name,
            project_name=project_name
        )
        if api_version == 2:
            client = keystone_client.Client(session=sess)
        else:
            client = keystone_client_v3.Client(session=sess)
        # This populates the client.service_catalog
        client.auth_ref = auth.get_access(sess)
        return client

    def get_keystone_session(self, keystone_ip, username, password,
                             api_version=False, admin_port=False,
                             user_domain_name=None, domain_name=None,
                             project_domain_name=None, project_name=None):
        """Return a keystone session object"""
        ep = self.get_keystone_endpoint(keystone_ip,
                                        api_version=api_version,
                                        admin_port=admin_port)
        if api_version == 2:
            auth = v2.Password(
                username=username,
                password=password,
                tenant_name=project_name,
                auth_url=ep
            )
            sess = keystone_session.Session(auth=auth)
        else:
            auth = v3.Password(
                user_domain_name=user_domain_name,
                username=username,
                password=password,
                domain_name=domain_name,
                project_domain_name=project_domain_name,
                project_name=project_name,
                auth_url=ep
            )
            sess = keystone_session.Session(auth=auth)
        return (sess, auth)

    def get_keystone_endpoint(self, keystone_ip, api_version=None,
                              admin_port=False):
        """Return keystone endpoint"""
        port = 5000
        if admin_port:
            port = 35357
        base_ep = "http://{}:{}".format(keystone_ip.strip().decode('utf-8'),
                                        port)
        if api_version == 2:
            ep = base_ep + "/v2.0"
        else:
            ep = base_ep + "/v3"
        return ep

    def get_default_keystone_session(self, keystone_sentry,
                                     openstack_release=None, api_version=2):
        """Return a keystone session object and client object assuming standard
           default settings

           Example call in amulet tests:
               self.keystone_session, self.keystone = u.get_default_keystone_session(
                   self.keystone_sentry,
                   openstack_release=self._get_openstack_release())

           The session can then be used to auth other clients:
               neutronclient.Client(session=session)
               aodh_client.Client(session=session)
               eyc
        """
        self.log.debug('Authenticating keystone admin...')
        # 11 => xenial_queens
        if api_version == 3 or (openstack_release and openstack_release >= 11):
            client_class = keystone_client_v3.Client
            api_version = 3
        else:
            client_class = keystone_client.Client
        keystone_ip = keystone_sentry.info['public-address']
        session, auth = self.get_keystone_session(
            keystone_ip,
            api_version=api_version,
            username='admin',
            password='openstack',
            project_name='admin',
            user_domain_name='admin_domain',
            project_domain_name='admin_domain')
        client = client_class(session=session)
        # This populates the client.service_catalog
        client.auth_ref = auth.get_access(session)
        return session, client

    def authenticate_keystone_admin(self, keystone_sentry, user, password,
                                    tenant=None, api_version=None,
                                    keystone_ip=None, user_domain_name=None,
                                    project_domain_name=None,
                                    project_name=None):
        """Authenticates admin user with the keystone admin endpoint."""
        self.log.debug('Authenticating keystone admin...')
        if not keystone_ip:
            keystone_ip = keystone_sentry.info['public-address']

        # To support backward compatibility usage of this function
        if not project_name:
            project_name = tenant
        if api_version == 3 and not user_domain_name:
            user_domain_name = 'admin_domain'
        if api_version == 3 and not project_domain_name:
            project_domain_name = 'admin_domain'
        if api_version == 3 and not project_name:
            project_name = 'admin'

        return self.authenticate_keystone(
            keystone_ip, user, password,
            api_version=api_version,
            user_domain_name=user_domain_name,
            project_domain_name=project_domain_name,
            project_name=project_name,
            admin_port=True)

    def authenticate_keystone_user(self, keystone, user, password, tenant):
        """Authenticates a regular user with the keystone public endpoint."""
        self.log.debug('Authenticating keystone user ({})...'.format(user))
        ep = keystone.service_catalog.url_for(service_type='identity',
                                              interface='publicURL')
        keystone_ip = urlparse.urlparse(ep).hostname

        return self.authenticate_keystone(keystone_ip, user, password,
                                          project_name=tenant)

    def authenticate_glance_admin(self, keystone, force_v1_client=False):
        """Authenticates admin user with glance."""
        self.log.debug('Authenticating glance admin...')
        ep = keystone.service_catalog.url_for(service_type='image',
                                              interface='adminURL')
        if not force_v1_client and keystone.session:
            return glance_clientv2.Client("2", session=keystone.session)
        else:
            return glance_client.Client(ep, token=keystone.auth_token)

    def authenticate_heat_admin(self, keystone):
        """Authenticates the admin user with heat."""
        self.log.debug('Authenticating heat admin...')
        ep = keystone.service_catalog.url_for(service_type='orchestration',
                                              interface='publicURL')
        if keystone.session:
            return heat_client.Client(endpoint=ep, session=keystone.session)
        else:
            return heat_client.Client(endpoint=ep, token=keystone.auth_token)

    def authenticate_nova_user(self, keystone, user, password, tenant):
        """Authenticates a regular user with nova-api."""
        self.log.debug('Authenticating nova user ({})...'.format(user))
        ep = keystone.service_catalog.url_for(service_type='identity',
                                              interface='publicURL')
        if keystone.session:
            return nova_client.Client(NOVA_CLIENT_VERSION,
                                      session=keystone.session,
                                      auth_url=ep)
        elif novaclient.__version__[0] >= "7":
            return nova_client.Client(NOVA_CLIENT_VERSION,
                                      username=user, password=password,
                                      project_name=tenant, auth_url=ep)
        else:
            return nova_client.Client(NOVA_CLIENT_VERSION,
                                      username=user, api_key=password,
                                      project_id=tenant, auth_url=ep)

    def authenticate_swift_user(self, keystone, user, password, tenant):
        """Authenticates a regular user with swift api."""
        self.log.debug('Authenticating swift user ({})...'.format(user))
        ep = keystone.service_catalog.url_for(service_type='identity',
                                              interface='publicURL')
        if keystone.session:
            return swiftclient.Connection(session=keystone.session)
        else:
            return swiftclient.Connection(authurl=ep,
                                          user=user,
                                          key=password,
                                          tenant_name=tenant,
                                          auth_version='2.0')

    def create_flavor(self, nova, name, ram, vcpus, disk, flavorid="auto",
                      ephemeral=0, swap=0, rxtx_factor=1.0, is_public=True):
        """Create the specified flavor."""
        try:
            nova.flavors.find(name=name)
        except (exceptions.NotFound, exceptions.NoUniqueMatch):
            self.log.debug('Creating flavor ({})'.format(name))
            nova.flavors.create(name, ram, vcpus, disk, flavorid,
                                ephemeral, swap, rxtx_factor, is_public)

    def glance_create_image(self, glance, image_name, image_url,
                            download_dir='tests',
                            hypervisor_type=None,
                            disk_format='qcow2',
                            architecture='x86_64',
                            container_format='bare'):
        """Download an image and upload it to glance, validate its status
        and return an image object pointer. KVM defaults, can override for
        LXD.

        :param glance: pointer to authenticated glance api connection
        :param image_name: display name for new image
        :param image_url: url to retrieve
        :param download_dir: directory to store downloaded image file
        :param hypervisor_type: glance image hypervisor property
        :param disk_format: glance image disk format
        :param architecture: glance image architecture property
        :param container_format: glance image container format
        :returns: glance image pointer
        """
        self.log.debug('Creating glance image ({}) from '
                       '{}...'.format(image_name, image_url))

        # Download image
        http_proxy = os.getenv('AMULET_HTTP_PROXY')
        self.log.debug('AMULET_HTTP_PROXY: {}'.format(http_proxy))
        if http_proxy:
            proxies = {'http': http_proxy}
            opener = urllib.FancyURLopener(proxies)
        else:
            opener = urllib.FancyURLopener()

        abs_file_name = os.path.join(download_dir, image_name)
        if not os.path.exists(abs_file_name):
            opener.retrieve(image_url, abs_file_name)

        # Create glance image
        glance_properties = {
            'architecture': architecture,
        }
        if hypervisor_type:
            glance_properties['hypervisor_type'] = hypervisor_type
        # Create glance image
        if float(glance.version) < 2.0:
            with open(abs_file_name) as f:
                image = glance.images.create(
                    name=image_name,
                    is_public=True,
                    disk_format=disk_format,
                    container_format=container_format,
                    properties=glance_properties,
                    data=f)
        else:
            image = glance.images.create(
                name=image_name,
                visibility="public",
                disk_format=disk_format,
                container_format=container_format)
            glance.images.upload(image.id, open(abs_file_name, 'rb'))
            glance.images.update(image.id, **glance_properties)

        # Wait for image to reach active status
        img_id = image.id
        ret = self.resource_reaches_status(glance.images, img_id,
                                           expected_stat='active',
                                           msg='Image status wait')
        if not ret:
            msg = 'Glance image failed to reach expected state.'
            amulet.raise_status(amulet.FAIL, msg=msg)

        # Re-validate new image
        self.log.debug('Validating image attributes...')
        val_img_name = glance.images.get(img_id).name
        val_img_stat = glance.images.get(img_id).status
        val_img_cfmt = glance.images.get(img_id).container_format
        val_img_dfmt = glance.images.get(img_id).disk_format

        if float(glance.version) < 2.0:
            val_img_pub = glance.images.get(img_id).is_public
        else:
            val_img_pub = glance.images.get(img_id).visibility == "public"

        msg_attr = ('Image attributes - name:{} public:{} id:{} stat:{} '
                    'container fmt:{} disk fmt:{}'.format(
                        val_img_name, val_img_pub, img_id,
                        val_img_stat, val_img_cfmt, val_img_dfmt))

        if val_img_name == image_name and val_img_stat == 'active' \
                and val_img_pub is True and val_img_cfmt == container_format \
                and val_img_dfmt == disk_format:
            self.log.debug(msg_attr)
        else:
            msg = ('Image validation failed, {}'.format(msg_attr))
            amulet.raise_status(amulet.FAIL, msg=msg)

        return image

    def create_cirros_image(self, glance, image_name, hypervisor_type=None):
        """Download the latest cirros image and upload it to glance,
        validate and return a resource pointer.

        :param glance: pointer to authenticated glance connection
        :param image_name: display name for new image
        :param hypervisor_type: glance image hypervisor property
        :returns: glance image pointer
        """
        # /!\ DEPRECATION WARNING
        self.log.warn('/!\\ DEPRECATION WARNING:  use '
                      'glance_create_image instead of '
                      'create_cirros_image.')

        self.log.debug('Creating glance cirros image '
                       '({})...'.format(image_name))

        # Get cirros image URL
        http_proxy = os.getenv('AMULET_HTTP_PROXY')
        self.log.debug('AMULET_HTTP_PROXY: {}'.format(http_proxy))
        if http_proxy:
            proxies = {'http': http_proxy}
            opener = urllib.FancyURLopener(proxies)
        else:
            opener = urllib.FancyURLopener()

        f = opener.open('http://download.cirros-cloud.net/version/released')
        version = f.read().strip()
        cirros_img = 'cirros-{}-x86_64-disk.img'.format(version)
        cirros_url = 'http://{}/{}/{}'.format('download.cirros-cloud.net',
                                              version, cirros_img)
        f.close()

        return self.glance_create_image(
            glance,
            image_name,
            cirros_url,
            hypervisor_type=hypervisor_type)

    def delete_image(self, glance, image):
        """Delete the specified image."""

        # /!\ DEPRECATION WARNING
        self.log.warn('/!\\ DEPRECATION WARNING:  use '
                      'delete_resource instead of delete_image.')
        self.log.debug('Deleting glance image ({})...'.format(image))
        return self.delete_resource(glance.images, image, msg='glance image')

    def create_instance(self, nova, image_name, instance_name, flavor):
        """Create the specified instance."""
        self.log.debug('Creating instance '
                       '({}|{}|{})'.format(instance_name, image_name, flavor))
        image = nova.glance.find_image(image_name)
        flavor = nova.flavors.find(name=flavor)
        instance = nova.servers.create(name=instance_name, image=image,
                                       flavor=flavor)

        count = 1
        status = instance.status
        while status != 'ACTIVE' and count < 60:
            time.sleep(3)
            instance = nova.servers.get(instance.id)
            status = instance.status
            self.log.debug('instance status: {}'.format(status))
            count += 1

        if status != 'ACTIVE':
            self.log.error('instance creation timed out')
            return None

        return instance

    def delete_instance(self, nova, instance):
        """Delete the specified instance."""

        # /!\ DEPRECATION WARNING
        self.log.warn('/!\\ DEPRECATION WARNING:  use '
                      'delete_resource instead of delete_instance.')
        self.log.debug('Deleting instance ({})...'.format(instance))
        return self.delete_resource(nova.servers, instance,
                                    msg='nova instance')

    def create_or_get_keypair(self, nova, keypair_name="testkey"):
        """Create a new keypair, or return pointer if it already exists."""
        try:
            _keypair = nova.keypairs.get(keypair_name)
            self.log.debug('Keypair ({}) already exists, '
                           'using it.'.format(keypair_name))
            return _keypair
        except Exception:
            self.log.debug('Keypair ({}) does not exist, '
                           'creating it.'.format(keypair_name))

        _keypair = nova.keypairs.create(name=keypair_name)
        return _keypair

    def _get_cinder_obj_name(self, cinder_object):
        """Retrieve name of cinder object.

        :param cinder_object: cinder snapshot or volume object
        :returns: str cinder object name
        """
        # v1 objects store name in 'display_name' attr but v2+ use 'name'
        try:
            return cinder_object.display_name
        except AttributeError:
            return cinder_object.name

    def create_cinder_volume(self, cinder, vol_name="demo-vol", vol_size=1,
                             img_id=None, src_vol_id=None, snap_id=None):
        """Create cinder volume, optionally from a glance image, OR
        optionally as a clone of an existing volume, OR optionally
        from a snapshot.  Wait for the new volume status to reach
        the expected status, validate and return a resource pointer.

        :param vol_name: cinder volume display name
        :param vol_size: size in gigabytes
        :param img_id: optional glance image id
        :param src_vol_id: optional source volume id to clone
        :param snap_id: optional snapshot id to use
        :returns: cinder volume pointer
        """
        # Handle parameter input and avoid impossible combinations
        if img_id and not src_vol_id and not snap_id:
            # Create volume from image
            self.log.debug('Creating cinder volume from glance image...')
            bootable = 'true'
        elif src_vol_id and not img_id and not snap_id:
            # Clone an existing volume
            self.log.debug('Cloning cinder volume...')
            bootable = cinder.volumes.get(src_vol_id).bootable
        elif snap_id and not src_vol_id and not img_id:
            # Create volume from snapshot
            self.log.debug('Creating cinder volume from snapshot...')
            snap = cinder.volume_snapshots.find(id=snap_id)
            vol_size = snap.size
            snap_vol_id = cinder.volume_snapshots.get(snap_id).volume_id
            bootable = cinder.volumes.get(snap_vol_id).bootable
        elif not img_id and not src_vol_id and not snap_id:
            # Create volume
            self.log.debug('Creating cinder volume...')
            bootable = 'false'
        else:
            # Impossible combination of parameters
            msg = ('Invalid method use - name:{} size:{} img_id:{} '
                   'src_vol_id:{} snap_id:{}'.format(vol_name, vol_size,
                                                     img_id, src_vol_id,
                                                     snap_id))
            amulet.raise_status(amulet.FAIL, msg=msg)

        # Create new volume
        try:
            vol_new = cinder.volumes.create(display_name=vol_name,
                                            imageRef=img_id,
                                            size=vol_size,
                                            source_volid=src_vol_id,
                                            snapshot_id=snap_id)
            vol_id = vol_new.id
        except TypeError:
            vol_new = cinder.volumes.create(name=vol_name,
                                            imageRef=img_id,
                                            size=vol_size,
                                            source_volid=src_vol_id,
                                            snapshot_id=snap_id)
            vol_id = vol_new.id
        except Exception as e:
            msg = 'Failed to create volume: {}'.format(e)
            amulet.raise_status(amulet.FAIL, msg=msg)

        # Wait for volume to reach available status
        ret = self.resource_reaches_status(cinder.volumes, vol_id,
                                           expected_stat="available",
                                           msg="Volume status wait")
        if not ret:
            msg = 'Cinder volume failed to reach expected state.'
            amulet.raise_status(amulet.FAIL, msg=msg)

        # Re-validate new volume
        self.log.debug('Validating volume attributes...')
        val_vol_name = self._get_cinder_obj_name(cinder.volumes.get(vol_id))
        val_vol_boot = cinder.volumes.get(vol_id).bootable
        val_vol_stat = cinder.volumes.get(vol_id).status
        val_vol_size = cinder.volumes.get(vol_id).size
        msg_attr = ('Volume attributes - name:{} id:{} stat:{} boot:'
                    '{} size:{}'.format(val_vol_name, vol_id,
                                        val_vol_stat, val_vol_boot,
                                        val_vol_size))

        if val_vol_boot == bootable and val_vol_stat == 'available' \
                and val_vol_name == vol_name and val_vol_size == vol_size:
            self.log.debug(msg_attr)
        else:
            msg = ('Volume validation failed, {}'.format(msg_attr))
            amulet.raise_status(amulet.FAIL, msg=msg)

        return vol_new

    def delete_resource(self, resource, resource_id,
                        msg="resource", max_wait=120):
        """Delete one openstack resource, such as one instance, keypair,
        image, volume, stack, etc., and confirm deletion within max wait time.

        :param resource: pointer to os resource type, ex:glance_client.images
        :param resource_id: unique name or id for the openstack resource
        :param msg: text to identify purpose in logging
        :param max_wait: maximum wait time in seconds
        :returns: True if successful, otherwise False
        """
        self.log.debug('Deleting OpenStack resource '
                       '{} ({})'.format(resource_id, msg))
        num_before = len(list(resource.list()))
        resource.delete(resource_id)

        tries = 0
        num_after = len(list(resource.list()))
        while num_after != (num_before - 1) and tries < (max_wait / 4):
            self.log.debug('{} delete check: '
                           '{} [{}:{}] {}'.format(msg, tries,
                                                  num_before,
                                                  num_after,
                                                  resource_id))
            time.sleep(4)
            num_after = len(list(resource.list()))
            tries += 1

        self.log.debug('{}:  expected, actual count = {}, '
                       '{}'.format(msg, num_before - 1, num_after))

        if num_after == (num_before - 1):
            return True
        else:
            self.log.error('{} delete timed out'.format(msg))
            return False

    def resource_reaches_status(self, resource, resource_id,
                                expected_stat='available',
                                msg='resource', max_wait=120):
        """Wait for an openstack resources status to reach an
           expected status within a specified time.  Useful to confirm that
           nova instances, cinder vols, snapshots, glance images, heat stacks
           and other resources eventually reach the expected status.

        :param resource: pointer to os resource type, ex: heat_client.stacks
        :param resource_id: unique id for the openstack resource
        :param expected_stat: status to expect resource to reach
        :param msg: text to identify purpose in logging
        :param max_wait: maximum wait time in seconds
        :returns: True if successful, False if status is not reached
        """

        tries = 0
        resource_stat = resource.get(resource_id).status
        while resource_stat != expected_stat and tries < (max_wait / 4):
            self.log.debug('{} status check: '
                           '{} [{}:{}] {}'.format(msg, tries,
                                                  resource_stat,
                                                  expected_stat,
                                                  resource_id))
            time.sleep(4)
            resource_stat = resource.get(resource_id).status
            tries += 1

        self.log.debug('{}:  expected, actual status = {}, '
                       '{}'.format(msg, resource_stat, expected_stat))

        if resource_stat == expected_stat:
            return True
        else:
            self.log.debug('{} never reached expected status: '
                           '{}'.format(resource_id, expected_stat))
            return False

    def get_ceph_osd_id_cmd(self, index):
        """Produce a shell command that will return a ceph-osd id."""
        return ("`initctl list | grep 'ceph-osd ' | "
                "awk 'NR=={} {{ print $2 }}' | "
                "grep -o '[0-9]*'`".format(index + 1))

    def get_ceph_pools(self, sentry_unit):
        """Return a dict of ceph pools from a single ceph unit, with
        pool name as keys, pool id as vals."""
        pools = {}
        cmd = 'sudo ceph osd lspools'
        output, code = sentry_unit.run(cmd)
        if code != 0:
            msg = ('{} `{}` returned {} '
                   '{}'.format(sentry_unit.info['unit_name'],
                               cmd, code, output))
            amulet.raise_status(amulet.FAIL, msg=msg)

        # For mimic ceph osd lspools output
        output = output.replace("\n", ",")

        # Example output: 0 data,1 metadata,2 rbd,3 cinder,4 glance,
        for pool in str(output).split(','):
            pool_id_name = pool.split(' ')
            if len(pool_id_name) == 2:
                pool_id = pool_id_name[0]
                pool_name = pool_id_name[1]
                pools[pool_name] = int(pool_id)

        self.log.debug('Pools on {}: {}'.format(sentry_unit.info['unit_name'],
                                                pools))
        return pools

    def get_ceph_df(self, sentry_unit):
        """Return dict of ceph df json output, including ceph pool state.

        :param sentry_unit: Pointer to amulet sentry instance (juju unit)
        :returns: Dict of ceph df output
        """
        cmd = 'sudo ceph df --format=json'
        output, code = sentry_unit.run(cmd)
        if code != 0:
            msg = ('{} `{}` returned {} '
                   '{}'.format(sentry_unit.info['unit_name'],
                               cmd, code, output))
            amulet.raise_status(amulet.FAIL, msg=msg)
        return json.loads(output)

    def get_ceph_pool_sample(self, sentry_unit, pool_id=0):
        """Take a sample of attributes of a ceph pool, returning ceph
        pool name, object count and disk space used for the specified
        pool ID number.

        :param sentry_unit: Pointer to amulet sentry instance (juju unit)
        :param pool_id: Ceph pool ID
        :returns: List of pool name, object count, kb disk space used
        """
        df = self.get_ceph_df(sentry_unit)
        for pool in df['pools']:
            if pool['id'] == pool_id:
                pool_name = pool['name']
                obj_count = pool['stats']['objects']
                kb_used = pool['stats']['kb_used']

        self.log.debug('Ceph {} pool (ID {}): {} objects, '
                       '{} kb used'.format(pool_name, pool_id,
                                           obj_count, kb_used))
        return pool_name, obj_count, kb_used

    def validate_ceph_pool_samples(self, samples, sample_type="resource pool"):
        """Validate ceph pool samples taken over time, such as pool
        object counts or pool kb used, before adding, after adding, and
        after deleting items which affect those pool attributes.  The
        2nd element is expected to be greater than the 1st; 3rd is expected
        to be less than the 2nd.

        :param samples: List containing 3 data samples
        :param sample_type: String for logging and usage context
        :returns: None if successful, Failure message otherwise
        """
        original, created, deleted = range(3)
        if samples[created] <= samples[original] or \
                samples[deleted] >= samples[created]:
            return ('Ceph {} samples ({}) '
                    'unexpected.'.format(sample_type, samples))
        else:
            self.log.debug('Ceph {} samples (OK): '
                           '{}'.format(sample_type, samples))
            return None

    # rabbitmq/amqp specific helpers:

    def rmq_wait_for_cluster(self, deployment, init_sleep=15, timeout=1200):
        """Wait for rmq units extended status to show cluster readiness,
        after an optional initial sleep period.  Initial sleep is likely
        necessary to be effective following a config change, as status
        message may not instantly update to non-ready."""

        if init_sleep:
            time.sleep(init_sleep)

        message = re.compile('^Unit is ready and clustered$')
        deployment._auto_wait_for_status(message=message,
                                         timeout=timeout,
                                         include_only=['rabbitmq-server'])

    def add_rmq_test_user(self, sentry_units,
                          username="testuser1", password="changeme"):
        """Add a test user via the first rmq juju unit, check connection as
        the new user against all sentry units.

        :param sentry_units: list of sentry unit pointers
        :param username: amqp user name, default to testuser1
        :param password: amqp user password
        :returns: None if successful.  Raise on error.
        """
        self.log.debug('Adding rmq user ({})...'.format(username))

        # Check that user does not already exist
        cmd_user_list = 'rabbitmqctl list_users'
        output, _ = self.run_cmd_unit(sentry_units[0], cmd_user_list)
        if username in output:
            self.log.warning('User ({}) already exists, returning '
                             'gracefully.'.format(username))
            return

        perms = '".*" ".*" ".*"'
        cmds = ['rabbitmqctl add_user {} {}'.format(username, password),
                'rabbitmqctl set_permissions {} {}'.format(username, perms)]

        # Add user via first unit
        for cmd in cmds:
            output, _ = self.run_cmd_unit(sentry_units[0], cmd)

        # Check connection against the other sentry_units
        self.log.debug('Checking user connect against units...')
        for sentry_unit in sentry_units:
            connection = self.connect_amqp_by_unit(sentry_unit, ssl=False,
                                                   username=username,
                                                   password=password)
            connection.close()

    def delete_rmq_test_user(self, sentry_units, username="testuser1"):
        """Delete a rabbitmq user via the first rmq juju unit.

        :param sentry_units: list of sentry unit pointers
        :param username: amqp user name, default to testuser1
        :param password: amqp user password
        :returns: None if successful or no such user.
        """
        self.log.debug('Deleting rmq user ({})...'.format(username))

        # Check that the user exists
        cmd_user_list = 'rabbitmqctl list_users'
        output, _ = self.run_cmd_unit(sentry_units[0], cmd_user_list)

        if username not in output:
            self.log.warning('User ({}) does not exist, returning '
                             'gracefully.'.format(username))
            return

        # Delete the user
        cmd_user_del = 'rabbitmqctl delete_user {}'.format(username)
        output, _ = self.run_cmd_unit(sentry_units[0], cmd_user_del)

    def get_rmq_cluster_status(self, sentry_unit):
        """Execute rabbitmq cluster status command on a unit and return
        the full output.

        :param unit: sentry unit
        :returns: String containing console output of cluster status command
        """
        cmd = 'rabbitmqctl cluster_status'
        output, _ = self.run_cmd_unit(sentry_unit, cmd)
        self.log.debug('{} cluster_status:\n{}'.format(
            sentry_unit.info['unit_name'], output))
        return str(output)

    def get_rmq_cluster_running_nodes(self, sentry_unit):
        """Parse rabbitmqctl cluster_status output string, return list of
        running rabbitmq cluster nodes.

        :param unit: sentry unit
        :returns: List containing node names of running nodes
        """
        # NOTE(beisner): rabbitmqctl cluster_status output is not
        # json-parsable, do string chop foo, then json.loads that.
        str_stat = self.get_rmq_cluster_status(sentry_unit)
        if 'running_nodes' in str_stat:
            pos_start = str_stat.find("{running_nodes,") + 15
            pos_end = str_stat.find("]},", pos_start) + 1
            str_run_nodes = str_stat[pos_start:pos_end].replace("'", '"')
            run_nodes = json.loads(str_run_nodes)
            return run_nodes
        else:
            return []

    def validate_rmq_cluster_running_nodes(self, sentry_units):
        """Check that all rmq unit hostnames are represented in the
        cluster_status output of all units.

        :param host_names: dict of juju unit names to host names
        :param units: list of sentry unit pointers (all rmq units)
        :returns: None if successful, otherwise return error message
        """
        host_names = self.get_unit_hostnames(sentry_units)
        errors = []

        # Query every unit for cluster_status running nodes
        for query_unit in sentry_units:
            query_unit_name = query_unit.info['unit_name']
            running_nodes = self.get_rmq_cluster_running_nodes(query_unit)

            # Confirm that every unit is represented in the queried unit's
            # cluster_status running nodes output.
            for validate_unit in sentry_units:
                val_host_name = host_names[validate_unit.info['unit_name']]
                val_node_name = 'rabbit@{}'.format(val_host_name)

                if val_node_name not in running_nodes:
                    errors.append('Cluster member check failed on {}: {} not '
                                  'in {}\n'.format(query_unit_name,
                                                   val_node_name,
                                                   running_nodes))
        if errors:
            return ''.join(errors)

    def rmq_ssl_is_enabled_on_unit(self, sentry_unit, port=None):
        """Check a single juju rmq unit for ssl and port in the config file."""
        host = sentry_unit.info['public-address']
        unit_name = sentry_unit.info['unit_name']

        conf_file = '/etc/rabbitmq/rabbitmq.config'
        conf_contents = str(self.file_contents_safe(sentry_unit,
                                                    conf_file, max_wait=16))
        # Checks
        conf_ssl = 'ssl' in conf_contents
        conf_port = str(port) in conf_contents

        # Port explicitly checked in config
        if port and conf_port and conf_ssl:
            self.log.debug('SSL is enabled  @{}:{} '
                           '({})'.format(host, port, unit_name))
            return True
        elif port and not conf_port and conf_ssl:
            self.log.debug('SSL is enabled @{} but not on port {} '
                           '({})'.format(host, port, unit_name))
            return False
        # Port not checked (useful when checking that ssl is disabled)
        elif not port and conf_ssl:
            self.log.debug('SSL is enabled  @{}:{} '
                           '({})'.format(host, port, unit_name))
            return True
        elif not conf_ssl:
            self.log.debug('SSL not enabled @{}:{} '
                           '({})'.format(host, port, unit_name))
            return False
        else:
            msg = ('Unknown condition when checking SSL status @{}:{} '
                   '({})'.format(host, port, unit_name))
            amulet.raise_status(amulet.FAIL, msg)

    def validate_rmq_ssl_enabled_units(self, sentry_units, port=None):
        """Check that ssl is enabled on rmq juju sentry units.

        :param sentry_units: list of all rmq sentry units
        :param port: optional ssl port override to validate
        :returns: None if successful, otherwise return error message
        """
        for sentry_unit in sentry_units:
            if not self.rmq_ssl_is_enabled_on_unit(sentry_unit, port=port):
                return ('Unexpected condition:  ssl is disabled on unit '
                        '({})'.format(sentry_unit.info['unit_name']))
        return None

    def validate_rmq_ssl_disabled_units(self, sentry_units):
        """Check that ssl is enabled on listed rmq juju sentry units.

        :param sentry_units: list of all rmq sentry units
        :returns: True if successful.  Raise on error.
        """
        for sentry_unit in sentry_units:
            if self.rmq_ssl_is_enabled_on_unit(sentry_unit):
                return ('Unexpected condition:  ssl is enabled on unit '
                        '({})'.format(sentry_unit.info['unit_name']))
        return None

    def configure_rmq_ssl_on(self, sentry_units, deployment,
                             port=None, max_wait=60):
        """Turn ssl charm config option on, with optional non-default
        ssl port specification.  Confirm that it is enabled on every
        unit.

        :param sentry_units: list of sentry units
        :param deployment: amulet deployment object pointer
        :param port: amqp port, use defaults if None
        :param max_wait: maximum time to wait in seconds to confirm
        :returns: None if successful.  Raise on error.
        """
        self.log.debug('Setting ssl charm config option:  on')

        # Enable RMQ SSL
        config = {'ssl': 'on'}
        if port:
            config['ssl_port'] = port

        deployment.d.configure('rabbitmq-server', config)

        # Wait for unit status
        self.rmq_wait_for_cluster(deployment)

        # Confirm
        tries = 0
        ret = self.validate_rmq_ssl_enabled_units(sentry_units, port=port)
        while ret and tries < (max_wait / 4):
            time.sleep(4)
            self.log.debug('Attempt {}: {}'.format(tries, ret))
            ret = self.validate_rmq_ssl_enabled_units(sentry_units, port=port)
            tries += 1

        if ret:
            amulet.raise_status(amulet.FAIL, ret)

    def configure_rmq_ssl_off(self, sentry_units, deployment, max_wait=60):
        """Turn ssl charm config option off, confirm that it is disabled
        on every unit.

        :param sentry_units: list of sentry units
        :param deployment: amulet deployment object pointer
        :param max_wait: maximum time to wait in seconds to confirm
        :returns: None if successful.  Raise on error.
        """
        self.log.debug('Setting ssl charm config option:  off')

        # Disable RMQ SSL
        config = {'ssl': 'off'}
        deployment.d.configure('rabbitmq-server', config)

        # Wait for unit status
        self.rmq_wait_for_cluster(deployment)

        # Confirm
        tries = 0
        ret = self.validate_rmq_ssl_disabled_units(sentry_units)
        while ret and tries < (max_wait / 4):
            time.sleep(4)
            self.log.debug('Attempt {}: {}'.format(tries, ret))
            ret = self.validate_rmq_ssl_disabled_units(sentry_units)
            tries += 1

        if ret:
            amulet.raise_status(amulet.FAIL, ret)

    def connect_amqp_by_unit(self, sentry_unit, ssl=False,
                             port=None, fatal=True,
                             username="testuser1", password="changeme"):
        """Establish and return a pika amqp connection to the rabbitmq service
        running on a rmq juju unit.

        :param sentry_unit: sentry unit pointer
        :param ssl: boolean, default to False
        :param port: amqp port, use defaults if None
        :param fatal: boolean, default to True (raises on connect error)
        :param username: amqp user name, default to testuser1
        :param password: amqp user password
        :returns: pika amqp connection pointer or None if failed and non-fatal
        """
        host = sentry_unit.info['public-address']
        unit_name = sentry_unit.info['unit_name']

        # Default port logic if port is not specified
        if ssl and not port:
            port = 5671
        elif not ssl and not port:
            port = 5672

        self.log.debug('Connecting to amqp on {}:{} ({}) as '
                       '{}...'.format(host, port, unit_name, username))

        try:
            credentials = pika.PlainCredentials(username, password)
            parameters = pika.ConnectionParameters(host=host, port=port,
                                                   credentials=credentials,
                                                   ssl=ssl,
                                                   connection_attempts=3,
                                                   retry_delay=5,
                                                   socket_timeout=1)
            connection = pika.BlockingConnection(parameters)
            assert connection.is_open is True
            assert connection.is_closing is False
            self.log.debug('Connect OK')
            return connection
        except Exception as e:
            msg = ('amqp connection failed to {}:{} as '
                   '{} ({})'.format(host, port, username, str(e)))
            if fatal:
                amulet.raise_status(amulet.FAIL, msg)
            else:
                self.log.warn(msg)
                return None

    def publish_amqp_message_by_unit(self, sentry_unit, message,
                                     queue="test", ssl=False,
                                     username="testuser1",
                                     password="changeme",
                                     port=None):
        """Publish an amqp message to a rmq juju unit.

        :param sentry_unit: sentry unit pointer
        :param message: amqp message string
        :param queue: message queue, default to test
        :param username: amqp user name, default to testuser1
        :param password: amqp user password
        :param ssl: boolean, default to False
        :param port: amqp port, use defaults if None
        :returns: None.  Raises exception if publish failed.
        """
        self.log.debug('Publishing message to {} queue:\n{}'.format(queue,
                                                                    message))
        connection = self.connect_amqp_by_unit(sentry_unit, ssl=ssl,
                                               port=port,
                                               username=username,
                                               password=password)

        # NOTE(beisner): extra debug here re: pika hang potential:
        #   https://github.com/pika/pika/issues/297
        #   https://groups.google.com/forum/#!topic/rabbitmq-users/Ja0iyfF0Szw
        self.log.debug('Defining channel...')
        channel = connection.channel()
        self.log.debug('Declaring queue...')
        channel.queue_declare(queue=queue, auto_delete=False, durable=True)
        self.log.debug('Publishing message...')
        channel.basic_publish(exchange='', routing_key=queue, body=message)
        self.log.debug('Closing channel...')
        channel.close()
        self.log.debug('Closing connection...')
        connection.close()

    def get_amqp_message_by_unit(self, sentry_unit, queue="test",
                                 username="testuser1",
                                 password="changeme",
                                 ssl=False, port=None):
        """Get an amqp message from a rmq juju unit.

        :param sentry_unit: sentry unit pointer
        :param queue: message queue, default to test
        :param username: amqp user name, default to testuser1
        :param password: amqp user password
        :param ssl: boolean, default to False
        :param port: amqp port, use defaults if None
        :returns: amqp message body as string.  Raise if get fails.
        """
        connection = self.connect_amqp_by_unit(sentry_unit, ssl=ssl,
                                               port=port,
                                               username=username,
                                               password=password)
        channel = connection.channel()
        method_frame, _, body = channel.basic_get(queue)

        if method_frame:
            self.log.debug('Retreived message from {} queue:\n{}'.format(queue,
                                                                         body))
            channel.basic_ack(method_frame.delivery_tag)
            channel.close()
            connection.close()
            return body
        else:
            msg = 'No message retrieved.'
            amulet.raise_status(amulet.FAIL, msg)

    def validate_memcache(self, sentry_unit, conf, os_release,
                          earliest_release=5, section='keystone_authtoken',
                          check_kvs=None):
        """Check Memcache is running and is configured to be used

        Example call from Amulet test:

            def test_110_memcache(self):
                u.validate_memcache(self.neutron_api_sentry,
                                    '/etc/neutron/neutron.conf',
                                    self._get_openstack_release())

        :param sentry_unit: sentry unit
        :param conf: OpenStack config file to check memcache settings
        :param os_release: Current OpenStack release int code
        :param earliest_release: Earliest Openstack release to check int code
        :param section: OpenStack config file section to check
        :param check_kvs: Dict of settings to check in config file
        :returns: None
        """
        if os_release < earliest_release:
            self.log.debug('Skipping memcache checks for deployment. {} <'
                           'mitaka'.format(os_release))
            return
        _kvs = check_kvs or {'memcached_servers': 'inet6:[::1]:11211'}
        self.log.debug('Checking memcached is running')
        ret = self.validate_services_by_name({sentry_unit: ['memcached']})
        if ret:
            amulet.raise_status(amulet.FAIL, msg='Memcache running check'
                                'failed {}'.format(ret))
        else:
            self.log.debug('OK')
        self.log.debug('Checking memcache url is configured in {}'.format(
            conf))
        if self.validate_config_data(sentry_unit, conf, section, _kvs):
            message = "Memcache config error in: {}".format(conf)
            amulet.raise_status(amulet.FAIL, msg=message)
        else:
            self.log.debug('OK')
        self.log.debug('Checking memcache configuration in '
                       '/etc/memcached.conf')
        contents = self.file_contents_safe(sentry_unit, '/etc/memcached.conf',
                                           fatal=True)
        ubuntu_release, _ = self.run_cmd_unit(sentry_unit, 'lsb_release -cs')
        if CompareHostReleases(ubuntu_release) <= 'trusty':
            memcache_listen_addr = 'ip6-localhost'
        else:
            memcache_listen_addr = '::1'
        expected = {
            '-p': '11211',
            '-l': memcache_listen_addr}
        found = []
        for key, value in expected.items():
            for line in contents.split('\n'):
                if line.startswith(key):
                    self.log.debug('Checking {} is set to {}'.format(
                        key,
                        value))
                    assert value == line.split()[-1]
                    self.log.debug(line.split()[-1])
                    found.append(key)
        if sorted(found) == sorted(expected.keys()):
            self.log.debug('OK')
        else:
            message = "Memcache config error in: /etc/memcached.conf"
            amulet.raise_status(amulet.FAIL, msg=message)
