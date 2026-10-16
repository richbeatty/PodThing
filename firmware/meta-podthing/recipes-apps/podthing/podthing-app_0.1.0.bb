SUMMARY = "PodThing native iPod head-unit application"
# The repository owner has not selected an application license. Dependency
# license notices are supplied separately; this does not relicense their work.
LICENSE = "CLOSED"

FILESEXTRAPATHS:prepend := "${PODTHING_SOURCE_DIR}/dist:"
SRC_URI = " \
    file://podthing-linux-arm64 \
    file://licenses/go-runtime.txt \
    file://licenses/golang-x-image.txt \
    file://licenses/golang-x-text.txt \
    file://licenses/go-fonts.txt \
    file://licenses/purego.txt \
    file://podthing.service \
    file://podthing-start \
"
S = "${UNPACKDIR}"

inherit systemd
SYSTEMD_SERVICE:${PN} = "podthing.service"
SYSTEMD_AUTO_ENABLE:${PN} = "enable"
RDEPENDS:${PN} = "alsa-lib superbird-weston-init-kiosk superbird-usb-role"
COMPATIBLE_MACHINE = "^superbird$"
INSANE_SKIP:${PN} += "already-stripped"

do_install() {
    install -d ${D}${libdir}/podthing ${D}${bindir} ${D}${systemd_system_unitdir} ${D}${datadir}/licenses/podthing
    install -m 0755 ${S}/podthing-linux-arm64 ${D}${libdir}/podthing/podthing
    ln -s ${libdir}/podthing/podthing ${D}${bindir}/podthing
    install -m 0755 ${S}/podthing-start ${D}${libdir}/podthing/podthing-start
    install -m 0644 ${S}/podthing.service ${D}${systemd_system_unitdir}/podthing.service
    install -m 0644 ${S}/licenses/*.txt ${D}${datadir}/licenses/podthing/
}
FILES:${PN} += "${libdir}/podthing ${datadir}/licenses/podthing"
