__metaclass__ = type

from collections import namedtuple
from contextlib import contextmanager
from datetime import datetime
from mock import patch
import os
import random
import shutil
import string
from tempfile import mkdtemp


@contextmanager
def temp_dir():
    dirname = mkdtemp()
    try:
        yield dirname
    finally:
        shutil.rmtree(dirname)


class until_timeout:

    """Yields remaining number of seconds.  Stops when timeout is reached.

    :ivar timeout: Number of seconds to wait.
    """
    def __init__(self, timeout, start=None):
        self.timeout = timeout
        if start is None:
            start = self.now()
        self.start = start

    def __iter__(self):
        return self

    @staticmethod
    def now():
        return datetime.now()

    def next(self):
        elapsed = self.now() - self.start
        remaining = self.timeout - elapsed.total_seconds()
        if remaining <= 0:
            raise StopIteration
        return remaining


Series = namedtuple('Series', ['version', 'name', 'status'])


class JujuSeries:

    def __init__(self, data_path=None):
        self.all = {}
        if data_path is None:
            data_path = os.path.join(
                os.path.dirname(__file__), 'supported-releases.txt')
        with open(data_path) as f:
            data = f.read()
        for line in data.splitlines():
            if line.startswith('#'):
                continue
            series = Series(*line.split())
            self.all[series.name] = series


def get_random_hex_string(size=64):
    return ''.join(random.choice(string.hexdigits) for n in range(size))


def autopatch(target, **kwargs):
    return patch(target, autospec=True, **kwargs)
