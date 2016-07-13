#!/usr/bin/env python
"""Test Jujus log forwarding feature.

Log forwarding allows a controller to forward syslog from all models of a
controller to a syslog host via TCP (using SSL).

"""

from __future__ import print_function

import argparse
from collections import namedtuple
import logging
from OpenSSL import crypto
import os
import sys
from textwrap import dedent

from assess_model_migration import get_bootstrap_managers
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_log_forward")


def assess_log_forward(bs1, bs2, upload_tools):
    ensure_log_forwarding_in_simple_setup(bs1, bs2, upload_tools)


def ensure_log_forwarding_in_simple_setup(bs_dummy, bs_rsyslog, upload_tools):
    """Simple check where enabling log forwarding results in logs at the target

    Given 2 controllers set rsyslog-host and dummy:
      - 1 up rsyslog (charm) on rsyslog-host
      - Enable log forwarding on dummy
      - Perform defined actions on dummy (that must show up in the log)
      - Check the log results in rsyslog contain expected output

    """
    with bs_rsyslog.booted_context(upload_tools):
        rsyslog_client = bs_rsyslog.client
        log.info('Bootstrapped rsyslog environment')
        rsyslog_details = deploy_rsyslog(rsyslog_client)

        update_client_config(bs_dummy.client, rsyslog_details)

        # configure the bootstrap to include the extra details for syslog
        # forwarding.
        with bs_dummy.existing_booted_context(upload_tools):
            dummy_client = bs_dummy.client
            log.info('Bootstrapped dummy environment')


def update_client_config(client, rsyslog_details):
    # This could be a simple update if the dict is sane
    client.env.config['logforward-enabled'] = False
    client.env.config['syslog-host'] = rsyslog_details['host']
    client.env.config['syslog-ca-cert'] = rsyslog_details['ca_cert']
    client.env.config['syslog-client-cert'] = rsyslog_details['client_cert']
    client.env.config['syslog-client-key'] = rsyslog_details['client_key']


def deploy_rsyslog(client):
    """Deploy and setup the rsyslog charm on client.

    Returns the configuration details i.e. certs, ca, key and ip:port.
    TODO: This could use a namedtuple for the details

    """
    # why doesn't the deploy name the application as expected?
    # app_name = 'rsyslog-sink'
    app_name = 'rsyslog'
    client.deploy('rsyslog', (app_name))
    client.wait_for_started()
    client.juju('set-config', (app_name, 'protocol="tcp"'))
    client.juju('expose', app_name)

    return setup_tls_rsyslog(client, app_name)


def setup_tls_rsyslog(client, app_name):
    unit_machine = '{}/0'.format(app_name)

    ip_address = get_units_ipaddress(client, unit_machine)

    client.juju(
        'ssh',
        (unit_machine, 'sudo apt-get install rsyslog-gnutls'))

    with temp_dir() as config_dir:
        copy_across_rsyslog_config(client, config_dir, unit_machine)

        cert, key = create_certificate(config_dir, ip_address)
        # Write actual ca file and copy it cert and keys to the unit.
        ca_file = os.path.join(config_dir, 'ca.pem')
        with open(ca_file, 'wt') as f:
            f.write(get_ca_pem_contents())
        scp_command = (
            '--', cert, key, ca_file, '{unit}:/home/ubuntu/'.format(
                unit=unit_machine))
        client.juju('scp', scp_command)

        with open(cert, 'rt') as f:
            cert_contents = f.read()
        with open(key, 'rt') as f:
            key_contents = f.read()

        rsyslog_details = dict(
            host='{}:10514'.format(ip_address),
            ca_cert=get_ca_pem_contents(),
            client_cert=cert_contents,
            client_key=key_contents
        )

    # restart rsyslog to take into affect all changes
    client.juju('ssh', (unit_machine, 'sudo', 'service', 'rsyslog', 'restart'))

    return rsyslog_details


