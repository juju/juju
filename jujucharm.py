"""Helpers to create and manage local juju charms."""

import os

import yaml


__metaclass__ = type


class Charm:
    """Representation of a juju charm."""

    DEFAULT_MAINTAINER = "juju-qa@lists.canonical.com"
    DEFAULT_SERIES = ("xenial", "trusty")

    def __init__(self, name, summary, maintainer=None, series=None):
        self.metadata = {
            "name": name,
            "summary": summary,
            "maintainer": (
                self.DEFAULT_MAINTAINER if maintainer is None else maintainer),
        }
        self.series = self.DEFAULT_SERIES if series is None else series

    def to_dir(self, directory):
        """Serialize charm into a new directory."""
        with open(os.path.join(directory, "metadata.yaml"), "w") as f:
            yaml.safe_dump(self.metadata, f, default_flow_style=False)

    def to_repo_dir(self, repo_dir):
        """Serialize charm into a directory for a repository of charms."""
        charm_dir = os.path.join(repo_dir, self.series, self.metadata["name"])
        os.makedirs(charm_dir)
        self.to_dir(charm_dir)
        return charm_dir

    @property
    def series(self):
        series = self.metadata["series"]
        if series and isinstance(series, list):
            return series[0]
        return series

    @series.setter
    def series(self, series):
        self.metadata["series"] = series


def local_charm_path(charm, juju_ver, series=None, repository=None,
                     platform='ubuntu'):
    """Create either Juju 1.x or 2.x local charm path."""
    if juju_ver.startswith('1.'):
        if series:
            series = '{}/'.format(series)
        else:
            series = ''
        local_path = 'local:{}{}'.format(series, charm)
        return local_path
    else:
        charm_dir = {
            'ubuntu': 'charms',
            'win': 'charms-win',
            'centos': 'charms-centos'}
        abs_path = charm
        if repository:
            abs_path = os.path.join(repository, charm)
        elif os.environ.get('JUJU_REPOSITORY'):
            repository = os.path.join(
                os.environ['JUJU_REPOSITORY'], charm_dir[platform])
            abs_path = os.path.join(repository, charm)
        return abs_path


def make_charm(charm_dir, min_ver='1.25.0', name='dummy',
               description='description', summary='summary', series='trusty'):
    charm = Charm(name, summary, series=series)
    charm.metadata['description'] = description
    if min_ver is not None:
        charm.metadata['min-juju-version'] = min_ver
    if series is None:
        del charm.metadata["series"]
    charm.to_dir(charm_dir)
