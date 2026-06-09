Swagger UI assets are **not** committed. They are downloaded at build time by
`scripts/fetch-swagger-ui.sh` (via `make swagger-ui` or the Docker build).

Pin the version with `SWAGGER_UI_VERSION` (default `5.18.3`).
