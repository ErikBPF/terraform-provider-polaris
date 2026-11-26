import importlib.util
import unittest
from pathlib import Path


class MinikubeTests(unittest.TestCase):
    def test_fixture_requires_minikube_nodes(self):
        path = Path(__file__).with_name("minikube.py")
        self.assertTrue(path.exists(), "minikube fixture is not implemented")
        spec = importlib.util.spec_from_file_location("minikube_fixture", path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with self.assertRaisesRegex(RuntimeError, "minikube"):
            module.verify_nodes({"items": [{"metadata": {"labels": {"kubernetes.azure.com/cluster": "dev"}}}]})
        with self.assertRaisesRegex(RuntimeError, "minikube"):
            module.verify_nodes({"items": []})
        module.verify_nodes({"items": [{"metadata": {"labels": {"minikube.k8s.io/name": "test"}}}]})


if __name__ == "__main__":
    unittest.main()


class MinikubeCleanupTests(unittest.TestCase):
    def fixture_module(self):
        spec = importlib.util.spec_from_file_location("minikube_fixture", Path(__file__).with_name("minikube.py"))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def test_namespace_collision_never_deletes_existing_namespace(self):
        from unittest.mock import Mock, patch
        m = self.fixture_module()
        nodes = '{"items":[{"metadata":{"labels":{"minikube.k8s.io/name":"test"}}}]}'
        with patch.object(m.sys, "argv", ["minikube.py", "--context=test", "--", "true"]), patch.object(m.subprocess, "run", side_effect=[Mock(returncode=0, stdout=nodes), Mock(returncode=1)]) as run:
            with self.assertRaisesRegex(RuntimeError, "kubectl create"):
                m.main()
            self.assertEqual(run.call_count, 2)

    def test_cleanup_failure_preserves_original_create_error(self):
        import io
        from unittest.mock import Mock, patch
        m = self.fixture_module()
        nodes = '{"items":[{"metadata":{"labels":{"minikube.k8s.io/name":"test"}}}]}'
        with patch.object(m.sys, "argv", ["minikube.py", "--context=test", "--", "true"]), patch.object(m.sys, "stderr", new_callable=io.StringIO), patch.object(m.subprocess, "run", side_effect=[Mock(returncode=0, stdout=nodes), Mock(returncode=0, stdout=""), Mock(returncode=1), Mock(returncode=1)]) as run:
            with self.assertRaisesRegex(RuntimeError, "kubectl create"):
                m.main()
            self.assertEqual(run.call_args.args[0][3:5], ["delete", "namespace"])
