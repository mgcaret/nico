# Nico - Neon816 Integrated Console

Nico is, in a nutshell, a cross-platform terminal program meant for the
[Neon816](https://hackaday.io/project/164325-neon816).  In addition to
supporting the serial port of the Neon816 for use with the original
firmware and [OF816](https://github.com/mgcaret/of816), Nico supports
the Neon816 debug interface in the same window, allowing the user to
interact with both the running software and the debug interface in a 
convenient manner.

Nico is a Curses-based terminal program written in Go.

## Features

* ANSI terminal emulation.
* Many keyboard shortcuts.
* Compiles on MacOS X and Linux, and probably Windows.
* Character pacing to avoid overrunning the Neon816's buffers when
  pasting text.

## Building

You will need Go and the the following libraries:

* ``github.com/mgcaret/goncurses``
* ``github.com/jacobsa/go-serial/serial``
* ``github.com/marcinbor85/gohex``

Additionally you will need NCurses and the appropriate development 
headers/libraries for NCurses and its dependencies.

If everything is right, ``go build`` in the project directory should
give you a ``nico`` binary.

Alternatively, you may try the ``build.sh`` script, which will build
static binaries where supported, and will attempt to work around any
of the caveats listed below.

### Build Caveats

#### MacOS X

When using Homebrew's ``ncurses``, installing the goncurses library
and/or building Nico may fail. If it does, set ``LIBRARY_PATH``, e.g.:

``export LIBRARY_PATH=/usr/local/Cellar/ncurses/6.2/lib``

The ``build.sh`` script attempts to automatically compile with the above
variables set.

## Operation

``nico [options] <console-device> [<debug-device>]``

Where the devices are typically serial ports, however may also be
Unix domain sockets (for development purposes).  Additionally,
``console-device`` may be ``test`` which will execute Nico in a
test/demo mode (and ``debug-device`` is ignored).

### Options

``-console-baud <rate>``

Set console baud (default 9600)

``-debug-baud <rate>``

Set console baud (default 57600)

``-no-debug``

Disable debug/command interface entirely, leaving the whole screen
for the console.

### The Nico Interfaces 

Nico will start up quickly and draw the screen.  Unless ``-no-debug``
was given on the command line, the display is divided by a
horizontal line.  Above the line is the ANSI terminal interface, which
is connected to ``console-device``, and below is the debug/command
interface.  The debug/command interface will be of limited utility if
``debug-device`` was not specified, and ``--no-debug`` will omit it.

The bottom line of the debug/command interface is the command-line
input.

Both interfaces accept several keystrokes, documented further below.

Depending on your system configuration, you may move the cursor from
the terminal interface and the command-line input  with one of the
following keystrokes or sequences: Alt+Tab, F2, or ^] then Tab.

### The ANSI Terminal Interface

The ANSI terminal interface sends each character typed to the
``console-device``, and displays characters and control sequences
that it receives from the ``console-device``.

### The Debug/Command Interface

The debug/command interface accepts commands.  If ``debug-device``
was specified, this includes all of the commands supported by
Lenore Byron's ``neonprog`` distributed for the Neon816, plus
some additional commands.

#### Available Commands

##### Commands always available

``quit`` - quit the program

``help`` - small help text for keyboard shortcuts.

##### Commands available when ``debug-device`` was given

``resync`` - discards bytes in the read buffer, use when commands
like read return parse errors, or if you power-cycle your Neon.   

``mapram`` - map bank $80 RAM to bank $00

``read <addr>...`` - read $40 bytes of memory at the given address(es)

``write <addr> <byte>...`` - write byte(s) to the given address

``stop`` - stop execution

``step`` - single-step the PCU

``cont``, ``go`` - continue execution

``reset`` - reset the system

``run`` - reset and begin execution

``program <hex-file>`` - write Intel hex file data to RAM

``verify <hex-file>`` - verify programmed Intel hex file

``chipid`` - identify the flash ROM chip (stops CPU, use cont or go to
resume)

``flash <hex-file>`` - Erase ROM and flash with Intel Hex file data,
the addresses of the hex file are forced into the flash ROM range.

``verify-rom <hex-file>`` - verify flashed Intel hex file

``erase`` - Erase ROM.

### Keyboard Shortcuts

**Note:** Your system or terminal program may intercept some key combinations
listed below.  In many cases, multiple combinations perform the same function
so you should be able to find one that works if you cannot reconfigure the
intercepting software.

#### Accepted everywhere

*Ctrl+]* - Start command key.  The following command keys may follow:
  
  * Tab - swap between terminal and debug/command interfaces
  * c - clear terminal portion
  * d - clear debug/command portion
  * h - pop some help text into debug interface
  * q - quit the program
  * Esc - cancel command
 
If you let it time out or press Ctrl+] again while in the terminal
interface, it will send it through to the Neon816.

*F1* - same as Ctrl+], h

*F2*, *Alt+Tab* - swap between terminal and debug/command interfaces.

*F10* - quit program

If ``debug-device`` was specified:

*Alt+S* - send 'stop' command

*Alt+Space* - send 'step' command.

*Alt+G* - send 'go'/'cont' command

*Alt+R* - send 'run' command

*Alt+`* - send 'reset' command

#### Accepted in the debug/command input

The arrow keys and *Home*/*End* keys do what they normally do.

Bash-style cursor movement is also available.

*CANCEL* (if your keyboard has it) - clear input

*Ctrl+A* move to the beginning

*Ctrl+E* move to the end

*Ctrl+K* clear input to the right of the cursor

*Ctrl+L* clear debug/command output area

*CLEAR*, *Ctrl+X* - if the cursor is at right end of the line, clear the line
if in the middle, clear to the right of the cursor.

*DELETE CHAR*, *Ctrl+D* - delete the character under the cursor

*Alt+B* - go back a word

*Alt+F* - go forward a word

*Ctrl+Y*, *Alt+Enter* - send the current contents of the debug input
to ``console-device`` and clear the input.

*Alt+Y* - send the current contents of the debug input
to ``console-device`` and leave the input untouched.

## Bugs and Caveats

Nico doesn't currently support resizing its window.  If you do, things
might get weird.

The debug interface sometimes gets confused if you power off and then
on the Neon816 when port is open.  This manifests as many errors being
reported, or data from read being out of known alignment (often by
1/2 byte).  If this happes, execute the ``resync`` debug command. 

Nico sometimes exits and leaves the terminal in raw mode.  Unix/Linux
systems provide the ``reset`` command to fix this.
