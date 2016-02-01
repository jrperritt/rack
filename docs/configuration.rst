.. _installation_and_configuration:

Installation and Configuration
==============================

Installation
------------

The Rackspace CLI ``rack`` is a self-contained binary written in go_. This means
that installation is as simple as downloading the relevant binary for your
operating system and ensuring it is on your path.

Binary Downloads:

* `Mac OSX (64 bit)`_
* `Linux (64 bit)`_
* `Windows (64 bit)`_

OSX and Linux with Homebrew
^^^^^^^^^^^^^^^^^^^^^^^^^^^

If you are using `Homebrew`_, run the following command::

    brew install rack

OSX and Linux (without Homebrew)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

After downloading the binary on OSX and Linux you will need to make the binary
executable by typing::

    chmod a+x /path/to/rack

We also recommend you move or symbolically link it on these platforms to `/usr/local/bin`::

    mkdir -p /usr/local/bin/
    ln -s /path/to/rack /usr/local/bin/rack

You can now add it to your path with::

    export PATH=$PATH:/usr/local/bin

Windows with Chocolatey
^^^^^^^^^^^^^^^^^^^^^^^

If you are using `Chocolatey`_, run the following command::

    choco install rack

Windows (without Chocolatey)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Option 1 : Manual
~~~~~~~~~~~~~~~~~

After downloading the binary on Windows, you can immediately run it.

We recommend that you copy it to a location outside of your Downloads folder (e.g. C:\\tools) and add that location to your PATH. You must open a new command prompt after modifying the PATH variable.

1. Create a new directory for command line tools, if you don't already have one, e.g. C:\\tools.
2. Copy rack.exe to that directory
3. Add the directory to your user's PATH environment variable, e.g. ``setx path "%path%;C:\tools"`` or press the Windows key, type "set env", select "Edit environment variables for your account", select the PATH user variable and append ";C:\\tools" to the value and save your changes.
4. Open a new command prompt after modifying the PATH variable.

Option 2 : Script
~~~~~~~~~~~~~~~~~

Requires Powershell version 3 or above.

The following commands will set up Rackspace CLI. First, open Powershell_ise and paste the following script in the scripting pane, then click on the green play button to start the execution. Saving the script as a powershell file eg: rackspace-cli.ps1 and executing it will also set up Rackspace CLI on your windows computer.

::

  #requires -Version 3
  $DownloadPath = 'C:\Tools'

  Write-Output -InputObject "[$(Get-Date)] Status  :: Set the Tools Directory $DownloadPath"
  New-Item -Path $DownloadPath -ItemType Directory -ErrorAction SilentlyContinue > $null
  Set-Location -Path $DownloadPath -ErrorAction SilentlyContinue

  Write-Output -InputObject "[$(Get-Date)] Status  :: Download Rackspace CLI in C:\Tools"
  Invoke-WebRequest -Uri 'https://goo.gl/NMvmcx/Windows/amd64/rack.exe' -Method Get -OutFile rack.exe

  Write-Output -InputObject "[$(Get-Date)] Status  :: Unblock the executable file rack.exe"
  Unblock-File -Path $("$DownloadPath\rack.exe")

  Write-Output -InputObject "[$(Get-Date)] Status  :: Permanently set the path $DownloadPath to the Environment variable (Reboot required)."
  [System.Environment]::SetEnvironmentVariable('Path', $env:Path + 'C:\Tools', [System.EnvironmentVariableTarget]::Machine)
  Write-Output -InputObject "[$(Get-Date)] Status  :: Temporarily set the path $DownloadPath to the Environment variable for immediate use in the current powershell session"
  $env:Path += ';C:\Tools'




Configuration
-------------

To authenticate against the Rackspace API, there are 4 required paramaters:

* Username: a Rackspace username
* API key: a Rackspace API key
* Region: a Rackspace region
* Authentication Endpoint (URL): (Optional/Advanced) A URL to send the authentication request.


If this is your first time using the ``rack`` CLI, we recommend you
run the interactive ``configure`` command.

.. note::
   Windows users should use PowerShell, not PowerShell ISE to run this
   command.

``configure`` will automatically create a configuration file for you if it
doesn't exist and walk you through creating a profile for it::

    rack configure

    This interactive session will walk you through creating
    a profile in your configuration file. You may fill in all or none of the
    values.

    Rackspace Username: iamacat
    Rackspace API key: secrets
    Rackspace Region: IAD
    Profile Name (leave blank to create a default profile):

