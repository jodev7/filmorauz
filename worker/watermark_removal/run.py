#!/usr/bin/env python3
"""
Watermark Removal Runner
Called by Go worker via exec.Command.

JSON result is printed to stdout (clean, parseable).
All logs go to stderr (captured separately by Go).

Usage: run.py <input_path> <output_path>
"""
import sys
import os

# This file lives at worker/watermark_removal/run.py
# _script_dir = worker/watermark_removal/
# _worker_dir = worker/  — must be in sys.path for `import watermark_removal` to work
_script_dir = os.path.dirname(os.path.abspath(__file__))
_worker_dir = os.path.dirname(_script_dir)

for _p in [_worker_dir, _script_dir]:
    if _p not in sys.path:
        sys.path.insert(0, _p)

# Also honor PYTHONPATH set by Go (extra safety net)
for _p in os.environ.get("PYTHONPATH", "").split(os.pathsep):
    if _p and _p not in sys.path:
        sys.path.insert(0, _p)

import json
import logging

logging.basicConfig(
    level=logging.INFO,
    format="[WATERMARK] %(levelname)s %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger(__name__)


def main():
    if len(sys.argv) < 3:
        print(json.dumps({
            "success": False,
            "error": "Usage: run.py <input_path> <output_path>",
            "fallback_used": True,
        }))
        sys.exit(1)

    input_path = sys.argv[1]
    output_path = sys.argv[2]

    logger.info(f"Input:  {input_path}")
    logger.info(f"Output: {output_path}")
    logger.info(f"sys.path: {sys.path}")

    if not os.path.exists(input_path):
        print(json.dumps({
            "success": False,
            "error": f"Input file not found: {input_path}",
            "input_path": input_path,
            "output_path": input_path,
            "fallback_used": True,
        }))
        sys.exit(1)

    try:
        from watermark_removal import WatermarkRemovalPipeline, load_config
        logger.info("watermark_removal package imported successfully")
    except ImportError as e:
        logger.error(f"Import failed: {e}")
        logger.error(f"Tried sys.path: {sys.path}")
        print(json.dumps({
            "success": False,
            "error": f"Import error: {e}",
            "input_path": input_path,
            "output_path": input_path,
            "fallback_used": True,
        }))
        sys.exit(1)

    try:
        config = load_config()
        logger.info(
            f"Config: mode={config.mode}, samples={config.sample_count}, "
            f"confidence={config.confidence_threshold}, max_regions={config.max_regions}"
        )

        pipeline = WatermarkRemovalPipeline(config=config)
        result = pipeline.process(input_path, output_path)

        logger.info(
            f"Result: detected={result.watermark_detected}, "
            f"removed={result.watermark_removed}, "
            f"fallback={result.fallback_used}, "
            f"time={result.total_time:.1f}s"
        )

        print(json.dumps(result.to_dict()))

    except Exception as e:
        logger.exception(f"Pipeline error: {e}")
        # Return fallback result so Go can use the original video
        print(json.dumps({
            "success": True,
            "input_path": input_path,
            "output_path": input_path,
            "watermark_detected": False,
            "watermark_removed": False,
            "fallback_used": True,
            "error": str(e),
            "warning": f"Pipeline failed: {e}",
        }))


if __name__ == "__main__":
    main()
