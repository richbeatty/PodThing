#!/usr/bin/env python3
"""Boot the real ARM64 kernel/rootfs in QEMU; substitute only physical devices.

This verifies generic kernel boot and the OS/app startup, not Car Thing board
initialization, its display, USB controller, accessory protocol or audio.
"""
import json
import os
import pathlib
import subprocess
import time
import urllib.error
import urllib.request

root = pathlib.Path(__file__).resolve().parent.parent
out = root / 'build/direct'
common = ['docker', 'run', '--rm', '--user', '0', '-v', f'{root}:/work', '-w', '/work']
subprocess.run(common + ['--entrypoint', '/bin/sh', 'podthing-direct-builder', 'firmware/direct/prepare-boot-smoke.sh'], check=True)
command = common + [
    '--name', 'podthing-qemu-boot', '--network=host', '--entrypoint', 'qemu-system-aarch64', 'podthing-direct-builder',
    '-machine', 'virt', '-cpu', 'cortex-a53', '-m', '512M', '-smp', '4', '-display', 'none', '-no-reboot', '-serial', 'stdio',
    '-kernel', '/work/build/direct/kernel-out/arch/arm64/boot/Image',
    '-drive', 'if=none,format=raw,id=root,file=/work/build/direct/rootfs-smoke.img', '-device', 'virtio-blk-device,drive=root',
    '-netdev', 'user,id=n0,hostfwd=tcp:127.0.0.1:18787-:8787,hostfwd=tcp:127.0.0.1:18788-:8788', '-device', 'virtio-net-device,netdev=n0',
    '-append', 'root=/dev/vda rw rootwait rootfstype=ext4 console=ttyAMA0,115200 net.ifnames=0 '
    'ip=10.0.2.15::10.0.2.2:255.255.255.0:podthing:eth0:off '
    'systemd.mask=podthing-usb.service '
    'systemd.mask=podthing-boot-healthy.timer systemd.mask=seatd.service',
]
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
def request(path, action=None):
    body = None if action is None else json.dumps(action).encode()
    req = urllib.request.Request('http://127.0.0.1:18787'+path, data=body, headers={'Content-Type': 'application/json'})
    with opener.open(req, timeout=5) as response:
        return response.read()

with (out / 'qemu-boot.log').open('w') as log:
    process = subprocess.Popen(command, stdout=log, stderr=subprocess.STDOUT)
    try:
        deadline = time.monotonic()+300
        while time.monotonic() < deadline:
            if process.poll() is not None:
                raise SystemExit('QEMU exited before app startup; see build/direct/qemu-boot.log')
            try:
                state = json.loads(request('/api/state'))
                break
            except (OSError, urllib.error.URLError, json.JSONDecodeError):
                time.sleep(2)
        else:
            raise SystemExit('ARM64 boot smoke timed out; see build/direct/qemu-boot.log')
        if not state.get('demo'):
            raise SystemExit('Boot smoke did not start explicitly labeled demo mode')
        expected = os.environ.get('PODTHING_VERSION')
        if expected and state.get('version') != expected:
            raise SystemExit(f"Expected firmware {expected}, got {state.get('version')} in ARM64 image")
        previous_frame = request('/frame.png')
        request('/api/action', {'name': 'navigate', 'text': 'Songs'})
        after = json.loads(request('/api/state'))
        if after.get('category') != 'Songs' or not after.get('entries'):
            raise SystemExit('ARM64 browse action failed')
        for attempt in range(30):
            frame = request('/frame.png')
            if frame != previous_frame:
                break
            time.sleep(0.5)
        else:
            raise SystemExit('ARM64 frame did not update after browsing')
        if not frame.startswith(b'\x89PNG\r\n\x1a\n'):
            raise SystemExit('ARM64 app did not produce a rendered screen')
        with opener.open('http://127.0.0.1:18788/cgi-bin/boot-log', timeout=15) as response:
            bootlog = response.read()
        if b'PodThing boot diagnostics' not in bootlog or b'Linux podthing' not in bootlog:
            raise SystemExit('Independent boot diagnostic download failed')
        (out / 'qemu-recovery-log.txt').write_bytes(bootlog)
        (out / 'qemu-boot-frame.png').write_bytes(frame)
        (out / 'qemu-boot-state.json').write_text(json.dumps(state, indent=2)+'\n')
        print('PASS: Cortex-A53 kernel boot, ext4 root, systemd app startup, ARM64 Weston/Wayland presentation, HTTP action, PNG rendering and recovery log')
        print('Physical display/USB/iPod/audio remain untested.')
    finally:
        subprocess.run(['docker', 'stop', '--timeout', '2', 'podthing-qemu-boot'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
