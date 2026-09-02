import os
from typing import List, Set

# Changing any of these rebuilds everything: the first two are compiled into
# every binary, and the module files pin the dependency versions they all build
# against.
REBUILD_EVERYTHING = ("common/", "botModules/", "go.mod", "go.sum")


def detect_services(files: List[str]) -> Set[str]:
    services = set()
    for file in files:
        if file.startswith(REBUILD_EVERYTHING):
            return {"all"}
        elif file.startswith("engine/"):
            services.add("engine")
        # sberify-service is a service like any other and used to be invisible
        # here, so changes to it were never deployed.
        elif file.startswith("sberify-service/"):
            services.add("sberify-service")
        # notify лежит верхним уровнем, а не в modules/, потому что это не
        # BotModule. Без этой ветки он был бы здесь невидим.
        elif file.startswith("notify/"):
            services.add("notify")
        # admin лежит верхним уровнем по той же причине, что и notify: это не
        # BotModule, движок его не опрашивает.
        elif file.startswith("admin/"):
            services.add("admin")
        # settings линкуется в движок и в панель. В REBUILD_EVERYTHING он
        # намеренно не попал: пересобирать из-за него все семь сервисов на
        # хосте с двумя гигабайтами памяти незачем.
        elif file.startswith("settings/"):
            services.update(("engine", "admin"))
        elif file.startswith("modules/"):
            parts = file.split("/")
            if len(parts) > 1:
                services.add(parts[1])
    return services


if __name__ == "__main__":
    changed = os.getenv("CHANGED_FILES", "").splitlines()
    services = detect_services(changed)
    print(" ".join(services))
