#!/usr/bin/env python3
"""Render real Settings/onboarding QML with inert provider/host fixtures."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

parser = argparse.ArgumentParser()
parser.add_argument('--source', type=Path, default=Path(__file__).resolve().parents[2])
parser.add_argument('--output', type=Path, default=Path('/tmp/dayflow-ui-fit'))
parser.add_argument('--native', action='store_true', help='Use the current graphical session instead of offscreen Qt')
args = parser.parse_args()
args.output.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory(prefix='dayflow-ui-') as directory:
    root = Path(directory)
    for source in args.source.glob('*.qml'):
        (root / source.name).symlink_to(source.resolve())
    (root / 'Commons').mkdir()
    (root / 'Ui').mkdir()
    (root / 'Ui/qmldir').write_text('module qs.Ui\n')
    (root / 'Commons/qmldir').write_text('module qs.Commons\nsingleton Style 1.0 Style.qml\nsingleton Color 1.0 Color.qml\n')
    (root / 'Commons/Style.qml').write_text('''pragma Singleton
import QtQuick
QtObject {
 property int cornerRadius: 6
 property QtObject font: QtObject { property string family: "Sans Serif"; property int body: 12; property int caption: 12; property int subtitle: 18; property int title: 24 }
 function space(value) { return value }
}
''')
    (root / 'Commons/Color.qml').write_text('''pragma Singleton
import QtQuick
QtObject {
 property color foreground: "#e4e4e4"
 property color muted: "#a5a5a5"
 property color dim: muted
 property color accent: "#7aa2f7"
 property color urgent: "#f7768e"
}
''')
    bin_dir = root / 'bin'
    bin_dir.mkdir()
    # No real configuration, capture, provider or systemd command can run.
    (bin_dir / 'dayflow').write_text('#!/bin/sh\nprintf \'%s\\n\' \'{"providers":[],"presets":[],"agents":{}}\'\n')
    (bin_dir / 'dayflow').chmod(0o700)
    harness = Path(__file__).with_name('fit.qml').read_text()
    (root / 'shell.qml').write_text(harness.replace('OUTPUT_PATH', json.dumps(str(args.output.resolve()))))
    env = os.environ.copy()
    env['PATH'] = str(bin_dir) + ':' + env['PATH']
    if not args.native:
        env['QT_QPA_PLATFORM'] = 'offscreen'
        env['QT_QUICK_BACKEND'] = 'software'
        env['QT_QPA_PLATFORMTHEME'] = 'generic'
        runtime = root / 'runtime'
        runtime.mkdir(mode=0o700)
        env['XDG_RUNTIME_DIR'] = str(runtime)
    result = subprocess.run(['quickshell', '--no-color', '--path', str(root)], env=env, text=True, capture_output=True, timeout=90)
    log = result.stdout + result.stderr
    (args.output / 'runtime.log').write_text(log)
    failures = [line for line in log.splitlines() if 'FIT_FAIL' in line or 'Binding loop' in line or 'TypeError:' in line or 'ReferenceError:' in line or 'Unable to assign' in line or 'Error loading configuration' in line]
    if result.returncode or failures or 'FIT_COMPLETE' not in log:
        raise SystemExit('\n'.join(failures) or log[-5000:])
    summaries = [line[line.index('FIT_RESULT'): ] for line in log.splitlines() if 'FIT_RESULT' in line]
    print('\n'.join(summaries))
    print('All runtime fit checks passed; captures:', args.output)
