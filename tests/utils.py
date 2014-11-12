from contextlib import contextmanager
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


def get_random_hex_string(size=64):
    return ''.join(random.choice(string.hexdigits) for n in range(size))
