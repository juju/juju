#
# Copyright 2012 Canonical Ltd.
#
# This file is sourced from lp:openstack-charm-helpers
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Paul Collins <paul.collins@canonical.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

import json
import os
import subprocess
import socket
import sys


def do_hooks(hooks):
    hook = os.path.basename(sys.argv[0])

    try:
        hook_func = hooks[hook]
    except KeyError:
        juju_log('INFO',
                 "This charm doesn't know how to handle '{}'.".format(hook))
    else:
        hook_func()


def install(*pkgs):
    cmd = [
        'apt-get',
        '-y',
        'install'
          ]
    for pkg in pkgs:
        cmd.append(pkg)
    subprocess.check_call(cmd)

TEMPLATES_DIR = 'templates'

try:
    import jinja2
except ImportError:
    install('python-jinja2')
    import jinja2

try:
    import dns.resolver
except ImportError:
    install('python-dnspython')
    import dns.resolver


def render_template(template_name, context, template_dir=TEMPLATES_DIR):
    templates = jinja2.Environment(
                    loader=jinja2.FileSystemLoader(template_dir)
                    )
    template = templates.get_template(template_name)
    return template.render(context)

CLOUD_ARCHIVE = \
""" # Ubuntu Cloud Archive
deb http://ubuntu-cloud.archive.canonical.com/ubuntu {} main
"""

CLOUD_ARCHIVE_POCKETS = {
    'folsom': 'precise-updates/folsom',
    'folsom/updates': 'precise-updates/folsom',
    'folsom/proposed': 'precise-proposed/folsom',
    'grizzly': 'precise-updates/grizzly',
    'grizzly/updates': 'precise-updates/grizzly',
    'grizzly/proposed': 'precise-proposed/grizzly'
    }


def configure_source():
    source = str(config_get('openstack-origin'))
    if not source:
        return
    if source.startswith('ppa:'):
        cmd = [
            'add-apt-repository',
            source
            ]
        subprocess.check_call(cmd)
    if source.startswith('cloud:'):
        install('ubuntu-cloud-keyring')
        pocket = source.split(':')[1]
        with open('/etc/apt/sources.list.d/cloud-archive.list', 'w') as apt:
            apt.write(CLOUD_ARCHIVE.format(CLOUD_ARCHIVE_POCKETS[pocket]))
    if source.startswith('deb'):
        l = len(source.split('|'))
        if l == 2:
            (apt_line, key) = source.split('|')
            cmd = [
                'apt-key',
                'adv', '--keyserver keyserver.ubuntu.com',
                '--recv-keys', key
                ]
            subprocess.check_call(cmd)
        elif l == 1:
            apt_line = source

        with open('/etc/apt/sources.list.d/quantum.list', 'w') as apt:
            apt.write(apt_line + "\n")
    cmd = [
        'apt-get',
        'update'
        ]
    subprocess.check_call(cmd)

# Protocols
TCP = 'TCP'
UDP = 'UDP'


def expose(port, protocol='TCP'):
    cmd = [
        'open-port',
        '{}/{}'.format(port, protocol)
        ]
    subprocess.check_call(cmd)


def juju_log(severity, message):
    cmd = [
        'juju-log',
        '--log-level', severity,
        message
        ]
    subprocess.check_call(cmd)


def relation_ids(relation):
    cmd = [
        'relation-ids',
        relation
        ]
    result = str(subprocess.check_output(cmd)).split()
    if result == "":
        return None
    else:
        return result


def relation_list(rid):
    cmd = [
        'relation-list',
        '-r', rid,
        ]
    result = str(subprocess.check_output(cmd)).split()
    if result == "":
        return None
    else:
        return result


def relation_get(attribute, unit=None, rid=None):
    cmd = [
        'relation-get',
        ]
    if rid:
        cmd.append('-r')
        cmd.append(rid)
    cmd.append(attribute)
    if unit:
        cmd.append(unit)
    value = subprocess.check_output(cmd).strip()  # IGNORE:E1103
    if value == "":
        return None
    else:
        return value


def relation_set(**kwargs):
    cmd = [
        'relation-set'
        ]
    args = []
    for k, v in kwargs.items():
        if k == 'rid':
            if v:
                cmd.append('-r')
                cmd.append(v)
        else:
            args.append('{}={}'.format(k, v))
    cmd += args
    subprocess.check_call(cmd)


def unit_get(attribute):
    cmd = [
        'unit-get',
        attribute
        ]
    value = subprocess.check_output(cmd).strip()  # IGNORE:E1103
    if value == "":
        return None
    else:
        return value


def config_get(attribute):
    cmd = [
        'config-get',
        '--format',
        'json',
        ]
    out = subprocess.check_output(cmd).strip()  # IGNORE:E1103
    cfg = json.loads(out)

    try:
        return cfg[attribute]
    except KeyError:
        return None


def get_unit_hostname():
    return socket.gethostname()


def get_host_ip(hostname=unit_get('private-address')):
    try:
        # Test to see if already an IPv4 address
        socket.inet_aton(hostname)
        return hostname
    except socket.error:
        answers = dns.resolver.query(hostname, 'A')
        if answers:
            return answers[0].address
    return None


def _svc_control(service, action):
    subprocess.check_call(['service', service, action])


def restart(*services):
    for service in services:
        _svc_control(service, 'restart')


def stop(*services):
    for service in services:
        _svc_control(service, 'stop')


def start(*services):
    for service in services:
        _svc_control(service, 'start')


def reload(*services):
    for service in services:
        try:
            _svc_control(service, 'reload')
        except subprocess.CalledProcessError:
            # Reload failed - either service does not support reload
            # or it was not running - restart will fixup most things
            _svc_control(service, 'restart')


def running(service):
    try:
        output = subprocess.check_output(['service', service, 'status'])
    except subprocess.CalledProcessError:
        return False
    else:
        if ("start/running" in output or
            "is running" in output):
            return True
        else:
            return False


def is_relation_made(relation, key='private-address'):
    for r_id in (relation_ids(relation) or []):
        for unit in (relation_list(r_id) or []):
            if relation_get(key, rid=r_id, unit=unit):
                return True
    return False
