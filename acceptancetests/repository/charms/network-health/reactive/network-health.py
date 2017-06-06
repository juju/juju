#!/usr/bin/python3
from charms.reactive import when_not, set_state
from charmhelpers.core.hookenv import (
    status_set,
    open_port,
    log,
    unit_private_ip,
    INFO
    )
import subprocess
import os


@when_not('network-health.installed')
def install_network_health():
    status_set('maintenance', 'Removing sshguard')
    subprocess.call(['sudo', 'apt-get', 'remove', 'sshguard'])
    status_set('active', 'Started')
    set_state('network-health.installed')


@when_not('network-health.simple-http-started')
def start_simple_http():
    script = 'scripts.simple-server'
    file_path = os.path.join(os.environ['CHARM_DIR'], 'files/token.txt')
    port = 8039
    ip = unit_private_ip()
    log('Starting simple http server on: {}:{}'.format(
        ip, port), INFO)
    os.system('sudo python3 -m {} --file-path {} --port {} >> '
              '/tmp/server-output.log &'.format(script, file_path, port))
    open_port(port)
    set_state('network-health.simple-http-started')
