import os
from pathlib import Path


class Runner:
    """Runner fixture."""

    def run(self, value: str) -> str:
        cleaned = normalize(value)
        return cleaned.upper()


def normalize(value: str) -> str:
    text = value.strip()
    return text.replace("-", "_")


def use_path() -> str:
    return str(Path(os.getcwd()))
