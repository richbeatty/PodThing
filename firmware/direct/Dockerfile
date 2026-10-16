FROM ghcr.io/siemens/kas/kas@sha256:3f3d3257e8d4ca3adfcb2e0025e6da24339fb8d68a6999ca36c37381351582d5
USER root
COPY ca-bundle.crt /etc/ssl/certs/ca-certificates.crt
ENV DEBIAN_FRONTEND=noninteractive
RUN rm -f /etc/apt/sources.list /etc/apt/sources.list.d/* && printf '%s\n' 'deb [signed-by=/usr/share/keyrings/debian-archive-keyring.gpg] https://deb.debian.org/debian trixie main' > /etc/apt/sources.list && apt-get -o Acquire::Languages=none -o APT::Update::Error-Mode=any update && apt-get install -y --no-install-recommends gcc-aarch64-linux-gnu g++-aarch64-linux-gnu bc bison flex libssl-dev libgnutls28-dev python3-pyelftools swig device-tree-compiler e2fsprogs dosfstools mtools gdisk fdisk rsync zip file cpio kmod busybox-static u-boot-tools qemu-user qemu-system-arm debootstrap systemd && rm -rf /var/lib/apt/lists/*
USER builder
