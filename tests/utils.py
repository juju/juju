from contextlib import contextmanager
import shutil
from tempfile import mkdtemp


@contextmanager
def temp_dir():
    dirname = mkdtemp()
    try:
        yield dirname
    finally:
        shutil.rmtree(dirname)
