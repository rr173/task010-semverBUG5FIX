# task010-semver

轻量语义化版本（SemVer 2.0.0）解析、比较与范围匹配服务，使用纯计算无状态存储，不依赖数据库、第三方包和外部服务。

## 本地运行

```bash
go run . server --addr :8080
go run . --smoke-test
```

主要接口：`POST /api/parse`、`POST /api/compare`、`POST /api/satisfies`、`GET /healthz`。

## Docker

镜像使用国内 DaoCloud Go 1.26.3 Bookworm builder 和 Alpine 3.20 runtime；支持 `linux/amd64` 与 `linux/arm64`。
