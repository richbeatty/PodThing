#!/usr/bin/env python3
"""Keep the appliance's USB modules and their complete dependency closure."""
import pathlib
import subprocess
import sys
root = pathlib.Path(sys.argv[1])
release = sys.argv[2]
directory = root / 'usr/lib/modules' / release
if not directory.exists():
    directory = root / 'lib/modules' / release
modules = [p for p in directory.rglob('*') if p.name.endswith(('.ko', '.ko.xz', '.ko.zst'))]
def name(path):
    return path.name.split('.ko')[0].replace('-', '_')
byname = {name(p): p for p in modules}
required = ['usbhid', 'snd_usb_audio', 'usb_storage', 'uas', 'sd_mod']
keep = set()
for module in required:
    if module not in byname:
        # Built-in modules don't need on-disk .ko files.
        builtins = (directory / 'modules.builtin').read_text()
        if module not in [name(pathlib.Path(p)) for p in builtins.splitlines()]:
            raise SystemExit(f'Required module absent: {module}')
        continue
    dependencies = subprocess.check_output(['modprobe', '-d', str(root), '-S', release, '--show-depends', module], text=True)
    for line in dependencies.splitlines():
        if line.startswith('insmod '):
            keep.add(name(pathlib.Path(line.split()[1])))
if 'snd_usb_audio' not in keep:
    raise SystemExit('Expected USB audio module dependency closure')
for module in modules:
    if name(module) not in keep:
        module.unlink()
for link in ['build', 'source']:
    path = directory / link
    if path.is_symlink():
        path.unlink()
print('Retained USB module closure:', ', '.join(sorted(keep)))
