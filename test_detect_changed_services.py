"""Tests for the service detector the deploy workflow builds from.

A miss here is silent: the service simply never gets rebuilt, and the deploy
still reports success.
"""
import unittest

from detect_changed_services import detect_services


class DetectServicesTest(unittest.TestCase):
    def test_nothing_changed(self):
        self.assertEqual(detect_services([]), set())

    def test_files_outside_any_service_deploy_nothing(self):
        self.assertEqual(detect_services(["README.md", "LICENSE"]), set())

    def test_engine(self):
        self.assertEqual(detect_services(["engine/runBot.go"]), {"engine"})

    def test_module_is_named_after_its_directory(self):
        self.assertEqual(detect_services(["modules/skazka/main.go"]), {"skazka"})

    def test_several_modules_at_once(self):
        self.assertEqual(
            detect_services(["modules/sber/a.go", "modules/skazka/b.go"]),
            {"sber", "skazka"},
        )

    def test_sberify_service_is_detected(self):
        """It has its own compose service but used to match no rule at all."""
        self.assertEqual(
            detect_services(["sberify-service/app.py"]), {"sberify-service"}
        )

    def test_shared_code_rebuilds_everything(self):
        for path in ("common/util.go", "botModules/httpserver.go"):
            with self.subTest(path=path):
                self.assertEqual(detect_services([path]), {"all"})

    def test_dependency_pins_rebuild_everything(self):
        """Every binary builds against them, so none can be skipped."""
        for path in ("go.mod", "go.sum"):
            with self.subTest(path=path):
                self.assertEqual(detect_services([path]), {"all"})

    def test_all_wins_over_individual_services(self):
        self.assertEqual(
            detect_services(["modules/sber/a.go", "common/util.go"]), {"all"}
        )

    def test_prefix_lookalikes_are_not_shared_code(self):
        """"commonplace.md" starts with "common" but is not common/."""
        self.assertEqual(detect_services(["commonplace.md"]), set())


if __name__ == "__main__":
    unittest.main()
