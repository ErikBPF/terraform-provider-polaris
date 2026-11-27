import importlib.util
import io
import os
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location(
    "acceptance", Path(__file__).with_name("acceptance.py")
)
acceptance = importlib.util.module_from_spec(spec)
spec.loader.exec_module(acceptance)


class FixtureCleanupTests(unittest.TestCase):
    def test_failed_startup_preserves_original_error_if_cleanup_also_fails(self):
        with (
            patch.object(acceptance.sys, "stderr", new_callable=io.StringIO),
            patch.dict(os.environ, {}, clear=True),
            patch.object(acceptance.shutil, "which", return_value="fixture-engine"),
            patch.object(
                acceptance,
                "run",
                side_effect=[
                    RuntimeError("startup failed"),
                    RuntimeError("cleanup probe failed"),
                ],
            ),
            patch.object(
                acceptance.subprocess, "run", return_value=Mock(returncode=0)
            ) as remove,
        ):
            with self.assertRaisesRegex(RuntimeError, "startup failed"):
                acceptance.main()
            self.assertEqual(
                remove.call_args.args[0][:3], ["fixture-engine", "rm", "--force"]
            )


if __name__ == "__main__":
    unittest.main()
