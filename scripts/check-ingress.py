#!/usr/bin/env python3
"""Read-only checks of explicitly supplied staging HTTPS/API/frontend endpoints."""
import argparse
import json
import re
import sys
import urllib.request
from urllib.parse import urlsplit

MIN_HSTS_AGE = 15552000  # 180 days; includeSubDomains/preload are not imposed.


def validate_url(value):
    parts = urlsplit(value)
    if parts.scheme != "https" or not parts.hostname or parts.username or parts.password or parts.query or parts.fragment or any(c.isspace() for c in value):
        raise ValueError("supply an HTTPS staging URL without embedded credentials, query or fragment")
    _ = parts.port
    return value.rstrip("/")


def origin(value):
    parts = urlsplit(value)
    return parts.scheme, parts.hostname, parts.port or 443


def validate_headers(headers, frontend=False):
    hsts = headers.get("Strict-Transport-Security", "")
    ages = re.findall(r'(?:^|;)\s*max-age\s*=\s*"?(\d+)"?\s*(?=;|$)', hsts, re.I)
    if len(ages) != 1 or int(ages[0]) < MIN_HSTS_AGE:
        raise ValueError("enforced HSTS max-age must be at least 15552000 seconds")
    if headers.get("X-Content-Type-Options", "").lower() != "nosniff":
        raise ValueError("missing X-Content-Type-Options: nosniff")
    if not frontend:
        return
    policy = headers.get("Content-Security-Policy", "")
    directives = {}
    for section in policy.split(";"):
        words = section.split()
        if not words:
            continue
        name = words[0].lower()
        if name in directives:
            raise ValueError("duplicate CSP directive; consolidate the effective policy before verification")
        directives[name] = words[1:]
    if not directives.get("default-src") or "*" in directives["default-src"]:
        raise ValueError("frontend requires an enforced CSP with an explicit non-wildcard default-src")
    if directives.get("object-src") != ["'none'"]:
        raise ValueError("frontend CSP requires object-src 'none'")
    if directives.get("base-uri") not in (["'self'"], ["'none'"]):
        raise ValueError("frontend CSP requires base-uri 'self' or 'none'")


class SameOriginHTTPSRedirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, request, fp, code, message, headers, newurl):
        if origin(newurl) != origin(request.full_url):
            raise ValueError("redirect leaves the supplied HTTPS origin; use its canonical staging URL")
        return super().redirect_request(request, fp, code, message, headers, newurl)


def fetch(url):
    opener = urllib.request.build_opener(SameOriginHTTPSRedirects())
    request = urllib.request.Request(url, headers={"User-Agent": "autotable-release-check"})
    with opener.open(request, timeout=10) as response:
        if response.status != 200:
            raise ValueError("staging endpoint did not return HTTP 200")
        return response.headers, response.read(1024 * 1024)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-url", required=True)
    parser.add_argument("--frontend-url", required=True)
    args = parser.parse_args()
    try:
        api, frontend = validate_url(args.api_url), validate_url(args.frontend_url)
        for path, expected in (("/health", "ok"), ("/ready", "ready")):
            headers, body = fetch(api + path)
            validate_headers(headers)
            if json.loads(body).get("status") != expected:
                raise ValueError("API health/readiness response does not match the application contract")
        headers, _ = fetch(frontend)
        validate_headers(headers, frontend=True)
        if "text/html" not in headers.get("Content-Type", "").lower():
            raise ValueError("frontend URL did not serve an HTML document")
    except (OSError, ValueError, TypeError, AttributeError) as error:
        print(f"Ingress gate failed: {error}", file=sys.stderr)
        return 1
    print("Staging HTTPS/HSTS/CSP and API health/readiness checks passed; customer acceptance remains manual.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
