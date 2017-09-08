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
systemd. It changes root to initramfs in order to perform the `luksSuspend`,
suspend to RAM, and `luksResume` operations. It relies on the `shutdown`
initcpio hook to provide access to the initramfs.

This project is a rewrite of Vianney le Clément's excellent
[arch-luks-suspend][] in the Go programming language. Rewriting in Go provides
access to safe multithreading as well as better maintainability.

As a case in point, while `arch-luks-suspend` only locks the root device,
`go-luks-suspend` concurrently locks and unlocks all active LUKS volumes on
suspend and wake.

[Arch Linux]: https://www.archlinux.org/
[dm-crypt with LUKS]: https://wiki.archlinux.org/index.php/Dm-crypt_with_LUKS
[arch-luks-suspend]: https://github.com/vianney/arch-luks-suspend


Installation
------------

1. Install this AUR package: https://aur.archlinux.org/packages/go-luks-suspend/<br>
   Alternatively, run `make install` as root.

2. Edit `/etc/mkinitcpio.conf` and make sure the following hooks are enabled:<br>
   `udev`, `encrypt`, `shutdown`, `suspend`.

3. Rebuild the initramfs: `mkinitcpio -p linux`.

4. Enable the service: `systemctl enable go-luks-suspend.service`

5. Reboot.


Unlocking non-root LUKS volumes on wake
---------------------------------------

`go-luks-suspend` locks all active LUKS volumes on the system, but will only
unlock non-root LUKS volumes that have an entry in `/etc/crypttab` with a
corresponding keyfile:

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


Poweroff on error
-----------------

`go-luks-suspend` can power off the machine on error and when the user fails
to unlock the root volume on wake. This behavior can be enabled by adding the
`-poweroff` flag to the `ExecStart` line in the service file:

```ini
# /etc/systemd/system/systemd-suspend.service
…

[Service]
Type=oneshot
ExecStart=/usr/bin/openvt -ws /usr/lib/go-luks-suspend/go-luks-suspend -poweroff
```


Debug mode
----------

Running `go-luks-suspend` in debug mode prints debugging messages and spawns a
rescue shell on error.

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
