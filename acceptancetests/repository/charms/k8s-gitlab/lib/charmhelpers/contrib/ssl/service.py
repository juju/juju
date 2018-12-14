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

import os
from os.path import join as path_join
from os.path import exists
import subprocess

from charmhelpers.core.hookenv import log, DEBUG

STD_CERT = "standard"

# Mysql server is fairly picky about cert creation
# and types, spec its creation separately for now.
MYSQL_CERT = "mysql"


class ServiceCA(object):

    default_expiry = str(365 * 2)
    default_ca_expiry = str(365 * 6)

    def __init__(self, name, ca_dir, cert_type=STD_CERT):
        self.name = name
        self.ca_dir = ca_dir
        self.cert_type = cert_type

    ###############
    # Hook Helper API
    @staticmethod
    def get_ca(type=STD_CERT):
        service_name = os.environ['JUJU_UNIT_NAME'].split('/')[0]
        ca_path = os.path.join(os.environ['CHARM_DIR'], 'ca')
        ca = ServiceCA(service_name, ca_path, type)
        ca.init()
        return ca

    @classmethod
    def get_service_cert(cls, type=STD_CERT):
        service_name = os.environ['JUJU_UNIT_NAME'].split('/')[0]
        ca = cls.get_ca()
        crt, key = ca.get_or_create_cert(service_name)
        return crt, key, ca.get_ca_bundle()

    ###############

    def init(self):
        log("initializing service ca", level=DEBUG)
        if not exists(self.ca_dir):
            self._init_ca_dir(self.ca_dir)
            self._init_ca()

    @property
    def ca_key(self):
        return path_join(self.ca_dir, 'private', 'cacert.key')

    @property
    def ca_cert(self):
        return path_join(self.ca_dir, 'cacert.pem')

    @property
    def ca_conf(self):
        return path_join(self.ca_dir, 'ca.cnf')

    @property
    def signing_conf(self):
        return path_join(self.ca_dir, 'signing.cnf')

    def _init_ca_dir(self, ca_dir):
        os.mkdir(ca_dir)
        for i in ['certs', 'crl', 'newcerts', 'private']:
            sd = path_join(ca_dir, i)
            if not exists(sd):
                os.mkdir(sd)

        if not exists(path_join(ca_dir, 'serial')):
            with open(path_join(ca_dir, 'serial'), 'w') as fh:
                fh.write('02\n')

        if not exists(path_join(ca_dir, 'index.txt')):
            with open(path_join(ca_dir, 'index.txt'), 'w') as fh:
                fh.write('')

    def _init_ca(self):
        """Generate the root ca's cert and key.
        """
        if not exists(path_join(self.ca_dir, 'ca.cnf')):
            with open(path_join(self.ca_dir, 'ca.cnf'), 'w') as fh:
                fh.write(
                    CA_CONF_TEMPLATE % (self.get_conf_variables()))

        if not exists(path_join(self.ca_dir, 'signing.cnf')):
            with open(path_join(self.ca_dir, 'signing.cnf'), 'w') as fh:
                fh.write(
                    SIGNING_CONF_TEMPLATE % (self.get_conf_variables()))

        if exists(self.ca_cert) or exists(self.ca_key):
            raise RuntimeError("Initialized called when CA already exists")
        cmd = ['openssl', 'req', '-config', self.ca_conf,
               '-x509', '-nodes', '-newkey', 'rsa',
               '-days', self.default_ca_expiry,
               '-keyout', self.ca_key, '-out', self.ca_cert,
               '-outform', 'PEM']
        output = subprocess.check_output(cmd, stderr=subprocess.STDOUT)
        log("CA Init:\n %s" % output, level=DEBUG)

    def get_conf_variables(self):
        return dict(
            org_name="juju",
            org_unit_name="%s service" % self.name,
            common_name=self.name,
            ca_dir=self.ca_dir)

    def get_or_create_cert(self, common_name):
        if common_name in self:
            return self.get_certificate(common_name)
        return self.create_certificate(common_name)

    def create_certificate(self, common_name):
        if common_name in self:
            return self.get_certificate(common_name)
        key_p = path_join(self.ca_dir, "certs", "%s.key" % common_name)
        crt_p = path_join(self.ca_dir, "certs", "%s.crt" % common_name)
        csr_p = path_join(self.ca_dir, "certs", "%s.csr" % common_name)
        self._create_certificate(common_name, key_p, csr_p, crt_p)
        return self.get_certificate(common_name)

    def get_certificate(self, common_name):
        if common_name not in self:
            raise ValueError("No certificate for %s" % common_name)
        key_p = path_join(self.ca_dir, "certs", "%s.key" % common_name)
        crt_p = path_join(self.ca_dir, "certs", "%s.crt" % common_name)
        with open(crt_p) as fh:
            crt = fh.read()
        with open(key_p) as fh:
            key = fh.read()
        return crt, key

    def __contains__(self, common_name):
        crt_p = path_join(self.ca_dir, "certs", "%s.crt" % common_name)
        return exists(crt_p)

    def _create_certificate(self, common_name, key_p, csr_p, crt_p):
        template_vars = self.get_conf_variables()
        template_vars['common_name'] = common_name
        subj = '/O=%(org_name)s/OU=%(org_unit_name)s/CN=%(common_name)s' % (
            template_vars)

        log("CA Create Cert %s" % common_name, level=DEBUG)
        cmd = ['openssl', 'req', '-sha1', '-newkey', 'rsa:2048',
               '-nodes', '-days', self.default_expiry,
               '-keyout', key_p, '-out', csr_p, '-subj', subj]
        subprocess.check_call(cmd, stderr=subprocess.PIPE)
        cmd = ['openssl', 'rsa', '-in', key_p, '-out', key_p]
        subprocess.check_call(cmd, stderr=subprocess.PIPE)

        log("CA Sign Cert %s" % common_name, level=DEBUG)
        if self.cert_type == MYSQL_CERT:
            cmd = ['openssl', 'x509', '-req',
                   '-in', csr_p, '-days', self.default_expiry,
                   '-CA', self.ca_cert, '-CAkey', self.ca_key,
                   '-set_serial', '01', '-out', crt_p]
        else:
            cmd = ['openssl', 'ca', '-config', self.signing_conf,
                   '-extensions', 'req_extensions',
                   '-days', self.default_expiry, '-notext',
                   '-in', csr_p, '-out', crt_p, '-subj', subj, '-batch']
        log("running %s" % " ".join(cmd), level=DEBUG)
        subprocess.check_call(cmd, stderr=subprocess.PIPE)

    def get_ca_bundle(self):
        with open(self.ca_cert) as fh:
            return fh.read()


