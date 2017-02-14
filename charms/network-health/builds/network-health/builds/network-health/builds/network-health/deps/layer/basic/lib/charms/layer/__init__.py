import os


class LayerOptions(dict):
    def __init__(self, layer_file, section=None):
        import yaml  # defer, might not be available until bootstrap
        with open(layer_file) as f:
            layer = yaml.safe_load(f.read())
        opts = layer.get('options', {})
        if section and section in opts:
            super(LayerOptions, self).__init__(opts.get(section))
        else:
            super(LayerOptions, self).__init__(opts)


def options(section=None, layer_file=None):
    if not layer_file:
        base_dir = os.environ.get('CHARM_DIR', os.getcwd())
        layer_file = os.path.join(base_dir, 'layer.yaml')

    return LayerOptions(layer_file, section)
