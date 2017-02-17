#!/usr/bin/python
from charms.reactive import when, when_not, set_state


@when_not('network-health.installed')
def install_network_health():
    try:
        import charmhelpers  # noqa
    except ImportError:
        import subprocess
        subprocess.check_call(['apt-get', 'install', '-y', 'python-pip'])
        subprocess.check_call(['pip', 'install', 'charmhelpers'])
    set_state('network-health.installed')
