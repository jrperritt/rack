.. _home:

Rackspace Command Line Interface
================================

Description
-----------

The Rackspace Command Line Interface is a unified tool to manage your Rackspace
services. It provides streamlined and secure configuration as well as a single
point of entry for all Rackspace Cloud services.


Quickstart
----------

For full instructions on how to get started you should read :ref:`installation_and_configuration`.

The tl;dr version is to grab the binary for your platform:

* `Mac OSX (64 bit)`_
* `Linux (64 bit)`_
* `Windows (64 bit)`_

Once downloaded, you need to make it executable. If you are unfamiliar with this
or are running Windows, please see :ref:`installation_and_configuration`.

Next, move into the directory where you downloaded the ``rack`` binary and run::

    ./rack configure

This command will automatically create a configuration file for you if it
doesn't exist and walk you through creating a profile for it::

    This interactive session will walk you through creating
    a profile in your configuration file. You may fill in all or none of the
    values.

    Rackspace Username: iamacat
    Rackspace API key: secrets
    Rackspace Region: IAD
    Profile Name (leave blank to create a default profile):

This allows you to immediately get working::

    ./rack servers instance list

If the ``rack`` binary isn't on your system's PATH, you'll only be able to run it from the
directory in which it resides. To run ``rack`` from anywhere, move the binary into a directory
on your systems PATH. Then, you'll be able to run it from anywhere::

    rack servers instance list


Synopsis
--------

::

    rack <service> <subservice> <action> [flags]

All ``rack`` commands follow the pattern above - for example, if you wanted to
list all servers on your Rackspace account, you would type::

  rack servers instance list

And the response (**default**: table-based output) would look like::

      ID	Name		Status	Public IPv4	Private IPv4	Image	Flavor
      GUID	my_server	ACTIVE	101.130.19.31	10.208.128.233	GUID	io1-30

Global Options
--------------

The ``rack`` CLI uses global options to alter output, authenticate, or
pass in other **global** information into the tool. These options are
called global because they are valid for any command. To see these, see :ref:`global_options`.

Services
--------

* :ref:`servers` - Commands for Rackspace Cloud Servers, dedicated and virtual.
* :ref:`files` - Commands for Rackspace Cloud Files.
* :ref:`networks` - Commands for Rackspace Cloud Networks.
* :ref:`block_storage` - Commands for Rackspace Block Storage.
* :ref:`orchestration` - Commands for Rackspace Cloud Orchestration


.. toctree::
   :caption: Contents
   :name: mastertoc
   :maxdepth: 2

   configuration.rst
   globaloptions.rst
   services/index.rst
   troubleshooting.rst


Indices and tables
==================

* :ref:`genindex`
* :ref:`search`

.. _Github project: https://github.com/rackspace/rack
.. _Mac OSX (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Darwin/amd64/rack
.. _Linux (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Linux/amd64/rack
.. _Windows (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Windows/amd64/rack.exe
.. _Cloud Control panel: https://mycloud.rackspace.com/
