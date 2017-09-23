go-luks-suspend
===============

A package for [Arch Linux][] to lock LUKS encrypted volumes on suspend.

When using [dm-crypt with LUKS][] to set up full system encryption, the
encryption key is kept in memory when suspending the system. This drawback
defeats the purpose of encryption if you are ever physically separated from
your machine. One can use the `cryptsetup luksSuspend` command to freeze all
I/O and flush the key from memory, but special care must be taken when
applying it to the root device.

The `go-luks-suspend` program replaces the default suspend mechanism of
systemd. It chroots to initramfs in order to perform the `luksSuspend`,
suspend to RAM, and `luksResume` operations. It relies on the `shutdown`
initcpio hook to provide access to the initramfs.

This project is a rewrite of Vianney le Clément's excellent project
[arch-luks-suspend][] in the Go programming language, and features the
following improvements:

- All non-root LUKS volumes are locked on suspend.

- Root LUKS volumes can be unlocked with a keyfile. (Press `CTRL-R` at the
  prompt to unlock the root volume with a keyfile stored on a removable
  device. See [`cryptkey`][cryptkey].)

- Non-root LUKS volumes with keyfiles specified in `/etc/crypttab` are
  concurrently unlocked on wake.

- Press `Escape` to re-suspend the system after wake without having to unlock
  it first. ([N.B.][escape])

[Arch Linux]: https://www.archlinux.org/
[dm-crypt with LUKS]: https://wiki.archlinux.org/index.php/Dm-crypt_with_LUKS
[arch-luks-suspend]: https://github.com/vianney/arch-luks-suspend
[cryptkey]: https://wiki.archlinux.org/index.php/Dm-crypt/System_configuration#cryptkey
[escape]: https://github.com/guns/go-luks-suspend#q-my-system-doesnt-re-suspend-with-the-escape-key-after-wake-but-before-unlock


Installation
------------

1. Install this AUR package: https://aur.archlinux.org/packages/go-luks-suspend/<br>
   Alternatively, run `make install` as root.

2. Edit `/etc/mkinitcpio.conf` and make sure the following hooks are enabled:<br>
   `udev`, `encrypt`, `shutdown`, and `suspend`.

3. Rebuild the initramfs: `mkinitcpio -p linux`.

4. Enable the service: `systemctl enable go-luks-suspend.service`

5. Reboot.


Q. How do I unlock non-root LUKS volumes on wake?
-------------------------------------------------

A. `go-luks-suspend` locks all active LUKS volumes on the system, but will
only prompt the user to unlock the root volume on wake.

To unlock a non-root LUKS volume on wake, add an entry with a keyfile in
`/etc/crypttab`:

```ini
# /etc/crypttab
#
#<name>   <device>                                   <keyfile>           <options>
crypt-01  UUID=51932da0-6da1-4e92-9c2e-fc0063b2fcdb  /root/crypt-01.key  luks
crypt-02  UUID=4bf96ca0-8d10-47e9-bf57-aea2c72a472d  /root/crypt-02.key  luks
crypt-03  UUID=7a790264-34a3-40d7-837f-b76271710e2a  /root/crypt-03.key  luks
```

In the example above, `crypt-01`, `crypt-02`, and `crypt-03` will be unlocked
concurrently on wake after the user successfully unlocks the root volume with
a passphrase.


Q. How do I poweroff the system on errors?
------------------------------------------

A. The `-poweroff` flag instructs `go-luks-suspend` to power off the machine
on error or when the user fails to unlock the root volume on wake. To add this
flag to the `go-luks-suspend` command line:

1. Override the service file:

```
# systemctl edit go-luks-suspend.service
```

2. Redefine the `ExecStart` entry with the `-poweroff` flag:

```ini
[Service]
ExecStart=
ExecStart=/usr/bin/openvt -ws -- /usr/lib/go-luks-suspend/go-luks-suspend -poweroff
```


Q. My system doesn't re-suspend with the Escape key after wake but before unlock!
---------------------------------------------------------------------------------

A. The kernel calls [`thaw_processes()`][thaw] after waking the system from
suspend. This wakes up all processes on the system, any of which may initiate
IO with a locked LUKS volume.

These processes, in turn, refuse to be frozen by `freeze_processes()`, which
is called during the system suspend sequence. Because the kernel refuses to
suspend the system until the hanging processes are frozen, the only way to
re-suspend the system at this point is unlock the affected LUKS volume, let
the IO complete, and try again.

In practice, network IO after wake is the largest reason that suspend fails
after-wake-but-before-unlock. It is therefore recommended that you bring down
the machine's network interfaces before suspend and restore them on wake.

[thaw]: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/Documentation/power/freezing-of-tasks.txt


Q. How do I run go-luks-suspend in debug mode?
----------------------------------------------

A. Run `go-luks-suspend` with the `-debug` flag to print debugging messages
and to spawn a rescue shell on errors.

```
# /usr/lib/go-luks-suspend/go-luks-suspend -debug
```


Authors and license
-------------------

Copyright 2017 Sung Pae <self@sungpae.com> (Go implementation)

Copyright 2013 Vianney le Clément de Saint-Marcq <vleclement@gmail.com>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; version 3 of the License.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with This program.  If not, see <http://www.gnu.org/licenses/>.
