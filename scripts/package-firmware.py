#!/usr/bin/env python3
"""Package completed, real boot/root filesystems for Terbium. Never flash."""
import datetime
import hashlib
import json
import os
import pathlib
import subprocess
import zipfile

root = pathlib.Path(__file__).resolve().parent.parent
out = root / 'build/direct'
dist = root / 'dist'
version = '0.1.0'
release_version = os.environ.get('PODTHING_VERSION', '0.1.0-test')
artifacts = ['superbird-boot.bin', 'superbird.wic']
for name in artifacts:
    if not (out / name).is_file():
        raise SystemExit(f'Missing completed firmware artifact: {name}')

def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        while block := stream.read(1024 * 1024):
            h.update(block)
    return h.hexdigest()

meta = {
    'metadataVersion': 2, 'name': 'podthing-image', 'version': release_version,
    'description': 'Independent PodThing iPod controller firmware for Spotify Car Thing; hardware-test build.',
    'steps': [
        {'type': 'bulkcmd', 'value': 'amlmmc key'},
        *[{'type': 'writeBootPartition', 'value': {'hwpart': n, 'data': {'filePath': 'superbird-boot.bin'}}} for n in (1, 2)],
        {'type': 'writeUserArea', 'value': {'lba': 0, 'data': {'filePath': 'superbird.wic'}, 'sparse': True}},
    ],
}
info = {
    'version': release_version, 'built_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
    'source_commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip(),
    'architecture': 'aarch64', 'kernel': (out / 'kernel-out/include/config/kernel.release').read_text().strip(),
    'sources': json.loads((root / 'firmware/direct/sources.json').read_text()),
    'artifact_sha256': {name: sha(out / name) for name in artifacts},
    'app_sha256': sha(dist / 'podthing-linux-arm64'),
    'physical_car_thing_boot_tested': False, 'physical_ipod_control_tested': False,
    'physical_ipod_audio_tested': False,
    'app_source_sha256': {},
}
for directory in ['cmd', 'internal', 'firmware', 'scripts']:
    for path in sorted((root / directory).rglob('*')):
        if path.is_file() and '__pycache__' not in path.parts:
            info['app_source_sha256'][str(path.relative_to(root))] = sha(path)
for name in ['go.mod', 'go.sum', 'THIRD_PARTY.md']:
    info['app_source_sha256'][name] = sha(root / name)
(out / 'meta.json').write_text(json.dumps(meta, indent=2) + '\n')
(out / 'BUILD-INFO.json').write_text(json.dumps(info, indent=2) + '\n')
destination = dist / f'podthing-{version}-flashthing.zip'
temporary = destination.with_suffix('.zip.partial')
with zipfile.ZipFile(temporary, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=6, allowZip64=True) as archive:
    for name in ['meta.json', 'BUILD-INFO.json'] + artifacts:
        archive.write(out / name, name)
    archive.write(out / 'manifests/packages.tsv', 'packages.tsv')
    archive.write(out / 'manifests/packages.sha256', 'packages.sha256')
    archive.write(root / 'docs/FIRMWARE.md', 'INSTALL.md')
    archive.write(root / 'THIRD_PARTY.md', 'THIRD_PARTY.md')
os.replace(temporary, destination)
print(destination)
print(f'{destination.stat().st_size:,} bytes')
