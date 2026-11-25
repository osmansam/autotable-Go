# P0 advisory verification — 2026-11-24

Baseline: commit `9d29722`, Go 1.24.10. Official Go vulnerability database queried using govulncheck v1.1.4. Reachability is conservative static analysis, not proof of exploitability; platform-specific findings may not apply to the Linux production binary. All rows below are actual findings, not every advisory ever published for a module.

126 unique baseline advisories. Target versions and Go requirements were verified with `go list -m -json module@version` before upgrades. Supported target toolchain: Go 1.26.11, required by security-patched modules. Tests passed before changes.

| Advisory | Module at baseline | Fixed version from database | Analysis |
|---|---|---|---|
| [GO-2020-0017](https://pkg.go.dev/vuln/GO-2020-0017) | `github.com/dgrijalva/jwt-go@v3.2.0+incompatible` | `migration required` | module only |
| [GO-2024-2958](https://pkg.go.dev/vuln/GO-2024-2958) | `github.com/gorilla/schema@v1.2.0` | `v1.4.1` | called |
| [GO-2025-4134](https://pkg.go.dev/vuln/GO-2025-4134) | `golang.org/x/crypto@v0.43.0` | `v0.45.0` | module only |
| [GO-2025-4135](https://pkg.go.dev/vuln/GO-2025-4135) | `golang.org/x/crypto@v0.43.0` | `v0.45.0` | module only |
| [GO-2025-4155](https://pkg.go.dev/vuln/GO-2025-4155) | `stdlib@v1.24.10` | `v1.24.11` | called |
| [GO-2025-4175](https://pkg.go.dev/vuln/GO-2025-4175) | `stdlib@v1.24.10` | `v1.24.11` | called |
| [GO-2026-4337](https://pkg.go.dev/vuln/GO-2026-4337) | `stdlib@v1.24.10` | `v1.24.13` | called |
| [GO-2026-4340](https://pkg.go.dev/vuln/GO-2026-4340) | `stdlib@v1.24.10` | `v1.24.12` | called |
| [GO-2026-4341](https://pkg.go.dev/vuln/GO-2026-4341) | `stdlib@v1.24.10` | `v1.24.12` | called |
| [GO-2026-4342](https://pkg.go.dev/vuln/GO-2026-4342) | `stdlib@v1.24.10` | `v1.24.12` | module only |
| [GO-2026-4394](https://pkg.go.dev/vuln/GO-2026-4394) | `go.opentelemetry.io/otel/sdk@v1.30.0` | `v1.40.0` | called |
| [GO-2026-4471](https://pkg.go.dev/vuln/GO-2026-4471) | `github.com/gofiber/fiber/v2@v2.52.10` | `v2.52.11` | module only |
| [GO-2026-4543](https://pkg.go.dev/vuln/GO-2026-4543) | `github.com/gofiber/fiber/v2@v2.52.10` | `v2.52.12` | called |
| [GO-2026-4601](https://pkg.go.dev/vuln/GO-2026-4601) | `stdlib@v1.24.10` | `v1.25.8` | called |
| [GO-2026-4602](https://pkg.go.dev/vuln/GO-2026-4602) | `stdlib@v1.24.10` | `v1.25.8` | module only |
| [GO-2026-4603](https://pkg.go.dev/vuln/GO-2026-4603) | `stdlib@v1.24.10` | `v1.25.8` | module only |
| [GO-2026-4762](https://pkg.go.dev/vuln/GO-2026-4762) | `google.golang.org/grpc@v1.67.0-dev` | `v1.79.3` | module only |
| [GO-2026-4815](https://pkg.go.dev/vuln/GO-2026-4815) | `golang.org/x/image@v0.25.0` | `v0.38.0` | module only |
| [GO-2026-4864](https://pkg.go.dev/vuln/GO-2026-4864) | `stdlib@v1.24.10` | `v1.25.9` | module only |
| [GO-2026-4865](https://pkg.go.dev/vuln/GO-2026-4865) | `stdlib@v1.24.10` | `v1.25.9` | called |
| [GO-2026-4869](https://pkg.go.dev/vuln/GO-2026-4869) | `stdlib@v1.24.10` | `v1.25.9` | module only |
| [GO-2026-4870](https://pkg.go.dev/vuln/GO-2026-4870) | `stdlib@v1.24.10` | `v1.25.9` | called |
| [GO-2026-4918](https://pkg.go.dev/vuln/GO-2026-4918) | `stdlib@v1.24.10` | `v1.25.10` | called |
| [GO-2026-4946](https://pkg.go.dev/vuln/GO-2026-4946) | `stdlib@v1.24.10` | `v1.25.9` | called |
| [GO-2026-4947](https://pkg.go.dev/vuln/GO-2026-4947) | `stdlib@v1.24.10` | `v1.25.9` | called |
| [GO-2026-4950](https://pkg.go.dev/vuln/GO-2026-4950) | `github.com/valyala/fasthttp@v1.51.0` | `v1.70.0` | module only |
| [GO-2026-4961](https://pkg.go.dev/vuln/GO-2026-4961) | `golang.org/x/image@v0.25.0` | `v0.42.0` | called |
| [GO-2026-4962](https://pkg.go.dev/vuln/GO-2026-4962) | `golang.org/x/image@v0.25.0` | `v0.39.0` | module only |
| [GO-2026-4970](https://pkg.go.dev/vuln/GO-2026-4970) | `stdlib@v1.24.10` | `v1.25.12` | module only |
| [GO-2026-4971](https://pkg.go.dev/vuln/GO-2026-4971) | `stdlib@v1.24.10` | `v1.25.10` | called |
| [GO-2026-4976](https://pkg.go.dev/vuln/GO-2026-4976) | `stdlib@v1.24.10` | `v1.25.10` | module only |
| [GO-2026-4977](https://pkg.go.dev/vuln/GO-2026-4977) | `stdlib@v1.24.10` | `v1.25.10` | called |
| [GO-2026-4980](https://pkg.go.dev/vuln/GO-2026-4980) | `stdlib@v1.24.10` | `v1.25.10` | module only |
| [GO-2026-4981](https://pkg.go.dev/vuln/GO-2026-4981) | `stdlib@v1.24.10` | `v1.25.10` | module only |
| [GO-2026-4982](https://pkg.go.dev/vuln/GO-2026-4982) | `stdlib@v1.24.10` | `v1.25.10` | module only |
| [GO-2026-4986](https://pkg.go.dev/vuln/GO-2026-4986) | `stdlib@v1.24.10` | `v1.25.10` | called |
| [GO-2026-5005](https://pkg.go.dev/vuln/GO-2026-5005) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5006](https://pkg.go.dev/vuln/GO-2026-5006) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5013](https://pkg.go.dev/vuln/GO-2026-5013) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5014](https://pkg.go.dev/vuln/GO-2026-5014) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5015](https://pkg.go.dev/vuln/GO-2026-5015) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5016](https://pkg.go.dev/vuln/GO-2026-5016) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5017](https://pkg.go.dev/vuln/GO-2026-5017) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5018](https://pkg.go.dev/vuln/GO-2026-5018) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5019](https://pkg.go.dev/vuln/GO-2026-5019) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5020](https://pkg.go.dev/vuln/GO-2026-5020) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5021](https://pkg.go.dev/vuln/GO-2026-5021) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5023](https://pkg.go.dev/vuln/GO-2026-5023) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) | `golang.org/x/sys@v0.37.0` | `v0.44.0` | module only |
| [GO-2026-5025](https://pkg.go.dev/vuln/GO-2026-5025) | `golang.org/x/net@v0.46.0` | `v0.55.0` | module only |
| [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) | `stdlib@v1.24.10` | `v1.25.13` | called |
| [GO-2026-5027](https://pkg.go.dev/vuln/GO-2026-5027) | `golang.org/x/net@v0.46.0` | `v0.55.0` | module only |
| [GO-2026-5028](https://pkg.go.dev/vuln/GO-2026-5028) | `golang.org/x/net@v0.46.0` | `v0.55.0` | module only |
| [GO-2026-5029](https://pkg.go.dev/vuln/GO-2026-5029) | `golang.org/x/net@v0.46.0` | `v0.55.0` | module only |
| [GO-2026-5030](https://pkg.go.dev/vuln/GO-2026-5030) | `golang.org/x/net@v0.46.0` | `v0.55.0` | module only |
| [GO-2026-5031](https://pkg.go.dev/vuln/GO-2026-5031) | `golang.org/x/image@v0.25.0` | `v0.41.0` | module only |
| [GO-2026-5032](https://pkg.go.dev/vuln/GO-2026-5032) | `golang.org/x/image@v0.25.0` | `v0.41.0` | module only |
| [GO-2026-5033](https://pkg.go.dev/vuln/GO-2026-5033) | `golang.org/x/crypto@v0.43.0` | `v0.52.0` | module only |
| [GO-2026-5037](https://pkg.go.dev/vuln/GO-2026-5037) | `stdlib@v1.24.10` | `v1.25.11` | called |
| [GO-2026-5038](https://pkg.go.dev/vuln/GO-2026-5038) | `stdlib@v1.24.10` | `v1.25.11` | module only |
| [GO-2026-5039](https://pkg.go.dev/vuln/GO-2026-5039) | `stdlib@v1.24.10` | `v1.25.11` | called |
| [GO-2026-5061](https://pkg.go.dev/vuln/GO-2026-5061) | `golang.org/x/image@v0.25.0` | `v0.43.0` | called |
| [GO-2026-5062](https://pkg.go.dev/vuln/GO-2026-5062) | `golang.org/x/image@v0.25.0` | `v0.43.0` | module only |
| [GO-2026-5066](https://pkg.go.dev/vuln/GO-2026-5066) | `golang.org/x/image@v0.25.0` | `v0.43.0` | module only |
| [GO-2026-5327](https://pkg.go.dev/vuln/GO-2026-5327) | `go.mongodb.org/mongo-driver@v1.12.1` | `v1.17.7` | called |
| [GO-2026-5426](https://pkg.go.dev/vuln/GO-2026-5426) | `go.opentelemetry.io/otel/sdk@v1.30.0` | `v1.43.0` | module only |
| [GO-2026-5585](https://pkg.go.dev/vuln/GO-2026-5585) | `github.com/gofiber/fiber/v2@v2.52.10` | `v2.52.13` | module only |
| [GO-2026-5841](https://pkg.go.dev/vuln/GO-2026-5841) | `github.com/klauspost/compress@v1.17.9` | `v1.18.7` | module only |
| [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856) | `stdlib@v1.24.10` | `v1.25.12` | called |
| [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) | `golang.org/x/crypto@v0.43.0` | `v0.58.1-0.20261009181718-2f1f834330b2` | module only |
| [GO-2026-5942](https://pkg.go.dev/vuln/GO-2026-5942) | `golang.org/x/net@v0.46.0` | `v0.56.0` | module only |
| [GO-2026-5960](https://pkg.go.dev/vuln/GO-2026-5960) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.0` | called |
| [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) | `golang.org/x/text@v0.30.0` | `v0.39.0` | called |
| [GO-2026-5972](https://pkg.go.dev/vuln/GO-2026-5972) | `stdlib@v1.24.10` | `v1.25.13` | called |
| [GO-2026-6061](https://pkg.go.dev/vuln/GO-2026-6061) | `google.golang.org/grpc@v1.67.0-dev` | `v1.82.1` | called |
| [GO-2026-6088](https://pkg.go.dev/vuln/GO-2026-6088) | `stdlib@v1.24.10` | `v1.25.13` | called |
| [GO-2026-6089](https://pkg.go.dev/vuln/GO-2026-6089) | `stdlib@v1.24.10` | `v1.25.13` | module only |
| [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) | `stdlib@v1.24.10` | `v1.25.13` | called |
| [GO-2026-6091](https://pkg.go.dev/vuln/GO-2026-6091) | `stdlib@v1.24.10` | `v1.25.13` | module only |
| [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) | `stdlib@v1.24.10` | `v1.25.13` | called |
| [GO-2026-6222](https://pkg.go.dev/vuln/GO-2026-6222) | `golang.org/x/image@v0.25.0` | `v0.45.0` | called |
| [GO-2026-6303](https://pkg.go.dev/vuln/GO-2026-6303) | `golang.org/x/crypto@v0.43.0` | `v0.55.0` | module only |
| [GO-2026-6348](https://pkg.go.dev/vuln/GO-2026-6348) | `google.golang.org/grpc@v1.67.0-dev` | `v1.83.1` | called |
| [GO-2026-6354](https://pkg.go.dev/vuln/GO-2026-6354) | `golang.org/x/crypto@v0.43.0` | `v0.56.0` | module only |
| [GO-2026-6355](https://pkg.go.dev/vuln/GO-2026-6355) | `golang.org/x/crypto@v0.43.0` | `v0.56.0` | module only |
| [GO-2026-6441](https://pkg.go.dev/vuln/GO-2026-6441) | `google.golang.org/grpc@v1.67.0-dev` | `v1.83.1` | module only |
| [GO-2026-6443](https://pkg.go.dev/vuln/GO-2026-6443) | `google.golang.org/grpc@v1.67.0-dev` | `v1.82.2` | module only |
| [GO-2026-6452](https://pkg.go.dev/vuln/GO-2026-6452) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260728235842-f98df08a8f6a` | called |
| [GO-2026-6453](https://pkg.go.dev/vuln/GO-2026-6453) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.0` | called |
| [GO-2026-6505](https://pkg.go.dev/vuln/GO-2026-6505) | `go.opentelemetry.io/otel/sdk@v1.30.0` | `v1.45.0` | called |
| [GO-2026-6599](https://pkg.go.dev/vuln/GO-2026-6599) | `stdlib@v1.24.10` | `v1.26.9` | module only |
| [GO-2026-6600](https://pkg.go.dev/vuln/GO-2026-6600) | `stdlib@v1.24.10` | `v1.26.9` | module only |
| [GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603) | `golang.org/x/net@v0.46.0` | `v0.60.0` | called |
| [GO-2026-6604](https://pkg.go.dev/vuln/GO-2026-6604) | `stdlib@v1.24.10` | `v1.26.9` | module only |
| [GO-2026-6605](https://pkg.go.dev/vuln/GO-2026-6605) | `stdlib@v1.24.10` | `v1.26.9` | called |
| [GO-2026-6607](https://pkg.go.dev/vuln/GO-2026-6607) | `stdlib@v1.24.10` | `v1.26.9` | called |
| [GO-2026-6608](https://pkg.go.dev/vuln/GO-2026-6608) | `stdlib@v1.24.10` | `v1.26.9` | called |
| [GO-2026-6609](https://pkg.go.dev/vuln/GO-2026-6609) | `stdlib@v1.24.10` | `v1.26.9` | module only |
| [GO-2026-6610](https://pkg.go.dev/vuln/GO-2026-6610) | `stdlib@v1.24.10` | `v1.26.9` | called |
| [GO-2026-6611](https://pkg.go.dev/vuln/GO-2026-6611) | `golang.org/x/net@v0.46.0` | `v0.60.0` | called |
| [GO-2026-6612](https://pkg.go.dev/vuln/GO-2026-6612) | `golang.org/x/net@v0.46.0` | `v0.60.0` | called |
| [GO-2026-6613](https://pkg.go.dev/vuln/GO-2026-6613) | `stdlib@v1.24.10` | `v1.26.9` | called |
| [GO-2026-6616](https://pkg.go.dev/vuln/GO-2026-6616) | `go.opentelemetry.io/otel/sdk@v1.30.0` | `v1.33.0` | called |
| [GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617) | `golang.org/x/net@v0.46.0` | `v0.60.0` | called |
| [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629) | `golang.org/x/text@v0.30.0` | `v0.41.0` | module only |
| [GO-2026-6674](https://pkg.go.dev/vuln/GO-2026-6674) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260731010303-ae2113b410e5` | called |
| [GO-2026-6676](https://pkg.go.dev/vuln/GO-2026-6676) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260816084418-46a5eb289448` | called |
| [GO-2026-6677](https://pkg.go.dev/vuln/GO-2026-6677) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260910071107-696050fbf14e` | called |
| [GO-2026-6678](https://pkg.go.dev/vuln/GO-2026-6678) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260807015645-a54c578af309` | module only |
| [GO-2026-6679](https://pkg.go.dev/vuln/GO-2026-6679) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260805032953-db93f8d89de7` | called |
| [GO-2026-6680](https://pkg.go.dev/vuln/GO-2026-6680) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260820023833-99903a3240e5` | called |
| [GO-2026-6681](https://pkg.go.dev/vuln/GO-2026-6681) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260906004932-2badfcd5841d` | called |
| [GO-2026-6682](https://pkg.go.dev/vuln/GO-2026-6682) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260812075026-be7a16390fa6` | module only |
| [GO-2026-6684](https://pkg.go.dev/vuln/GO-2026-6684) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260915055537-22f76f9acb94` | called |
| [GO-2026-6685](https://pkg.go.dev/vuln/GO-2026-6685) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260930021559-01a9ff32fb3c` | called |
| [GO-2026-6687](https://pkg.go.dev/vuln/GO-2026-6687) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260929015830-8ffeb07ec9a3` | module only |
| [GO-2026-6688](https://pkg.go.dev/vuln/GO-2026-6688) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260911060113-ea12859e43c6` | called |
| [GO-2026-6689](https://pkg.go.dev/vuln/GO-2026-6689) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20260912113515-5f636f9dcde5` | called |
| [GO-2026-6695](https://pkg.go.dev/vuln/GO-2026-6695) | `github.com/xuri/excelize/v2@v2.10.0` | `v2.11.1-0.20261003002531-6258dcebc4e2` | module only |
| [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) | `stdlib@v1.24.10` | `v1.26.10` | called |
| [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859) | `stdlib@v1.24.10` | `v1.26.11` | called |
| [GO-2026-6860](https://pkg.go.dev/vuln/GO-2026-6860) | `golang.org/x/net@v0.46.0` | `v0.62.0` | module only |
| [GO-2026-6865](https://pkg.go.dev/vuln/GO-2026-6865) | `github.com/klauspost/compress@v1.17.9` | `v1.20.2` | called |
| [GO-2026-6866](https://pkg.go.dev/vuln/GO-2026-6866) | `golang.org/x/net@v0.46.0` | `v0.62.0` | module only |
| [GO-2026-6888](https://pkg.go.dev/vuln/GO-2026-6888) | `github.com/andybalholm/brotli@v1.1.0` | `v1.2.7` | called |
| [GO-2026-6975](https://pkg.go.dev/vuln/GO-2026-6975) | `go.opentelemetry.io/otel@v1.30.0` | `v1.47.0` | called |

Targets: Fiber v2.52.16 (Go 1.20), fasthttp v1.80.0 (Go 1.26), x/crypto v0.59.0, x/net v0.63.0, x/image v0.48.0 (Go 1.26), gRPC v1.85.0 (Go 1.26), Mongo driver v1.17.10 (Go 1.18; retain v1 API), OTel v1.47.0 (Go 1.26), Excelize v2.11.1 (Go 1.26), brotli v1.3.0 (Go 1.22), compress v1.20.5 (Go 1.25), JWT v5.3.1 (Go 1.21). JWT maintainer migration and releases: https://github.com/golang-jwt/jwt/releases and https://github.com/golang-jwt/jwt/blob/v5.3.1/MIGRATION_GUIDE.md .

Remaining findings are rescanned after implementation; see stage report. No claim that every baseline finding is exploitable or that upgrading alone resolves application security.

## Runtime OS findings verified on 2026-11-25

CI run [45648525018](https://github.com/osmansam/autotable-Go/actions/runs/45648525018) rejected these five HIGH package findings. Fixed versions were independently checked against [Alpine's v3.24 security database](https://secdb.alpinelinux.org/v3.24/main.json).

| Advisory | Runtime packages | Installed | Alpine fixed version |
| --- | --- | --- | --- |
| [CVE-2026-14456](https://security.alpinelinux.org/vuln/CVE-2026-14456) | libcrypto3, libssl3 | 3.5.7-r0 | 3.5.8-r0 |
| [CVE-2026-107891](https://security.alpinelinux.org/vuln/CVE-2026-107891) | musl, musl-utils | 1.2.6-r2 | 1.2.6-r3 |
| [CVE-2026-85091](https://security.alpinelinux.org/vuln/CVE-2026-85091) | zlib | 1.3.2-r0 | 1.3.2-r1 |

The [official image metadata](https://github.com/docker-library/official-images/blob/master/library/alpine) identifies Alpine 3.24.4 as the current 3.24 stable image. Docker Hub's versioned manifest resolves to `sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf`; this is the new runtime pin. Its amd64 package inventory was inspected directly: OpenSSL 3.5.9-r0, musl/musl-utils 1.2.6-r3, zlib 1.3.2-r1. Alpine 3.24.2 was rejected as insufficient because its musl and zlib remain below the fixes.

The build also upgrades installed packages from the existing v3.24 stable repositories before adding certificates/timezone data. The official [amd64 package index](https://dl-cdn.alpinelinux.org/alpine/v3.24/main/x86_64/APKINDEX.tar.gz) currently supplies OpenSSL 3.5.9-r1, musl/musl-utils 1.2.6-r3 and zlib 1.4-r0, incorporating additional fixes beyond the older snapshot. Repository patches remain dynamic, as certificate/timezone installation already was; the base image remains pinned and the complete resulting OS inventory must pass the unchanged scanner gate. Rebuild the package-install layer when refreshing security updates; a cached layer alone does not fetch later fixes.

Compatibility: runtime remains on Alpine 3.24, with no Go module/toolchain changes. The application remains a CGO-disabled Linux/amd64 binary built with Go 1.26.11, with UID/GID 10001 and the existing certificates, timezone data, paths and entrypoint. Full Go race tests pass before and after this runtime dependency group. A real CI build, non-root probe and OS rescan are required before claiming the image is clear; local Docker is stopped. No deployment is performed.
