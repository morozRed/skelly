#!/usr/bin/env python3
import json
import sys

request = json.load(sys.stdin)
symbol = ((request.get("input") or {}).get("symbol") or {})
name = symbol.get("name", "symbol")
kind = symbol.get("kind", "symbol")
path = symbol.get("path", "")

response = {
    "summary": f"{kind} {name} in {path}.",
    "purpose": f"Describe responsibilities of {name}.",
    "side_effects": "Unknown from static analysis.",
    "confidence": "medium",
}
print(json.dumps(response))
