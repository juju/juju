#!/usr/bin/python
#
# Copyright 2017 Canonical Ltd
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

import six
from charmhelpers.fetch import apt_install
from charmhelpers.contrib.openstack.context import IdentityServiceContext
from charmhelpers.core.hookenv import (
    log,
    ERROR,
)


def get_api_suffix(api_version):
    """Return the formatted api suffix for the given version
    @param api_version: version of the keystone endpoint
    @returns the api suffix formatted according to the given api
    version
    """
    return 'v2.0' if api_version in (2, "2", "2.0") else 'v3'


def format_endpoint(schema, addr, port, api_version):
    """Return a formatted keystone endpoint
    @param schema: http or https
    @param addr: ipv4/ipv6 host of the keystone service
    @param port: port of the keystone service
    @param api_version: 2 or 3
    @returns a fully formatted keystone endpoint
    """
    return '{}://{}:{}/{}/'.format(schema, addr, port,
                                   get_api_suffix(api_version))


def get_keystone_manager(endpoint, api_version, **kwargs):
    """Return a keystonemanager for the correct API version

    @param endpoint: the keystone endpoint to point client at
    @param api_version: version of the keystone api the client should use
    @param kwargs: token or username/tenant/password information
    @returns keystonemanager class used for interrogating keystone
    """
    if api_version == 2:
        return KeystoneManager2(endpoint, **kwargs)
    if api_version == 3:
        return KeystoneManager3(endpoint, **kwargs)
    raise ValueError('No manager found for api version {}'.format(api_version))


def get_keystone_manager_from_identity_service_context():
    """Return a keystonmanager generated from a
    instance of charmhelpers.contrib.openstack.context.IdentityServiceContext
    @returns keystonamenager instance
   """
    context = IdentityServiceContext()()
    if not context:
        msg = "Identity service context cannot be generated"
        log(msg, level=ERROR)
        raise ValueError(msg)

    endpoint = format_endpoint(context['service_protocol'],
                               context['service_host'],
                               context['service_port'],
                               context['api_version'])

    if context['api_version'] in (2, "2.0"):
        api_version = 2
    else:
        api_version = 3

    return get_keystone_manager(endpoint, api_version,
                                username=context['admin_user'],
                                password=context['admin_password'],
                                tenant_name=context['admin_tenant_name'])


class KeystoneManager(object):

    def resolve_service_id(self, service_name=None, service_type=None):
        """Find the service_id of a given service"""
        services = [s._info for s in self.api.services.list()]

        service_name = service_name.lower()
        for s in services:
            name = s['name'].lower()
            if service_type and service_name:
                if (service_name == name and service_type == s['type']):
                    return s['id']
            elif service_name and service_name == name:
                return s['id']
            elif service_type and service_type == s['type']:
                return s['id']
        return None

    def service_exists(self, service_name=None, service_type=None):
        """Determine if the given service exists on the service list"""
        return self.resolve_service_id(service_name, service_type) is not None


class KeystoneManager2(KeystoneManager):

    def __init__(self, endpoint, **kwargs):
        try:
            from keystoneclient.v2_0 import client
            from keystoneclient.auth.identity import v2
            from keystoneclient import session
        except ImportError:
            if six.PY2:
                apt_install(["python-keystoneclient"], fatal=True)
            else:
                apt_install(["python3-keystoneclient"], fatal=True)

            from keystoneclient.v2_0 import client
            from keystoneclient.auth.identity import v2
            from keystoneclient import session

        self.api_version = 2

        token = kwargs.get("token", None)
        if token:
            api = client.Client(endpoint=endpoint, token=token)
        else:
            auth = v2.Password(username=kwargs.get("username"),
                               password=kwargs.get("password"),
                               tenant_name=kwargs.get("tenant_name"),
                               auth_url=endpoint)
            sess = session.Session(auth=auth)
            api = client.Client(session=sess)

        self.api = api


class KeystoneManager3(KeystoneManager):

    def __init__(self, endpoint, **kwargs):
        try:
            from keystoneclient.v3 import client
            from keystoneclient.auth import token_endpoint
            from keystoneclient import session
            from keystoneclient.auth.identity import v3
        except ImportError:
            if six.PY2:
                apt_install(["python-keystoneclient"], fatal=True)
            else:
                apt_install(["python3-keystoneclient"], fatal=True)

            from keystoneclient.v3 import client
            from keystoneclient.auth import token_endpoint
            from keystoneclient import session
            from keystoneclient.auth.identity import v3

        self.api_version = 3

        token = kwargs.get("token", None)
        if token:
            auth = token_endpoint.Token(endpoint=endpoint,
                                        token=token)
            sess = session.Session(auth=auth)
        else:
            auth = v3.Password(auth_url=endpoint,
                               user_id=kwargs.get("username"),
                               password=kwargs.get("password"),
                               project_id=kwargs.get("tenant_name"))
            sess = session.Session(auth=auth)

        self.api = client.Client(session=sess)
