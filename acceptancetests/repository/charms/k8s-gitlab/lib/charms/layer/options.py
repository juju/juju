import os
from pathlib import Path

import yaml


_CHARM_PATH = Path(os.environ.get('JUJU_CHARM_DIR', '.'))
_DEFAULT_FILE = _CHARM_PATH / 'layer.yaml'
_CACHE = {}


def get(section=None, option=None, layer_file=_DEFAULT_FILE):
    if option and not section:
        raise ValueError('Cannot specify option without section')

    layer_file = (_CHARM_PATH / layer_file).resolve()
    if layer_file not in _CACHE:
        with layer_file.open() as fp:
            _CACHE[layer_file] = yaml.safe_load(fp.read())

    data = _CACHE[layer_file].get('options', {})
    if section:
        data = data.get(section, {})
    if option:
        data = data.get(option)
    return data