def copy_across_rsyslog_config(client, config_dir, unit_machine):
    config = write_rsyslog_config_file(config_dir)
    client.juju('scp', (config, '{unit}:/tmp'.format(unit=unit_machine)))
    client.juju(
        'ssh',
        (unit_machine, 'sudo', 'mv', '/tmp/{}'.format(
            os.path.basename(config)), '/etc/rsyslog.d/'))


def get_units_ipaddress(client, unit_name):
    status = client.get_status()
    return status.get_unit(unit_name)['public-address']


def write_rsyslog_config_file(tmp_dir):
    """Write rsyslog config file to `tmp_dir`"""
    config = dedent("""\
    # make gtls driver the default
    $DefaultNetstreamDriver gtls

    # certificate files
    $DefaultNetstreamDriverCAFile /home/ubuntu/ca.pem
    $DefaultNetstreamDriverCertFile /home/ubuntu/cert.pem
    $DefaultNetstreamDriverKeyFile /home/ubuntu/key.pem

    $ModLoad imtcp # load TCP listener
    $InputTCPServerStreamDriverAuthMode x509/name
    $InputTCPServerStreamDriverPermittedPeer anyServer
    $InputTCPServerStreamDriverMode 1 # run driver in TLS-only mode
    $InputTCPServerRun 10514 # port 10514
    """)
    config_path = os.path.join(tmp_dir, '10-securelogging.conf')
    with open(config_path, 'wt') as f:
        f.write(config)
    return config_path


def create_certificate(target_dir, ip_address):
    """Generate a cert and key file incl. IP SAN for `ip_address`

    Creates a cert.pem and key.pem file signed with a known ca cert.
    The generated cert will contain a IP SAN (subject alternative name) that
    includes the ip address of the server. This is required for log-forwarding.

    :return: tuple containing generated cert, key filepath pair

    """
    ip_address = 'IP:{}'.format(ip_address)

    key = crypto.PKey()
    key.generate_key(crypto.TYPE_RSA, 2048)

    csr_contents = generate_csr(target_dir, key, ip_address)
    req = crypto.load_certificate_request(crypto.FILETYPE_PEM, csr_contents)

    ca_cert = crypto.load_certificate(
        crypto.FILETYPE_PEM, get_ca_pem_contents())
    ca_key = crypto.load_privatekey(
        crypto.FILETYPE_PEM, get_ca_key_pem_contents())

    cert = crypto.X509()
    cert.set_version(0x2)
    cert.set_subject(req.get_subject())
    cert.set_serial_number(1)
    cert.gmtime_adj_notBefore(0)
    cert.gmtime_adj_notAfter(24 * 60 * 60)
    cert.set_issuer(ca_cert.get_subject())
    cert.set_pubkey(req.get_pubkey())
    cert.add_extensions([
        crypto.X509Extension('subjectAltName', False, ip_address),
        crypto.X509Extension(
            'extendedKeyUsage', False, 'clientAuth, serverAuth'),
        crypto.X509Extension(
            'keyUsage', True, 'keyEncipherment'),
    ])
    cert.sign(ca_key, "sha256")

    cert_filepath = os.path.join(target_dir, 'cert.pem')
    with open(cert_filepath, 'wt') as f:
        f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, cert))

    key_filepath = os.path.join(target_dir, 'key.pem')
    with open(key_filepath, 'wt') as f:
        f.write(crypto.dump_privatekey(crypto.FILETYPE_PEM, key))

    return (cert_filepath, key_filepath)


def generate_csr(target_dir, key, ip_address):
    req = crypto.X509Req()
    req.set_version(0x2)
    req.get_subject().CN = "anyServer"
    # Add the IP SAN
    req.add_extensions([
        crypto.X509Extension("subjectAltName", False, ip_address)
    ])
    req.set_pubkey(key)
    req.sign(key, "sha256")

    return crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)


