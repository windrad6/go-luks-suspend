# Maintainer: guns <self@sungpae.com>
# Contributor: Vianney le Clément de Saint-Marcq <vleclement AT gmail·com>
pkgname=arch-luks-suspend-nerv
pkgver=
pkgrel=1
pkgdesc='Custom arch-luks-suspend build'
arch=('any')
url="https://github.com/guns/arch-luks-suspend"
license=('GPL3')
groups=('nerv')
depends=('systemd' 'cryptsetup' 'mkinitcpio')
makedepends=('git')
backup=('etc/systemd/system/systemd-suspend.service')
install=install
provides=('arch-luks-suspend' 'arch-luks-suspend-git')
conflicts=('arch-luks-suspend' 'arch-luks-suspend-git')
replaces=('arch-luks-suspend-git')

pkgver() {
  cd "$startdir"
  _date=$(git show -s --format='%ci' | cut -d' ' -f1 | sed 's/-//g')
  _hash=$(git show -s --format='%h')
  echo "$_date.g$_hash"
}

package() {
  cd "$startdir"
  make DESTDIR="$pkgdir/" install
}

# vim:set ts=2 sw=2 et:
