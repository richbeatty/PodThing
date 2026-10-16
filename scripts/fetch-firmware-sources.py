#!/usr/bin/env python3
"""Fetch exact source commits and apply the pinned BSP kernel patch series."""
import hashlib
import json
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parent.parent
sources = json.loads((root / 'firmware/direct/sources.json').read_text())
for name, source in sources.items():
    dest = root / source['path']
    if not (dest / '.git').exists():
        dest.mkdir(parents=True, exist_ok=True)
        subprocess.run(['git', 'init', str(dest)], check=True)
        subprocess.run(['git', '-C', str(dest), 'remote', 'add', 'origin', source['url']], check=True)
        subprocess.run(['git', '-C', str(dest), 'fetch', '--depth=1', 'origin', source['commit']], check=True)
        subprocess.run(['git', '-C', str(dest), 'checkout', '--detach', source['commit']], check=True)
    actual = subprocess.check_output(['git', '-C', str(dest), 'rev-parse', 'HEAD'], text=True).strip()
    if actual != source['commit']:
        raise SystemExit(f'{name}: existing source has unexpected HEAD; preserving it')
    print(name, actual, flush=True)

bsp = root / sources['bsp']['path'] / 'meta-superbird/recipes-kernel/linux/linux-superbird'
kernel = root / sources['kernel']['path']
patches = sorted(bsp.glob('*.patch'))
if len(patches) != 25:
    raise SystemExit('Unexpected BSP patch series')
# Compose the complete expected tree in a temporary Git index. Later patches
# edit earlier patches, so reverse-checking each patch individually is invalid.
# The checkout's real Git index and user edits are never reset.
import os
import tempfile
with tempfile.TemporaryDirectory(prefix='podthing-patches-') as tmp:
    env = dict(os.environ, GIT_INDEX_FILE=str(pathlib.Path(tmp) / 'index'))
    cmd = ['git', '-C', str(kernel)]
    subprocess.run(cmd + ['read-tree', 'HEAD'], env=env, check=True)
    for patch in patches:
        subprocess.run(cmd + ['apply', '--cached', str(patch)], env=env, check=True)
    dtspath = 'arch/arm64/boot/dts/amlogic/meson-g12a-superbird.dts'
    makepath = 'arch/arm64/boot/dts/amlogic/Makefile'
    make = subprocess.check_output(cmd + ['show', 'HEAD:' + makepath])
    if b'meson-g12a-superbird.dtb' not in make:
        make += b'\ndtb-$(CONFIG_ARCH_MESON) += meson-g12a-superbird.dtb\n'
    for path, data in [(dtspath, (bsp / 'meson-g12a-superbird.dts').read_bytes()), (makepath, make)]:
        blob = subprocess.check_output(cmd + ['hash-object', '-w', '--stdin'], input=data).decode().strip()
        subprocess.run(cmd + ['update-index', '--add', '--cacheinfo', '100644', blob, path], env=env, check=True)
    expected = subprocess.check_output(cmd + ['diff', '--cached', '--binary', 'HEAD'], env=env)
    forward = subprocess.run(cmd + ['apply', '--check'], input=expected, capture_output=True)
    if forward.returncode == 0:
        subprocess.run(cmd + ['apply'], input=expected, check=True)
    else:
        subprocess.run(cmd + ['diff', '--exit-code'], env=env, check=True)
print('Pinned source and complete BSP patch verification passed')
