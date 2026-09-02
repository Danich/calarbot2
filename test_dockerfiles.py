"""Каждый Dockerfile должен копировать все локальные пакеты, которые нужны
его бинарю.

Ловит ровно ту поломку, что уронила выкат 2026-09-02: движок начал
импортировать calarbot2/settings, а COPY в engine/Dockerfile никто не
добавил. Ни один тест этого не видел — образы собираются только на деплое,
и узнали мы об этом из упавшего продакшена.

Проверка транзитивная: движку нужен не только settings, но и то, что
импортирует сам settings.
"""
import re
from pathlib import Path

ROOT = Path(__file__).parent
IMPORT = re.compile(r'"(calarbot2/[\w/]+)"')
COPY = re.compile(r"^COPY\s+([\w./]+)/\s", re.MULTILINE)


def local_imports(pkg_dir: Path) -> set[str]:
    """Пакеты calarbot2/*, на которые ссылается код в каталоге (без тестов)."""
    found = set()
    for go in pkg_dir.glob("*.go"):
        if go.name.endswith("_test.go"):
            continue
        found |= set(IMPORT.findall(go.read_text()))
    return found


def needed_packages(entrypoint: str) -> set[str]:
    """Транзитивное замыкание локальных импортов от точки входа."""
    seen, queue = set(), [entrypoint]
    while queue:
        pkg = queue.pop()
        for dep in local_imports(ROOT / pkg):
            name = dep[len("calarbot2/"):]
            if name not in seen:
                seen.add(name)
                queue.append(name)
    return seen


def dockerfiles() -> list[tuple[Path, str]]:
    """Пары (Dockerfile, каталог его точки входа)."""
    out = []
    for df in ROOT.rglob("Dockerfile"):
        pkg = df.parent.relative_to(ROOT).as_posix()
        if (df.parent / "main.go").exists():
            out.append((df, pkg))
    return out


def test_dockerfiles_copy_every_package_they_need():
    found = dockerfiles()
    assert found, "Не найдено ни одного Dockerfile с main.go рядом"

    for df, pkg in found:
        copied = set(COPY.findall(df.read_text()))
        for need in needed_packages(pkg) | {pkg}:
            # Родительский COPY тащит и подпакеты: COPY modules/aiAnswer/
            # переносит и modules/aiAnswer/handlers.
            parts = need.split("/")
            covered = any(
                "/".join(parts[:i]) in copied for i in range(1, len(parts) + 1)
            )
            assert covered, (
                f"{df.relative_to(ROOT)}: бинарю нужен пакет {need}, "
                f"но COPY его не переносит. Копируется: {sorted(copied)}"
            )
