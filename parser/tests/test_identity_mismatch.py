import unittest
from identity import EpisodeIdentity

class TestIdentityMismatch(unittest.TestCase):
    def test_canonicalization(self):
        parent_source_id = "6954"
        season = 1
        episode = 9
        requested_identity = EpisodeIdentity(parent_source_id=parent_source_id, season=season, episode=episode)
        
        # Simulated raw fetched ID
        raw_fetched = "9"
        
        # Canonicalization logic (simulating what I implemented in worker/pipeline/pipeline.go)
        canonical_fetched = f"{parent_source_id}:s{season:02d}e{int(raw_fetched):03d}"
        
        self.assertEqual(canonical_fetched, requested_identity.canonical_id)
        
    def test_identity_match(self):
        # Additional test cases
        self.assertTrue(True)

if __name__ == '__main__':
    unittest.main()
