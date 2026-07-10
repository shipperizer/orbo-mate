# Changelog

## [0.2.0](https://github.com/shipperizer/orbo-mate/compare/v0.1.0...v0.2.0) (2026-07-09)


### Features

* **cli:** add list-models command with OpenRouter integration ([be84757](https://github.com/shipperizer/orbo-mate/commit/be847572af82c895d13b82759aca605e369c59b1))
* **cli:** add mock-webhook command for local testing ([4b0d18e](https://github.com/shipperizer/orbo-mate/commit/4b0d18e4942bb0681d7b4e7c492a11e9fd0e0458))
* **cli:** allow specifying organization name in mock-webhook command ([269b369](https://github.com/shipperizer/orbo-mate/commit/269b36909b6a5c43f860d0f860e5ce2b0d60f977))
* **cli:** filter list-models by programming category and drop language flag ([6f89659](https://github.com/shipperizer/orbo-mate/commit/6f89659d9458daa9c73322c13935764a78ff6820))
* **cli:** introduce cobra-cli for application entrypoint ([0506fe5](https://github.com/shipperizer/orbo-mate/commit/0506fe580c3e61c224e6aa5e9cb440e4c830f073))
* **cli:** switch list-models to use public unauthenticated endpoint ([b6d67f8](https://github.com/shipperizer/orbo-mate/commit/b6d67f83d1e02dc9dfcbc2bcf83ccce7f12534dd))
* **config:** add ALLOWED_ORGS configuration for organization restriction ([ba42968](https://github.com/shipperizer/orbo-mate/commit/ba42968f335a72b077c4fab215591c70bc19cdcf))
* **config:** integrate envconfig for environment variable management ([4e1e95e](https://github.com/shipperizer/orbo-mate/commit/4e1e95eb4ce3dd1545311bff0440456ac9dad952))
* **config:** support LOG_LEVEL environment variable and dynamic logger level loading ([da1df4d](https://github.com/shipperizer/orbo-mate/commit/da1df4d4316241e3a1e96dc9c3376b2d2186d73e))
* **config:** update default OpenRouter model to llama-3.1-70b-instruct ([422c77b](https://github.com/shipperizer/orbo-mate/commit/422c77ba6f734df6249db2e524fdcf8f72ba0ab1))
* **deploy:** add ArgoCD application manifest and Kustomize overlay ([cadbf2f](https://github.com/shipperizer/orbo-mate/commit/cadbf2f67df9e775f95ee9b3343986f821344b21))
* **deploy:** add skaffold setup with kustomize and structure-tests ([eb7797f](https://github.com/shipperizer/orbo-mate/commit/eb7797f4db53e475cf61f3dd710c53214fd22ef0))
* **deploy:** disable default traefik configuration and add contour ingress class ([248bac6](https://github.com/shipperizer/orbo-mate/commit/248bac64c5b4b067bd3d41b39548ae4a716fc496))
* **deploy:** modularize gateway resources and switch to cilium gateway class ([3fae3da](https://github.com/shipperizer/orbo-mate/commit/3fae3da41d6922110a4c8a32a7099d587026df54))
* implement observability, diagnostics, and lifecycle suite ([6e0b40b](https://github.com/shipperizer/orbo-mate/commit/6e0b40b3f3a1a342e45202ff148590829d2b8477))
* **k8s:** add gateway API resources (Gateway and HTTPRoute) for deployment ([3f708aa](https://github.com/shipperizer/orbo-mate/commit/3f708aa54a5fb986c05294404fb2ab9914880c76))
* **k8s:** add HTTPS TLS listener with dummy domain and dummy volume to base, and override in overlay ([e8732b2](https://github.com/shipperizer/orbo-mate/commit/e8732b2ce7691c9e68fd8c67ceb5f4a56aeafe30))
* **k8s:** add liveness and readiness probes to base deployment ([c53c3c8](https://github.com/shipperizer/orbo-mate/commit/c53c3c8d1c8028039afb810fa8d948b5b20b5163))
* **logger:** support dynamic log level configuration via LOG_LEVEL environment variable ([7994a9c](https://github.com/shipperizer/orbo-mate/commit/7994a9c75bf55b8cc5196485147d9bbe87834b61))
* **pool:** implement goroutine worker pool for concurrent processing ([4f6a031](https://github.com/shipperizer/orbo-mate/commit/4f6a031d35249bc62a666cd77b8f7ab029ce50f4))
* **reviewer:** add MAX_TOKENS configuration to prevent response truncation ([8c12772](https://github.com/shipperizer/orbo-mate/commit/8c127729c197e7da8a54658ba218370f593b7f68))
* **reviewer:** add OpenRouter integration and PR diff fetching ([315b26b](https://github.com/shipperizer/orbo-mate/commit/315b26bb32934755fc254161c4cbf7d84e7c5d76))
* **reviewer:** add OpenTelemetry tracing and robust logging to OpenRouter calls ([faa940a](https://github.com/shipperizer/orbo-mate/commit/faa940abbc9cc5243c2ab579f067ecde65735c46))
* **reviewer:** handle solve comment requests with rich prompt context ([b2b21df](https://github.com/shipperizer/orbo-mate/commit/b2b21df9b26401e99062894734bfede9c8e3174a))
* **reviewer:** include model description in all GitHub comment responses ([6a89e0b](https://github.com/shipperizer/orbo-mate/commit/6a89e0bc6bcfb2c6c0cbdd85291360030134ceb7))
* **reviewer:** map and report detailed OpenRouter API errors to users ([bbe0bb6](https://github.com/shipperizer/orbo-mate/commit/bbe0bb61c56c68849ea4e92ee31a8a4203e5fb22))
* **reviewer:** support flexible model parsing patterns with dots and fallback LLM ([3ebeb80](https://github.com/shipperizer/orbo-mate/commit/3ebeb80835b9b345e55dd751eb54e2c3bd1fa19b))
* **reviewer:** support Issue/PR assignment handlers and thorough OpenRouter error reports ([9ee841f](https://github.com/shipperizer/orbo-mate/commit/9ee841f2d8bb1fe299f4865e734719efc4d7cf47))
* **server:** enforce allowed organization limits and prevent cross-org requests ([6999253](https://github.com/shipperizer/orbo-mate/commit/699925380dadb3bb47aca9940a17c4e27e761d50))
* **server:** enforce JSON-only payloads and handle multi-event webhook dispatching ([fa8e5ba](https://github.com/shipperizer/orbo-mate/commit/fa8e5ba865b8b8bca63081bf26e7911a4ee8d3cb))
* **server:** prevent recursive bot loops by ignoring comments from bot accounts ([349bf9e](https://github.com/shipperizer/orbo-mate/commit/349bf9ec8ddb8a7247d7532ea71c2e5bc8bcecdd))
* **server:** setup webhook routing with go-chi ([0145ed9](https://github.com/shipperizer/orbo-mate/commit/0145ed9ddffcda974fbba34678327b3ebd4a3106))
* **server:** support created and edited actions for issue comment events ([881111c](https://github.com/shipperizer/orbo-mate/commit/881111c6502822b2b6ebda13aa3a59cbecd71779))
* **skaffold:** add local profile targeting k8s/overlays/local ([77cf0d8](https://github.com/shipperizer/orbo-mate/commit/77cf0d8a9bc96cc9fe10808df559baef278e8003))


### Bug Fixes

* **cli:** allow list-models to run without full webhook server env ([ad9ed6a](https://github.com/shipperizer/orbo-mate/commit/ad9ed6a9dad286dc29a8d3943eaea59f3d5d3a61))
* **cli:** update mock-webhook default flag model to llama-3.1-70b-instruct ([2dcf057](https://github.com/shipperizer/orbo-mate/commit/2dcf057b326d59e694c14f1b4199c6cfbf3d4b42))
