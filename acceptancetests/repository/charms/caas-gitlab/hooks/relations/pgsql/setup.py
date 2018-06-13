# setup.py for readthedocs. We need this to install into a venv so we
# can pull in dependencies.
from setuptools import setup

import os.path

if os.path.exists('test_requirements.txt'):
    reqs = open('test_requirements.txt', 'r').read().splitlines()
else:
    reqs = []


if __name__ == '__main__':
    setup(name='interface-pgsql',
          version='2.0.0',
          author='Stuart Bishop',
          author_email='stuart.bishop@canonical.com',
          license='GPL3',
          py_modules=['requires'],
          install_requires=reqs)
