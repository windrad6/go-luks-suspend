.PHONY: all install

INSTALL_DIR := /usr/lib/arch-luks-suspend

all: arch-luks-suspend initramfs-suspend

arch-luks-suspend:
	GOPATH="$(CURDIR)" go build archLuksSuspend/cmd/arch-luks-suspend

initramfs-suspend:
	GOPATH="$(CURDIR)" go build archLuksSuspend/cmd/initramfs-suspend

install: all
	install -Dm755 arch-luks-suspend "$(DESTDIR)$(INSTALL_DIR)/arch-luks-suspend"
	install -Dm755 initramfs-suspend "$(DESTDIR)$(INSTALL_DIR)/initramfs-suspend"
	install -Dm644 initcpio-hook "$(DESTDIR)/usr/lib/initcpio/install/suspend"
	install -Dm644 systemd-suspend.service "$(DESTDIR)/etc/systemd/system/systemd-suspend.service"

clean:
	rm -f arch-luks-suspend initramfs-suspend

# vim:set sw=4 ts=4 noet:
