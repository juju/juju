# Creating a New CI Test

Test scripts will be run under many conditions to reproduce real cases.
Most scripts cannot assume special knowledge of the substrate, region,
bootstrap constraints, tear down, and log collection, etc.

You can base your new script and its unit tests on the template files.
They provide the infrastructure to setup and tear down a test. Your script
can focus on the unique aspects of your test. Start by making a copy of
template_assess.py.tmpl.

    make new-assess name=my_function

Run make lint early and often. (You may need to do sudo apt-get install python-
flake8).

If your tests require new charms, please write them in Python.
