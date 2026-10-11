"""Exercise cleanup after an injected assertion, including a missing PID file."""
from contextlib import nullcontext
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from run_agent_identity_ci import cleanup_fixture, main


class CleanupTests(unittest.TestCase):
    def test_failure_stops_only_fixture_process_and_removes_files(self):
        fixture = Path(tempfile.mkdtemp(prefix="beszel-cleanup-test-"))
        binary = fixture / "sleep"
        shutil.copyfile("/bin/sleep", binary)
        binary.chmod(0o755)
        owned = subprocess.Popen([str(binary), "60"])
        unrelated = subprocess.Popen(["/bin/sleep", "60"])
        # Invalid/stale PID metadata must neither abort cleanup nor kill another process.
        (fixture / "agent.pid").write_text(str(unrelated.pid))
        try:
            # The local sandbox may hide /proc; model its executable ownership
            # while using real subprocesses to verify signal delivery.
            proc = fixture / "proc"
            proc.mkdir()
            for process, executable in ((owned, binary), (unrelated, Path("/bin/sleep"))):
                entry = proc / str(process.pid)
                entry.mkdir()
                (entry / "exe").symlink_to(executable)
                (entry / "stat").write_text(f"{process.pid} (sleep) Z")
            def mapped_path(value):
                return proc if value == "/proc" else Path(value)
            context = nullcontext() if Path("/proc").is_dir() else patch("run_agent_identity_ci.Path", side_effect=mapped_path)
            with context:
                with self.assertRaisesRegex(AssertionError, "injected"):
                    try:
                        raise AssertionError("injected test failure")
                    finally:
                        cleanup_fixture(fixture)
            owned.wait(timeout=10)
            self.assertIsNone(unrelated.poll())
            self.assertFalse(fixture.exists())
        finally:
            owned.kill() if owned.poll() is None else None
            unrelated.terminate()
            unrelated.wait(timeout=10)
            shutil.rmtree(fixture, ignore_errors=True)

    def test_linux_arguments_are_required(self):
        with patch("sys.argv", ["e2e", "--linux"]):
            with self.assertRaises(SystemExit) as error:
                main()
        self.assertEqual(error.exception.code, 2)


if __name__ == "__main__":
    unittest.main()
