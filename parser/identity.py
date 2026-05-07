from dataclasses import dataclass
import logging

logger = logging.getLogger(__name__)

@dataclass(frozen=True)
class EpisodeIdentity:
    parent_source_id: str
    season: int
    episode: int

    @property
    def canonical_id(self) -> str:
        return f"{self.parent_source_id}:s{self.season:02d}e{self.episode:03d}"

    def __str__(self) -> str:
        return self.canonical_id

def validate_identity(expected: EpisodeIdentity, actual_str: str):
    if str(expected) != actual_str:
        error_msg = f"IDENTITY MISMATCH: expected={expected.canonical_id}, found={actual_str}"
        logger.error(f"[IDENTITY VALIDATION] {error_msg}")
        raise ValueError(error_msg)
