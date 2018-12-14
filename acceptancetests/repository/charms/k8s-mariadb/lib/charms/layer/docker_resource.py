import yaml
from pathlib import Path

from charmhelpers.core import hookenv
from charmhelpers.core import unitdata
from charms.reactive import set_flag, clear_flag, toggle_flag, is_flag_set
from charms.reactive import data_changed

from charms import layer


class DockerImageInfo:
    def __init__(self, data):
        self._registry_path = data['registrypath']
        self._username = data['username']
        self._password = data['password']

    @property
    def registry_path(self):
        return self._registry_path

    @property
    def username(self):
        return self._username

    @property
    def password(self):
        return self._password


def fetch(resource_name):
    queue = unitdata.kv().get('layer.docker-resource.pending', [])
    queue.append(resource_name)
    unitdata.kv().set('layer.docker-resource.pending', queue)
    set_flag('layer.docker-resource.{}.fetched'.format(resource_name))
    set_flag('layer.docker-resource.pending')


def _fetch():
    should_set_status = layer.options.get('docker-resource', 'set-status')
    queue = unitdata.kv().get('layer.docker-resource.pending', [])
    failed = []
    for res_name in queue:
        prefix = 'layer.docker-resource.{}'.format(res_name)
        if should_set_status:
            layer.status.maintenance('fetching resource: {}'.format(res_name))
        try:
            image_info_filename = hookenv.resource_get(res_name)
            if not image_info_filename:
                raise ValueError('no filename returned')
            image_info = yaml.safe_load(Path(image_info_filename).read_text())
            if not image_info:
                raise ValueError('no data returned')
        except Exception as e:
            hookenv.log(
                'unable to fetch docker resource {}: {}'.format(res_name, e),
                level=hookenv.ERROR)
            failed.append(res_name)
            set_flag('{}.failed'.format(prefix))
            clear_flag('{}.available'.format(prefix))
            clear_flag('{}.changed'.format(prefix))
        else:
            unitdata.kv().set('{}.image-info'.format(prefix), image_info)
            was_available = is_flag_set('{}.available'.format(prefix))
            is_changed = data_changed(prefix, image_info)
            set_flag('{}.available'.format(prefix))
            clear_flag('{}.failed'.format(prefix))
            toggle_flag('{}.changed'.format(prefix),
                        was_available and is_changed)
    if failed:
        if should_set_status:
            pl = 's' if len(failed) > 1 else ''
            layer.status.blocked(
                'unable to fetch resource{}: {}'.format(
                    pl, ', '.join(failed)
                )
            )
        unitdata.kv().set('layer.docker-resource.pending', failed)
        set_flag('layer.docker-resource.pending')
    else:
        unitdata.kv().set('layer.docker-resource.pending', [])
        clear_flag('layer.docker-resource.pending')


def get_info(resource_name):
    key = 'layer.docker-resource.{}.image-info'.format(resource_name)
    data = unitdata.kv().get(key)
    if not data:
        raise ValueError('resource {} not available'.format(resource_name))
    return DockerImageInfo(data)
