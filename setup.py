from setuptools import setup
setup(
    name='jujupy',
    version='0.1.3',
    description='A library for driving the Juju client.',
    packages=['jujupy', 'utility', 'timeout'],
    install_requires=[
        'python-dateutil >= 2',
        'pexpect >= 4.0.0',
        'PyYAML >= 3.0',
        ]
    )
