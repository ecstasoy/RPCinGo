# docs/

Project documentation lives in two places:

- **[`deepwiki/`](../deepwiki/INDEX.md)** — authoritative, auto-maintained. Start at `deepwiki/INDEX.md`.
  - `overview.md`, `architecture.md`, `data-flow.md`, `dependencies.md`, `glossary.md`
  - `modules/` — one reference per package (circuitbreaker, client, codec, config, interceptor, loadbalancer, logger, pool, protocol, ratelimiter, registry, server, tracing, transport, mini-rpc)
  - `guides/` — configuration, telemetry, testing
  - `diagrams/` — HTML sequence/swimlane diagrams
- **`archive/`** — historical material kept for reference only; not maintained.
  - `design/` — planning docs, protobuf migration notes, acceptance reports
  - `guide/` — older per-layer tutorials superseded by `deepwiki/modules/`
  - `wiki/` — previous wiki tree superseded by `deepwiki/`
  - `interview/` — interview prep / followup / intro
  - `architecture.md`, `ARCHITECTURE_EXPLAINED.md`, `developer-guide.md`, `bugfixes.md` — superseded by `deepwiki/architecture.md` and `deepwiki/guides/`

If you're adding new documentation, put it in `deepwiki/`.
