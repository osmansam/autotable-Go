"""Validate release ingress policy checks without contacting any deployment."""
from email.message import Message
import importlib.util
from pathlib import Path
import unittest

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("ingress_gate",ROOT/"scripts/check-ingress.py")
gate=importlib.util.module_from_spec(spec);spec.loader.exec_module(gate)


def headers(**changes):
    values={"Strict-Transport-Security":"max-age=31536000", "X-Content-Type-Options":"nosniff", "Content-Security-Policy":"default-src 'self'; object-src 'none'; base-uri 'self'"}
    values.update(changes)
    result=Message()
    for name,value in values.items():
        if value is not None:result[name]=value
    return result


class IngressPolicyGateTests(unittest.TestCase):
    def test_api_needs_hsts_without_requiring_frontend_csp(self):
        gate.validate_headers(headers(**{"Content-Security-Policy":None}),frontend=False)
        for value in (None,"max-age=0","max-age=300","max-age=31536000invalid"):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):gate.validate_headers(headers(**{"Strict-Transport-Security":value}),frontend=False)

    def test_malformed_and_duplicate_hsts_directives_are_not_browser_policies(self):
        invalid=[
            'max-age="31536000', 'max-age=31536000"',
            'max-age=31536000; max-age=invalid', 'max-age=31536000; MAX-AGE=0',
            'max-age=31536000; includeSubDomains=true',
            'max-age=31536000; bad extension',
        ]
        for value in invalid:
            with self.subTest(value=value):
                with self.assertRaises(ValueError):gate.validate_headers(headers(**{"Strict-Transport-Security":value}))
        for value in ('max-age="31536000"; includeSubDomains; preload', 'MAX-AGE=31536000; extension="a;b"'):
            gate.validate_headers(headers(**{"Strict-Transport-Security":value}))
        response=headers(**{"Strict-Transport-Security":"max-age=0"})
        response["Strict-Transport-Security"]="max-age=31536000"
        with self.assertRaises(ValueError):gate.validate_headers(response)

    def test_host_wildcards_and_unrestricted_schemes_do_not_pass(self):
        for source in ('https://*','https://*.example.test','https:','data:'):
            policy="default-src "+source+"; object-src 'none'; base-uri 'self'"
            with self.subTest(source=source):
                with self.assertRaises(ValueError):gate.validate_headers(headers(**{"Content-Security-Policy":policy}),frontend=True)
        gate.validate_headers(headers(**{"Content-Security-Policy":"default-src 'self' https://cdn.example.test; object-src 'none'; base-uri 'self'"}),frontend=True)

    def test_frontend_requires_enforced_csp_and_minimum_directives(self):
        gate.validate_headers(headers(),frontend=True)
        for value in (None,"default-src *; object-src 'none'; base-uri 'self'", "default-src 'self'", "default-src 'self'; object-src *; base-uri *", "default-src *; default-src 'self'; object-src 'none'; base-uri 'self'"):
            with self.subTest(value=value):
                with self.assertRaises(ValueError):gate.validate_headers(headers(**{"Content-Security-Policy":value}),frontend=True)

    def test_report_only_policy_does_not_pass(self):
        response=headers(**{"Content-Security-Policy":None})
        response["Content-Security-Policy-Report-Only"]="default-src 'self'; object-src 'none'; base-uri 'self'"
        with self.assertRaises(ValueError):gate.validate_headers(response,frontend=True)

    def test_redirects_cannot_downgrade_tls_or_leave_staging_origin(self):
        from urllib.request import Request
        handler=gate.SameOriginHTTPSRedirects()
        request=Request("https://staging.example.test/")
        redirected=handler.redirect_request(request,None,302,"redirect",headers(),"https://staging.example.test/login")
        self.assertEqual(redirected.full_url,"https://staging.example.test/login")
        for target in ("http://staging.example.test/", "https://other.example.test/"):
            with self.assertRaises(ValueError):handler.redirect_request(request,None,302,"redirect",headers(),target)

    def test_urls_require_https_and_no_embedded_secrets(self):
        self.assertEqual(gate.validate_url("https://staging.example.test/"),"https://staging.example.test")
        for url in ("http://staging.example.test","https://user:password@example.test","https://example.test/?token=private","https://example.test/#private"):
            with self.subTest(url=url):
                with self.assertRaises(ValueError):gate.validate_url(url)


if __name__=="__main__":unittest.main()
