from setuptools import setup


with open('jujupy-description.rst') as f:
    long_description = f.read()


setup(
    name='jujupy',
    version='0.9.0',
    description='A library for driving the Juju client.',
    long_description=long_description,
    packages=['jujupy'],
    install_requires=[
        'python-dateutil >= 2',
        'pexpect >= 4.0.0',
        'PyYAML >= 3.0',
        ],
    author='The Juju QA Team',
    author_email='juju-qa@lists.canonical.com',
    url='https://launchpad.net/juju-ci-tools',
    license='Apache 2',
    classifiers=[
        "License :: OSI Approved :: GNU Lesser General Public License v3"
        " (LGPLv3)",
        "Development Status :: 5 - Production/Stable",
        "Intended Audience :: Developers",
        "Programming Language :: Python",
        "Programming Language :: Python :: 2.7",
        "Programming Language :: Python :: 3.5",
        "Operating System :: POSIX :: Linux",
        "Operating System :: Microsoft :: Windows",
        "Operating System :: MacOS :: MacOS X"
    ],
    )
