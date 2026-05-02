import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from freekino_serial import FreekinoSerialParser


class FakeResponse:
    def __init__(self, text: str, status_code: int = 200, url: str = ""):
        self.text = text
        self.status_code = status_code
        self.url = url

    def raise_for_status(self):
        if self.status_code >= 400:
            raise RuntimeError(f"http {self.status_code}")


SERIAL_HTML = """
<html>
  <head>
    <meta property="og:image" content="https://freekino.net/poster.jpg">
    <meta property="og:description" content="Serial description">
  </head>
  <body>
    <h1>Test serial</h1>
    <a href="/serie/123-test-1-qism">1-qism</a>
    <div data-url="/serie/123-test-2-qism">2-qism</div>
    <script>
      var playlist = ["/serie/123-test-3-qism"];
    </script>
  </body>
</html>
"""

EPISODE_HTML = "<html><body>episode page</body></html>"


class FreekinoSerialParserTests(unittest.TestCase):
    def test_collects_visible_hidden_and_script_episode_links(self):
        parser = FreekinoSerialParser()

        def fake_get(url, timeout=30):
            if "/serie/" in url:
                return FakeResponse(EPISODE_HTML, url=url)
            return FakeResponse(SERIAL_HTML, url=url)

        parser.session.get = fake_get
        parser._movie._extract_video = lambda soup, url: (
            [{"url": f"https://cdn.freekino.net/{url.rsplit('-', 1)[-1]}.m3u8", "quality": "auto", "type": "hls"}],
            {}
        )

        result = parser.parse("https://freekino.net/serial/test.html")

        self.assertTrue(result["success"])
        self.assertEqual(len(result["episodes"]), 3)
        self.assertEqual([ep["episode"] for ep in result["episodes"]], [1, 2, 3])
        self.assertEqual(len(result["seasons"]), 1)
        self.assertEqual(result["seasons"][0]["episodes"][2]["episode_number"], 3)


if __name__ == "__main__":
    unittest.main()
