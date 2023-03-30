#!/usr/bin/env python3

from distutils.core import setup

setup(name='juju_txn_helper',
      version='0.1',
      description='Juju transaction helper library',
      author='Paul Goins (Canonical Ltd)',
      author_email='paul.goins@canonical.com',
      url='https://code.launchpad.net/~vultaire/+git/juju_txn_helper',
      scripts=['txn_helper.py'],
     )