This allows you to immediately get working::

    rack servers instance list


Otherwise, ``rack`` lets you provide these parameters in a few different ways:

Command-line Options
^^^^^^^^^^^^^^^^^^^^

If provided, command-line authentication flags will take precedence over any
other parameters located in any other forms of authentication (config file and
environment variables).

* ``--username``
* ``--api-key``
* ``--region``

Config File
^^^^^^^^^^^

If provided, any authentication parameters not set on the command-line will be
looked for in a config file. The config file should be located in ``$HOME/.rack/config``.
The config file format is like the following::

    username=<your rackspace username>
    api-key=<your rackspace api key>
    region=<the rackspace region>

    [another-profile]
    username=<another rackspace username>
    api-key=<another rackspace api key>

In the example above there is a default profile that doesn't have a named section. "another-profile" is a different profile in the config file. When using the default profile, you don't need to supply a flag when executing ``rack``. A specific profile can be specified on the command-line with the ``profile`` flag.

::

    rack servers instance list --profile another-profile

Note that none of the authentication parameters
have to be set in the config file. Parameters not set there will be looked for elsewhere.


Environment Variables
^^^^^^^^^^^^^^^^^^^^^

Finally, ``rack`` will look for any remaining unset authentication parameters
in environment variables. The following are values are permitted (case matters):

* ``RS_REGION_NAME`` (DFW, IAD, ORD, LON, SYD, HKG)
* ``RS_USERNAME`` (Your Rackspace username)
* ``RS_API_KEY`` (Your Rackspace API key)

So, for example on OSX and Linux; you would type::

    export RS_REGION_NAME=IAD
    export RS_USERNAME=<your rackspace username>
    export RS_API_KEY=<secrets>

On Windows you would type::

    set RS_REGION_NAME=IAD
    set RS_USERNAME=<your rackspace username>
    set RS_API_KEY=<secrets>

You can get your API key by logging into the `Cloud Control panel`_ and clicking
on *account -> account settings* and clicking "show" next to "API Key". Be careful;
this key is special. Don't share it!

Command Completion
------------------
Run ``rack init`` to set up Bash command completion. Currently, this is only
available for the Bash shell. If you're using a Linux OS, ``rack init`` will look for,
and, if found, amend ``$HOME/.bashrc`` to enable Bash completion. If you're on a
Darwin OS (like Mac), it will look for ``$HOME/.bash_profile``.

If you'd like to set up command completion yourself (or if you're on a Windows OS and using a Bash shell),
you can copy file in the location below to the appropriate directory and source it:
`https://github.com/rackspace/rack/blob/master/setup/commandcompletion_bash.sh`

If you are using PowerShell and want command-completion, you can run the ``commandcompletion_posh.ps1`` script in the
``setup`` directory. That script will perform normal command-completion for non-rack commands, and rack-specific
completions for ``rack`` commands. A few caveats for PowerShell users:
* The script overrides the ``global:TabExpansion2`` function.
* This should work for PowerShell versions greater than or equal to 3, but it was tested with PowerShell_ISE v4.
* You will get the normal Windows command-completion (with a circular buffer).

Check the version
-----------------

To see the current version, run::

    rack version

    rack version 0.0.0-dev
    commit: d69f4d2030b307076ad0a10f4b5addf440493aec

Advanced Configuration Values
-----------------------------

Identity Endpoint
^^^^^^^^^^^^^^^^^

If you require pointing to a custom Cloud Identity endpoint; you can set the
following environment variable:

* ``RS_AUTH_URL`` (https://identity.api.rackspacecloud.com/v2.0)

For example::

    export RS_AUTH_URL=https://identity.api.rackspacecloud.com/v2.0

In addition, you may provide it as a flag on the command-line or as a value in a
config file profile. In either case, the parameter name will be ``auth-url``.




.. _go: https://golang.org/
.. _Mac OSX (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Darwin/amd64/rack
.. _Linux (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Linux/amd64/rack
.. _Windows (64 bit): https://ec4a542dbf90c03b9f75-b342aba65414ad802720b41e8159cf45.ssl.cf5.rackcdn.com/1.1.0-beta1/Windows/amd64/rack.exe
.. _Homebrew: http://brew.sh
.. _Chocolatey: http://chocolatey.org
.. _Cloud Control panel: https://mycloud.rackspace.com/