def get_ca_pem_contents():
    return dedent("""\
    -----BEGIN CERTIFICATE-----
    MIIEFTCCAn2gAwIBAgIBBzANBgkqhkiG9w0BAQsFADAjMRIwEAYDVQQDEwlhbnlT
    ZXJ2ZXIxDTALBgNVBAoTBGp1anUwHhcNMTYwNzExMDQyOTM1WhcNMjYwNzA5MDQy
    OTM1WjAjMRIwEAYDVQQDEwlhbnlTZXJ2ZXIxDTALBgNVBAoTBGp1anUwggGiMA0G
    CSqGSIb3DQEBAQUAA4IBjwAwggGKAoIBgQCn6OxY33yAirABoE4UaZJBOnQORIzC
    125R71E2TG5gSHjHKA70L0C3dgyWhW9wcyhUbXBuz8Oep2J7kHvzuUPw2AWXI+Y2
    c0afWVqfj5kuyUpGhXsqylyf7NDPFs8hwGA6ZCFS3oUAvX8awsVucklxGeZNXZNK
    ZFilXKaX1Z3soORmKFZzVfDRqDuofZ2E0tmPh9C5gQ8qswjdBnTrj+0rCnvNekO0
    aND6AlkBHU+87pvcax0uUF6PYkXxPikKk1ftCQSII5oB5ksAtRpcZsYl5hT3U/t1
    DOA7c35RuIx7ogkcXP9jZ6J2tkmX+GMtUF29KEEnVCht32VDX+C3yS6lbfQB4oDt
    Yp3wXRY/LXTW7XTUrhoXB4nkYbw59gis5Cr7zDtUpiWFVYgy/kbxalljSM4N3w2i
    dtfxJHYjTfK98124qbCBb4A4ZNBJE2jy//lSIcIMXJv1LXQtTqR4rO1j6TBurohF
    NmUYpy3Zv7gn2CkfX6QfNFIj8elKT6dd+RUCAwEAAaNUMFIwDwYDVR0TAQH/BAUw
    AwEB/zAPBgNVHREECDAGhwQKwoylMA8GA1UdDwEB/wQFAwMHBAAwHQYDVR0OBBYE
    FP+v8GAqHiUCIygXbwWzbUhl/22DMA0GCSqGSIb3DQEBCwUAA4IBgQBVYKeT1O2M
    U3OPOy0IwqcA1/64rS1GlRmiw+papmjsy3aru03r8igahnbFd7wQawHaCScXbI/n
    OAPT4PDGXn6b71t4uHwWaM8wde159RO3G32N/VfhV6LPRUQunmAZh5QcJK6wWpYu
    B1f0dPkU+Q1AfX12oTOX/ld2/o7jaVswHoHoW6K2WQmwzlRQ953J+RJ7jXfrYDKl
    OAp3Hb69wAN4Ayc1s92iYUwV5q8UaHQoskHOLWJu964yFBHL8SLe6TLD+Jjv05Mc
    Ca7NKq/n25VTDNNaXl5MCNZ048m/GGHfktxxCddaF2grhC5HTUetwkq026PE0Wcq
    P+cDrIq6uTA25QqyBYistSa/7z2o0NBi56ySRqxlP2J2TPFZyOb+ZiA4EgYY5no5
    u2E+WuKZLVWl7eaQYOHgfYzFf3CvalSBwIjNynRwD/2Ebk7K29GPrIugb3V2+Vwh
    rltUXOHUkFGjEHIhr8zixfCxh5OzPJMnJwCZZRYzMO0/0Gw7ll9DmH0=
    -----END CERTIFICATE-----
    """)


def get_ca_key_pem_contents():
    return dedent("""\
    -----BEGIN RSA PRIVATE KEY-----
    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
    -----END RSA PRIVATE KEY-----
    """)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test log forwarding of logs.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs1, bs2 = get_bootstrap_managers(args)
    assess_log_forward(bs1, bs2, args.upload_tools)
    return 0


if __name__ == '__main__':
    sys.exit(main())
