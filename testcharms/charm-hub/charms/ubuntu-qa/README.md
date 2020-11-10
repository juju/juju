# ubuntu-qa

## Description

A ubuntu charm to use in juju testing.  Has a few actions to play with.

## Usage

juju deploy ubuntu-qa

## Developing

Create and activate a virtualenv,
and install the development requirements,

    virtualenv -p python3 venv
    source venv/bin/activate
    pip install -r requirements-dev.txt

## Testing

Just run `run_tests`:

    ./run_tests
