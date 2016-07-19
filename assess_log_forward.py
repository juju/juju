#!/usr/bin/env python
"""Test Juju's log forwarding feature.

Log forwarding allows a controller to forward syslog from all models of a
controller to a syslog host via TCP (using SSL).

"""

from __future__ import print_function

import argparse
import logging
import os
import sys
import subprocess
import time
from textwrap import dedent
import yaml

from assess_model_migration import get_bootstrap_managers
import certificates
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_log_forward")


def assess_log_forward(bs_dummy, bs_rsyslog, upload_tools):
    check_logfwd_enabled_after_bootstrap(bs_dummy, bs_rsyslog, upload_tools)


def check_logfwd_enabled_after_bootstrap(bs_dummy, bs_rsyslog, upload_tools):
    """Ensure logs are forwarded after forwarding enabled after bootstrapping.

    Given 2 controllers set rsyslog and dummy:
      - setup rsyslog with secure details
      - Enable log forwarding on dummy
      - Ensure intial logs are present in the rsyslog sinks logs

    """
    with bs_rsyslog.booted_context(upload_tools):
        log.info('Bootstrapped rsyslog environment')
        rsyslog = bs_rsyslog.client
        rsyslog_details = deploy_rsyslog(rsyslog)

        update_client_config(bs_dummy.client, rsyslog_details)

        with bs_dummy.existing_booted_context(upload_tools):
            log.info('Bootstrapped dummy environment')
            dummy_client = bs_dummy.client

            # ensure turning on log-fwd emits logs to the client.
            # Should I ensure that nothing turns up beforehand
            ensure_enabling_log_forwarding_forwards_previous_messages(
                rsyslog, dummy_client)


def ensure_enabling_log_forwarding_forwards_previous_messages(rsyslog, dummy):
    """Assert when enabled log forwarding forwards messages from log start."""
    uuid = get_controller_uuid(dummy)

    enable_log_forwarding(dummy)
    assert_initial_message_forwarded(rsyslog, uuid)


def assert_initial_message_forwarded(rsyslog, uuid):
    """Assert that mention of the sources logs appear in the sinks logging.

    Given a rsyslog sink and an output source assert that logging details from
    the source appear in the sinks logging.
    Attempt a check over a period of time (10 seconds).

    :returns: As soon as the expected message appears.
    :raises JujuAssertionError: If the expected message does not appear in the
      given timeframe.
    :raises JujuAssertionError: If the log message check fails in an unexpected
      way.

    """
    check = get_assert_regex(uuid)

    for _ in range(0, 10):
        try:
            rsyslog.juju(
                'ssh',
                ('rsyslog/0', 'sudo', 'egrep', check, '/var/log/syslog'))
            # Success! No need to continue.
            break
        except subprocess.CalledProcessError as e:
            if e.returncode == 1:
                time.sleep(1)
            else:
                raise JujuAssertionError(
                    'Failed to parse the logs in an expected way.')
    else:
        # If we get here than the command never succeeded.
        raise JujuAssertionError('Forwarded log message never appeared.')


def get_assert_regex(uuid, message=None):
    # Maybe over simplified removing the last 8 characters
    short_uuid = uuid[:-8]
    date_check = '[A-Z][a-z]{,2}\ [0-9]+\ [0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}'
    machine = 'machine-0.{}'.format(uuid)
    agent = 'jujud-machine-agent-{}'.format(short_uuid)
    message = message or 'running\ jujud\ \[.*\]'

    return '"^{datecheck}\ {machine}\ {agent}\ {message}$"'.format(
        datecheck=date_check,
        machine=machine,
        agent=agent,
        message=message)


def enable_log_forwarding(client):
    client.juju(
        'set-model-config',
        ('-m', 'controller', 'logforward-enabled=true'), include_e=False)


def get_controller_uuid(client):
    name = client.env.controller.name
    output_yaml = client.get_juju_output(
        'show-controller', '--format', 'yaml', include_e=False)
    output = yaml.safe_load(output_yaml)
    return output[name]['details']['uuid']


def update_client_config(client, rsyslog_details):
    client.env.config['logforward-enabled'] = False
    client.env.config.update(rsyslog_details)


def deploy_rsyslog(client):
    """Deploy and setup the rsyslog charm on client.

    :returns: Configuration details needed: cert, ca, key and ip:port.

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

    ip_address = get_unit_ipaddress(client, unit_machine)

    client.juju(
        'ssh',
        (unit_machine, 'sudo apt-get install rsyslog-gnutls'))

    with temp_dir() as config_dir:
        install_rsyslog_config(client, config_dir, unit_machine)
        rsyslog_details = install_certificates(
            client, config_dir, ip_address, unit_machine)

    # restart rsyslog to take into affect all changes
    client.juju('ssh', (unit_machine, 'sudo', 'service', 'rsyslog', 'restart'))

    return rsyslog_details


def install_certificates(client, config_dir, ip_address, unit_machine):
    cert, key = certificates.create_certificate(config_dir, ip_address)

    # Write contents to file to scp across
    ca_file = os.path.join(config_dir, 'ca.pem')
    with open(ca_file, 'wt') as f:
        f.write(certificates.ca_pem_contents)

    scp_command = (
        '--', cert, key, ca_file, '{unit}:/home/ubuntu/'.format(
            unit=unit_machine))
    client.juju('scp', scp_command)

    return _get_rsyslog_details(cert, key, ip_address)


def _get_rsyslog_details(cert_file, key_file, ip_address):
    with open(cert_file, 'rt') as f:
        cert_contents = f.read()
    with open(key_file, 'rt') as f:
        key_contents = f.read()

    return {
        'syslog-host': '{}:10514'.format(ip_address),
        'syslog-ca-cert': certificates.ca_pem_contents,
        'syslog-client-cert': cert_contents,
        'syslog-client-key': key_contents
    }


def install_rsyslog_config(client, config_dir, unit_machine):
    config = write_rsyslog_config_file(config_dir)
    client.juju('scp', (config, '{unit}:/tmp'.format(unit=unit_machine)))
    client.juju(
        'ssh',
        (unit_machine, 'sudo', 'mv', '/tmp/{}'.format(
            os.path.basename(config)), '/etc/rsyslog.d/'))


def get_unit_ipaddress(client, unit_name):
    status = client.get_status()
    return status.get_unit(unit_name)['public-address']


def write_rsyslog_config_file(tmp_dir):
    """Write rsyslog config file to `tmp_dir`/10-securelogging.conf."""
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


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test log forwarding of logs.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_dummy, bs_rsyslog = get_bootstrap_managers(args)
    assess_log_forward(bs_dummy, bs_rsyslog, args.upload_tools)
    return 0


if __name__ == '__main__':
    sys.exit(main())
