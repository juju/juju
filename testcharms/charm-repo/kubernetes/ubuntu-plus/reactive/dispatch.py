import yaml
import os
from charmhelpers.core.hookenv import (
    config,
    log,
    metadata,
    status_set,
)
from charms.reactive.flags import set_flag
from charms import layer
from charms.reactive import hook, when_not, when
from datetime import datetime

def dispatch():
    return 'JUJU_DISPATCH_HOOK' in os.environ


@hook('install')
def install():
    status_set('waiting', "Hello from install, it is {0}".format(date_string()))
    log("Hello from update-status")


@hook('start')
def start():
    status_set('waiting', "Hello from start, it is {0}".format(date_string()))
    log("Hello from start")


def date_string():
    now = datetime.now()
    return now.strftime("%d/%m/%Y %H:%M:%S")

@hook('update-status')
def update():
   status_set('active', "Hello from update-status, it is {0}".format(date_string()))
   log ("Hello from update-status")


@when_not('layer.docker-resource.ubuntu_image.fetched')
def fetch_image():
    layer.docker_resource.fetch('ubuntu_image')


@when('layer.docker-resource.ubuntu_image.available')
@when_not('dispatch.configured')
def dispatch_config():
    status_set('maintenance', 'Configuring dispatch container')

    spec = make_pod_spec()
    log('set pod spec:\n{}'.format(spec))
    layer.caas_base.pod_spec_set(spec)

    set_flag('dispatch.configured')
    status_set('active', 'pod spec set')


def make_pod_spec():
    with open('reactive/spec_template.yaml') as spec_file:
        pod_spec_template = spec_file.read()

    md = metadata()
    cfg = config()

    image_info = layer.docker_resource.get_info('ubuntu_image')

    data = {
        'name': md.get('name'),
        'docker_image_path': image_info.registry_path,
        'docker_image_username': image_info.username,
        'docker_image_password': image_info.password,
        'port': cfg.get('mysql_port'),
    }
    data.update(cfg)
    return pod_spec_template % data

