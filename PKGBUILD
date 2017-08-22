# Maintainer: guns <self@sungpae.com>
# Contributor: Vianney le Clément de Saint-Marcq <vleclement AT gmail·com>
pkgname=go-luks-suspend
pkgver=1.0.0
pkgrel=1
pkgdesc='Encrypt LUKS volumes on system suspend'
arch=('x86_64')
url="https://github.com/guns/go-luks-suspend"
license=('GPL3')
depends=('systemd' 'cryptsetup' 'mkinitcpio')
makedepends=('go' 'git')
backup=('etc/systemd/system/systemd-suspend.service')
install=install
conflicts=('arch-luks-suspend' 'arch-luks-suspend-git')
replaces=('arch-luks-suspend' 'arch-luks-suspend-git')

package() {
  cd "$startdir"
  make DESTDIR="$pkgdir/" install
}

# vim:set ts=2 sw=2 et:
