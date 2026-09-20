"""Exercise the installer with isolated HOME and mocked macOS service tools.

Run: python3 deploy/macos/test-installer.py
No real launchd jobs, browser windows, or user files are changed.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

PACKAGE = Path(__file__).resolve().parent / 'package'


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='beszel-installer-')
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.home = self.root / 'home with spaces'
        self.source = self.home / 'Downloads' / 'release'
        shutil.copytree(PACKAGE, self.source)
        (self.source / 'agents').mkdir()
        for name in ['beszel', 'beszel-agent', 'agents/beszel-agent_linux_amd64',
                     'agents/beszel-agent_linux_arm64', 'build-info.txt']:
            (self.source / name).write_text('fixture\n')
        sums = subprocess.check_output(['shasum', '-a', '256', 'beszel'], cwd=self.source)
        (self.source / 'sha256sums.txt').write_bytes(sums)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.env = dict(os.environ, HOME=str(self.home),
                        PATH=str(self.bin) + ':' + os.environ['PATH'],
                        CALLS=str(self.root / 'calls'))
        self.dest = self.home / 'Applications' / 'oh-my-beszel-darwin-arm64'
        self.stub('uname', 'echo arm64')
        self.stub('file', 'echo "Mach-O 64-bit executable arm64"')
        self.stub('launchctl', 'echo "launchctl $*" >> "$CALLS"\n'
                  'if [ "$1" = print ]; then echo " pid = 12345"; fi')
        self.stub('lsof', 'exit 0')
        self.stub('curl', 'echo "curl $*" >> "$CALLS"')
        self.stub('sleep', 'exit 0')
        self.stub('open', 'echo "open $*" >> "$CALLS"')

    def stub(self, name, body):
        p = self.bin / name
        p.write_text('#!/bin/sh\n' + body + '\n')
        p.chmod(0o755)

    def run_installer(self, directory=None):
        return subprocess.run(['/bin/sh', str((directory or self.source) / 'install-service.sh')],
                              env=self.env, text=True, capture_output=True)

    def test_install_from_downloads_then_upgrade_preserves_config(self):
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Hub is ready', result.stdout)
        plist = self.home / 'Library/LaunchAgents/com.oh-my-beszel.hub.plist'
        program = subprocess.check_output(['plutil', '-extract', 'Program', 'raw', '-o', '-', str(plist)], text=True).strip()
        self.assertEqual(program, str(self.dest / 'start.sh'))
        config = 'BESZEL_HTTP="127.0.0.1:18090"\nAPP_URL="http://127.0.0.1:18090"\n'
        (self.dest / 'config.env').write_text(config)
        result = self.run_installer()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.dest / 'config.env').read_text(), config)
        self.assertTrue(list((self.home / 'Applications').glob('oh-my-beszel-backup.*/package/config.env')))
        self.assertIn('http://127.0.0.1:18090/api/health', (self.root / 'calls').read_text())
        self.assertEqual(self.run_installer(self.dest).returncode, 0)
        self.assertFalse((self.source / 'config.env').exists())

    def test_preserves_config_from_previous_registered_location(self):
        old = self.home / 'Applications/old version'
        old.mkdir(parents=True)
        config = 'BESZEL_HTTP="127.0.0.1:19090"\n'
        (old / 'config.env').write_text(config)
        plist = self.home / 'Library/LaunchAgents/com.oh-my-beszel.hub.plist'
        plist.parent.mkdir(parents=True)
        shutil.copyfile(PACKAGE / 'com.oh-my-beszel.hub.plist', plist)
        subprocess.check_call(['plutil', '-replace', 'Program', '-string', str(old / 'start.sh'), str(plist)])
        self.assertEqual(self.run_installer().returncode, 0)
        self.assertEqual((self.dest / 'config.env').read_text(), config)

    def test_wrong_architecture_and_corrupt_binary_leave_service_untouched(self):
        self.stub('file', 'echo "Mach-O 64-bit executable x86_64"')
        self.assertNotEqual(self.run_installer().returncode, 0)
        self.assertFalse((self.root / 'calls').exists())
        self.stub('file', 'echo "Mach-O 64-bit executable arm64"')
        (self.source / 'beszel').write_text('corrupt\n')
        self.assertNotEqual(self.run_installer().returncode, 0)
        self.assertFalse((self.root / 'calls').exists())

    def test_foreign_listener_does_not_count_as_ready(self):
        self.stub('lsof', 'exit 1')
        result = self.run_installer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Hub did not become ready', result.stderr)
        self.assertNotIn('Hub is ready', result.stdout)

    def test_launcher_opens_browser_only_after_success(self):
        result = subprocess.run(['/bin/sh', str(self.source / 'Open.command')], env=self.env,
                                input='', text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('open http://127.0.0.1:8090', (self.root / 'calls').read_text())
        (self.root / 'calls').write_text('')
        self.stub('lsof', 'exit 1')
        result = subprocess.run(['/bin/sh', str(self.source / 'Open.command')], env=self.env,
                                input='\n', text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('open http', (self.root / 'calls').read_text())


if __name__ == '__main__':
    unittest.main()
