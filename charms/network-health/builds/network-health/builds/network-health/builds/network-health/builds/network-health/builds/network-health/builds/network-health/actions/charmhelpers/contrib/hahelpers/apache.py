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

#
# Copyright 2012 Canonical Ltd.
#
# This file is sourced from lp:openstack-charm-helpers
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

import os
import subprocess

from charmhelpers.core.hookenv import (
    config as config_get,
    relation_get,
    relation_ids,
    related_units as relation_list,
    log,
    INFO,
)


def get_cert(cn=None):
    # TODO: deal with multiple https endpoints via charm config
    cert = config_get('ssl_cert')
    key = config_get('ssl_key')
    if not (cert and key):
        log("Inspecting identity-service relations for SSL certificate.",
            level=INFO)
        cert = key = None
        if cn:
            ssl_cert_attr = 'ssl_cert_{}'.format(cn)
            ssl_key_attr = 'ssl_key_{}'.format(cn)
        else:
            ssl_cert_attr = 'ssl_cert'
            ssl_key_attr = 'ssl_key'
        for r_id in relation_ids('identity-service'):
            for unit in relation_list(r_id):
                if not cert:
                    cert = relation_get(ssl_cert_attr,
                                        rid=r_id, unit=unit)
                if not key:
                    key = relation_get(ssl_key_attr,
                                       rid=r_id, unit=unit)
    return (cert, key)


def get_ca_cert():
    ca_cert = config_get('ssl_ca')
    if ca_cert is None:
        log("Inspecting identity-service relations for CA SSL certificate.",
            level=INFO)
        for r_id in relation_ids('identity-service'):
            for unit in relation_list(r_id):
                if ca_cert is None:
                    ca_cert = relation_get('ca_cert',
                                           rid=r_id, unit=unit)
    return ca_cert


def retrieve_ca_cert(cert_file):
    cert = None
    if os.path.isfile(cert_file):
        with open(cert_file, 'r') as crt:
            cert = crt.read()
    return cert


def install_ca_cert(ca_cert):
    if ca_cert:
        cert_file = ('/usr/local/share/ca-certificates/'
                     'keystone_juju_ca_cert.crt')
        old_cert = retrieve_ca_cert(cert_file)
        if old_cert and old_cert == ca_cert:
            log("CA cert is the same as installed version", level=INFO)
        else:
            log("Installing new CA cert", level=INFO)
            with open(cert_file, 'w') as crt:
                crt.write(ca_cert)
            subprocess.check_call(['update-ca-certificates', '--fresh'])