CA_CONF_TEMPLATE = """
[ ca ]
default_ca = CA_default

[ CA_default ]
dir                     = %(ca_dir)s
policy                  = policy_match
database                = $dir/index.txt
serial                  = $dir/serial
certs                   = $dir/certs
crl_dir                 = $dir/crl
new_certs_dir           = $dir/newcerts
certificate             = $dir/cacert.pem
private_key             = $dir/private/cacert.key
RANDFILE                = $dir/private/.rand
default_md              = default

[ req ]
default_bits            = 1024
default_md              = sha1

prompt                  = no
distinguished_name      = ca_distinguished_name

x509_extensions         = ca_extensions

[ ca_distinguished_name ]
organizationName        = %(org_name)s
organizationalUnitName  = %(org_unit_name)s Certificate Authority


[ policy_match ]
countryName             = optional
stateOrProvinceName     = optional
organizationName        = match
organizationalUnitName  = optional
commonName              = supplied

[ ca_extensions ]
basicConstraints        = critical,CA:true
subjectKeyIdentifier    = hash
authorityKeyIdentifier  = keyid:always, issuer
keyUsage                = cRLSign, keyCertSign
"""


SIGNING_CONF_TEMPLATE = """
[ ca ]
default_ca = CA_default

[ CA_default ]
dir                     = %(ca_dir)s
policy                  = policy_match
database                = $dir/index.txt
serial                  = $dir/serial
certs                   = $dir/certs
crl_dir                 = $dir/crl
new_certs_dir           = $dir/newcerts
certificate             = $dir/cacert.pem
private_key             = $dir/private/cacert.key
RANDFILE                = $dir/private/.rand
default_md              = default

[ req ]
default_bits            = 1024
default_md              = sha1

prompt                  = no
distinguished_name      = req_distinguished_name

x509_extensions         = req_extensions

[ req_distinguished_name ]
organizationName        = %(org_name)s
organizationalUnitName  = %(org_unit_name)s machine resources
commonName              = %(common_name)s

[ policy_match ]
countryName             = optional
stateOrProvinceName     = optional
organizationName        = match
organizationalUnitName  = optional
commonName              = supplied

[ req_extensions ]
basicConstraints        = CA:false
subjectKeyIdentifier    = hash
authorityKeyIdentifier  = keyid:always, issuer
keyUsage                = digitalSignature, keyEncipherment, keyAgreement
extendedKeyUsage        = serverAuth, clientAuth
"""
