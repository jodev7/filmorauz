import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from asilmedia_serial import AsilmediaSerialParser


class FakeResponse:
    def __init__(self, text: str, status_code: int = 200):
        self.text = text
        self.status_code = status_code

    def raise_for_status(self):
        if self.status_code >= 400:
            raise RuntimeError(f"http {self.status_code}")


ASILMEDIA_SERIAL_HTML = """
<html>
  <head>
    <meta property="og:title" content="Chinakam izquvar Barcha qismlar O'zbek tilida 2014 Uzbekcha tarjima &raquo; AsilMedia.NET">
    <meta property="og:description" content="Birinchi mavsum. Ikkinchi fasl. Uchinchi fasl.">
    <meta property="og:image" content="https://asilmedia.org/poster.webp">
    <meta name="twitter:image" content="https://asilmedia.org/backdrop.webp">
  </head>
  <body>
    <article class="fullstory" itemscope itemtype="https://schema.org/TVSeries">
      <h1 class="fs-title">Chinakam izquvar Barcha qismlar O'zbek tilida 2014 Uzbekcha tarjima</h1>
      <div id="episodes-raw-data">
        <a href="https://fayllar1.ru/34/Seriallar/Chinakam%20izquvar/Chinakam%20izquvar%201-1%20480p%20O%27zbek%20tilida%20(asilmedia.net).mp4" data-label="1-qism 480p">1</a>
        <a href="https://fayllar1.ru/35/Seriallar/Chinakam%20izquvar/Chinakam%20izquvar%201-1%201080p%20O%27zbek%20tilida%20(asilmedia.net).mp4" data-label="1-qism 1080p">2</a>
        <a href="https://fayllar1.ru/34/Seriallar/Chinakam%20izquvar/Chinakam%20izquvar%202-1%20480p%20O%27zbek%20tilida%20(asilmedia.net).mp4" data-label="2-fasl 1-qism 480p">3</a>
        <a href="https://fayllar1.ru/35/Seriallar/Chinakam%20izquvar/Chinakam%20izquvar%202-1%201080p%20O%27zbek%20tilida%20(asilmedia.net).mp4" data-label="2-fasl 1-qism 1080p">4</a>
      </div>
    </article>
  </body>
</html>
"""


class AsilmediaSerialParserTests(unittest.TestCase):
    def test_extracts_seasons_and_best_quality_from_hidden_episode_data(self):
        parser = AsilmediaSerialParser()
        parser.session.get = lambda url, timeout=30: FakeResponse(ASILMEDIA_SERIAL_HTML)

        result = parser.parse(
            "https://asilmedia.org/16564-chinakam-izquvar-barcha-qismlar-ozbek-tilida-2014-uzbekcha-tarjima.html"
        )

        self.assertTrue(result["success"])
        self.assertEqual(result["type"], "serial")
        self.assertEqual(result["provider"], "asilmedia")
        self.assertEqual(result["title"], "Chinakam izquvar Barcha qismlar O'zbek tilida 2014 Uzbekcha tarjima")
        self.assertEqual(len(result["episodes"]), 2)
        self.assertEqual(len(result["seasons"]), 2)

        first = result["episodes"][0]
        self.assertEqual(first["season"], 1)
        self.assertEqual(first["episode"], 1)
        self.assertTrue(first["video_url"].endswith("1080p%20O%27zbek%20tilida%20(asilmedia.net).mp4"))
        self.assertIn("480p", first["quality_urls"])
        self.assertIn("1080p", first["quality_urls"])

        second_season = result["seasons"][1]
        self.assertEqual(second_season["season_number"], 2)
        self.assertEqual(second_season["episodes"][0]["episode_number"], 1)
        self.assertIn("1080p", second_season["episodes"][0]["quality_urls"])


if __name__ == "__main__":
    unittest.main()
