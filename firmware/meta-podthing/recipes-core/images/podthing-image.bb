SUMMARY = "Independent PodThing firmware for Spotify Car Thing"
DESCRIPTION = "Native iPod controller, Weston, ALSA, and Car Thing BSP; no Prelude application or filesystem dependency."
LICENSE = "MIT"

inherit core-image superbird-image superbird-headroom-check mainline-flashthing

IMAGE_FEATURES += "post-install-logging"
IMAGE_INSTALL = " \
    packagegroup-core-boot \
    packagegroup-superbird-runtime \
    podthing-app \
    weston \
    superbird-weston-init-kiosk \
    blank-cursor \
    cursor-suppress \
    mesa \
    alsa-lib \
    kernel-module-snd-usb-audio \
    kernel-module-usbhid \
    kernel-module-usb-storage \
    kernel-module-sd-mod \
"

BAD_RECOMMENDATIONS += "kernel-modules udev-hwdb wpa-supplicant wireless-regdb wireless-regdb-static weston-init"
IMAGE_FSTYPES = "wic ext4"
EXTRA_IMAGECMD:ext4 = "-i 16384 -m 0 -O ^has_journal -O ^huge_file"
SUPERBIRD_WKS_ROOTFS_FSTYPE = "ext4"
WKS_FILE = "superbird-mainline.wks.in"

do_image_wic[depends] += " \
    virtual/kernel:do_deploy \
    superbird-uenv:do_deploy \
    superbird-extlinux:do_deploy \
    superbird-logo:do_deploy \
"
EXTRA_IMAGEDEPENDS += "superbird-uboot"

# Physical USB recovery is available from the touchscreen/Back-at-boot.
# SSH and ADB are optional debugging facilities, not install requirements.
ROOTFS_POSTPROCESS_COMMAND:append = " podthing_no_remote_login; "
podthing_no_remote_login() {
    if [ -f ${IMAGE_ROOTFS}${sysconfdir}/shadow ]; then
        sed -i 's/^root:[^:]*:/root:!:/' ${IMAGE_ROOTFS}${sysconfdir}/shadow
    fi
}
