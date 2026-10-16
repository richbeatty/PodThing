#!/usr/bin/env python3
"""Assemble real filesystem images into an aligned, checked GPT disk."""
import json
import pathlib
import subprocess
import sys

out = pathlib.Path('/work/build/direct')
layout = [('env', 'env.img', 8), ('boot_a', 'boot.img', 64),
          ('root_a', 'rootfs.img', 768), ('boot_b', 'boot.img', 64),
          ('root_b', 'rootfs.img', 768), ('bandaid', 'bandaid.img', 192)]
disk = out / 'superbird.wic'
size = (8 + sum(item[2] for item in layout)) * 1024 * 1024
with disk.open('wb') as stream:
    stream.truncate(size)
lines = ['label: gpt', 'unit: sectors', '']
start = 8192
for name, filename, mib in layout:
    sectors = mib * 2048
    # FAT boot/env are Basic Data; Linux rootfs have Linux filesystem type.
    kind = 'EBD0A0A2-B9E5-4433-87C0-68B6B72699C7' if name == 'env' or name.startswith('boot_') else '0FC63DAF-8483-4772-8E79-3D69D8477DE4'
    lines.append(f'start={start},size={sectors},type={kind},name="{name}"')
    start += sectors
subprocess.run(['sfdisk', str(disk)], input='\n'.join(lines)+'\n', text=True, check=True)
parts = json.loads(subprocess.check_output(['sfdisk', '--json', str(disk)]))['partitiontable']['partitions']
with disk.open('r+b') as dest:
    for part, (name, filename, mib) in zip(parts, layout):
        source = out / filename
        if source.stat().st_size != mib * 1024 * 1024:
            raise SystemExit(f'Incorrect {name} filesystem image size')
        dest.seek(part['start'] * 512)
        with source.open('rb') as stream:
            # Skip zero blocks so the assembled disk stays sparse on the host.
            while block := stream.read(1024 * 1024):
                if block.strip(b'\0'):
                    dest.write(block)
                else:
                    dest.seek(len(block), 1)
subprocess.run(['sgdisk', '--verify', str(disk)], check=True)
sys.path.insert(0, '/work/scripts')
from importlib import import_module
with disk.open('rb') as stream:
    result = import_module('validate-firmware').check_gpt(stream, size)
print(json.dumps(result, indent=2))
