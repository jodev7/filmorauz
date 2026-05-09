
import json
import unittest
import sys
from pathlib import Path
from io import BytesIO
from unittest.mock import MagicMock

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

# Import the handler from server
from server import ParserHandler

class MockWfile(BytesIO):
    def write(self, data):
        super().write(data)

class TestParserContract(unittest.TestCase):
    def setUp(self):
        self.mock_request = MagicMock()
        self.mock_client_address = ('127.0.0.1', 12345)
        self.mock_server = MagicMock()
        
        # We need to mock the initialization of BaseHTTPRequestHandler
        # which normally reads from rfile.
        ParserHandler.__init__ = lambda self, *args, **kwargs: None
        self.handler = ParserHandler(self.mock_request, self.mock_client_address, self.mock_server)
        self.handler.wfile = MockWfile()
        self.handler.send_response = MagicMock()
        self.handler.send_header = MagicMock()
        self.handler.end_headers = MagicMock()

    def test_send_json_success_envelope(self):
        data = {"title": "Interstellar", "year": 2014}
        self.handler._send_json(data, status=200)
        
        self.handler.wfile.seek(0)
        response_data = json.loads(self.handler.wfile.read().decode('utf-8'))
        
        # Check envelope
        self.assertTrue(response_data["success"])
        self.assertEqual(response_data["data"]["title"], "Interstellar")
        # Check backward compatibility (flat fields)
        self.assertEqual(response_data["title"], "Interstellar")
        self.assertEqual(response_data["year"], 2014)

    def test_send_json_error_envelope(self):
        data = {"error": "video_url_not_found", "success": False}
        self.handler._send_json(data, status=422)
        
        self.handler.wfile.seek(0)
        response_data = json.loads(self.handler.wfile.read().decode('utf-8'))
        
        self.assertFalse(response_data["success"])
        self.assertEqual(response_data["error"], "video_url_not_found")

    def test_send_error_standardization(self):
        self.handler._send_error("Something went wrong", status=400)
        
        self.handler.wfile.seek(0)
        response_data = json.loads(self.handler.wfile.read().decode('utf-8'))
        
        self.assertFalse(response_data["success"])
        self.assertEqual(response_data["error"], "Something went wrong")

    def test_send_json_search_envelope(self):
        data = {
            "source": "uzmovi",
            "query": "interstellar",
            "results": [{"title": "Interstellar", "year": 2014}]
        }
        self.handler._send_json(data, status=200)
        
        self.handler.wfile.seek(0)
        response_data = json.loads(self.handler.wfile.read().decode('utf-8'))
        
        self.assertTrue(response_data["success"])
        self.assertIn("results", response_data["data"])
        self.assertEqual(len(response_data["data"]["results"]), 1)
        # Compatibility
        self.assertIn("results", response_data)
        self.assertEqual(len(response_data["results"]), 1)

    def test_unicode_handling(self):
        data = {"title": "Юлдузлар аро"} # Interstellar in Uzbek Cyrillic
        self.handler._send_json(data, status=200)
        
        self.handler.wfile.seek(0)
        raw_response = self.handler.wfile.read().decode('utf-8')
        
        # Check that it's NOT escaped as \uXXXX
        self.assertIn("Юлдузлар аро", raw_response)
        
        response_data = json.loads(raw_response)
        self.assertEqual(response_data["data"]["title"], "Юлдузлар аро")

if __name__ == "__main__":
    unittest.main()
