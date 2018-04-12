# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2018 Canonical Ltd.
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

from __future__ import print_function

from urllib2 import urlopen
from jujucharm import local_charm_path
import logging
import os
import requests

from jujupy.wait_condition import (
    AgentsIdle,
    AllApplicationActive,
    AllApplicationWorkloads,
    )
from jujupy.utility import (
    get_unit_public_ip,
    temp_dir,
)


__metaclass__ = type

log = logging.getLogger(__name__)


def deploy_mediawiki_with_db(client):
    client.deploy('cs:mysql')
    client.wait_for_started()
    client.wait_for_workloads()

    client.deploy('cs:mediawiki')
    client.wait_for_started()
    # mediawiki workload is blocked ('Database needed') until a db
    # relation is successfully made.
    client.juju('relate', ('mediawiki:db', 'mysql:db'))
    client.wait_for_workloads()
    client.wait_for_started()
    client.juju('expose', 'mediawiki')
    client.wait_for_workloads()
    client.wait_for_started()


def assert_mediawiki_is_responding(client):
    log.debug('Assert mediawiki is responding.')
    status = client.get_status()
    [wiki_unit_name] = [
        k for k, v in status.get_applications()['mediawiki']['units'].items()
        if v.get('leader', False)]
    wiki_ip = get_unit_public_ip(client, wiki_unit_name)
    resp = requests.get('http://{}'.format(wiki_ip))
    if not resp.ok:
        raise AssertionError('Mediawiki not responding; {}: {}'.format(
            resp.status_code, resp.reason
        ))
    if '<title>Please set name of wiki</title>' not in resp.content:
        raise AssertionError('Got unexpected mediawiki page content.')


def deploy_simple_server_to_new_model(
        client, model_name, resource_contents=None, series='xenial'):
    # As per bug LP:1709773 deploy 2 primary apps and have a subordinate
    #  related to both
    new_model = client.add_model(client.env.clone(model_name))
    application = deploy_simple_resource_server(
        new_model, resource_contents, series)
    _, deploy_complete = new_model.deploy('cs:ubuntu', series=series)
    new_model.wait_for(deploy_complete)
    new_model.deploy('cs:nrpe', series=series)
    new_model.juju('add-relation', ('nrpe', application))
    new_model.juju('add-relation', ('nrpe', 'ubuntu'))
    # Need to wait for the subordinate charms too.
    new_model.wait_for(AllApplicationActive())
    new_model.wait_for(AllApplicationWorkloads())
    new_model.wait_for(AgentsIdle(['nrpe/0', 'nrpe/1']))
    assert_deployed_charm_is_responding(new_model, resource_contents)

    return new_model, application


def deploy_simple_resource_server(
        client, resource_contents=None, series='xenial'):
    application_name = 'simple-resource-http'
    log.info('Deploying charm: '.format(application_name))
    charm_path = local_charm_path(
        charm=application_name, juju_ver=client.version)
    # Create a temp file which we'll use as the resource.
    if resource_contents is not None:
        with temp_dir() as temp:
            index_file = os.path.join(temp, 'index.html')
            with open(index_file, 'wt') as f:
                f.write(resource_contents)
            client.deploy(
                charm_path,
                series=series,
                resource='index={}'.format(index_file))
    else:
        client.deploy(charm_path, series=series)

    client.wait_for_started()
    client.wait_for_workloads()
    client.juju('expose', (application_name))
    return application_name


def deploy_dummy_source_to_new_model(client, model_name):
    new_model_client = client.add_model(client.env.clone(model_name))
    charm_path = local_charm_path(
        charm='dummy-source', juju_ver=new_model_client.version)
    new_model_client.deploy(charm_path)
    new_model_client.wait_for_started()
    new_model_client.set_config('dummy-source', {'token': 'one'})
    new_model_client.wait_for_workloads()
    return new_model_client


def assert_deployed_charm_is_responding(client, expected_output=None):
    """Ensure that the deployed simple-server charm is still responding."""
    # Set default value if needed.
    if expected_output is None:
        expected_output = 'simple-server.'
    ipaddress = get_unit_public_ip(client, 'simple-resource-http/0')
    if expected_output != get_server_response(ipaddress):
        raise AssertionError('Server charm is not responding as expected.')


def get_server_response(ipaddress):
    return urlopen('http://{}'.format(ipaddress)).read().rstrip()
