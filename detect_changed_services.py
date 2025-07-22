import os
from typing import List, Set

def detect_services(files: List[str]) -> Set[str]:
    services = set()
    for file in files:
        if file.startswith("common/") or file.startswith("botModules/"):
            return {"all"}
        elif file.startswith("engine/"):
            services.add("engine")
        elif file.startswith("Modules/"):
            parts = file.split("/")
            if len(parts) > 1:
                services.add(parts[1])
    return services

if __name__ == "__main__":
    changed = os.getenv("CHANGED_FILES", "").splitlines()
    services = detect_services(changed)
    print(" ".join(services))