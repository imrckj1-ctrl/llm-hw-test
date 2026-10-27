[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

English | [简体中文](README_CN.md)

# Large Model Inference Hardware Automated Testing Platform: Go Backend + Vue 3 Frontend

This repository contains an automated testing platform for large-model inference hardware. It combines a Go backend, an embedded SQLite database, and a Vue 3 frontend in a single deployable service for isolated test networks.

The platform measures inference performance across input length, concurrency, prefix caching, stability, quality, and long-context retrieval, then generates reusable reports and data packages.

## Build

```bash
# Build the frontend
cd web && npm install && npm run build && cd ..

# Build the Go backend with the frontend assets embedded
CGO_ENABLED=0 go build -ldflags="-s -w" -o hwtest .
```

## Run

```bash
./hwtest --addr 127.0.0.1:8080 --data ./data
```

- The first startup prints a random 12-character initial password.
- The first login forces a password change.
- To reset a lost password, run `./hwtest --reset-password --data ./data`.
- For production, bind the server to an isolated network address.
- A systemd example is available at `deploy/hwtest.service`.
- Open `http://<server>:8080` in a browser.

## Remote Access and API

- The Web console is normally available at `http://<server-address>:8899`; the port can be changed with `--addr`.
- Generate a platform API Token under **Settings → API Token**. The token is shown only once.
- The tested model service must expose an OpenAI-compatible endpoint such as `http://<model-server>:<port>/v1`.
- The task **Data Package** button returns the absolute server path containing logs, request and response bodies, detail CSV files, corpus snapshots, and generated artifacts.
- For local hardware tests, an operator may provide SSH access so the platform can discover GPU, model-service, startup-command, and host information. SSH credentials are never stored in this repository.

## CLI Skills

The `skills/hwtest-platform` package includes `hwtest-cli.mjs`, which lets an agent perform the complete workflow through the REST API:

```bash
export HWTEST_URL=http://<platform-address>:8899
export HWTEST_TOKEN=<API Token>

node hwtest-cli.mjs check
node hwtest-cli.mjs corpus
node hwtest-cli.mjs questions
node hwtest-cli.mjs models http://<model-server>:<port>/v1
```

Create a task, follow its execution, inspect results, and export a report:

```bash
node hwtest-cli.mjs create-task '{ "name": "...", "task": { ... } }'
node hwtest-cli.mjs status 9
node hwtest-cli.mjs wait 9
node hwtest-cli.mjs digest 9
node hwtest-cli.mjs quality 9
node hwtest-cli.mjs package 9
node hwtest-cli.mjs export 9 task-9-sample.docx
node hwtest-cli.mjs compare 9,12
```

The full operating manual is available at [`skills/hwtest-platform/SKILL.md`](skills/hwtest-platform/SKILL.md).

## Architecture

```text
main
 ├─ httpd        chi routes, session authentication, embedded static assets
 ├─ auth         bcrypt passwords, 7-day sessions, account lockout
 ├─ scheduler    device-level FIFO, global concurrency limit, cancellation and recovery
 ├─ runner       warmup, ladder, concurrency, cache, stability, quality, and NIAH execution
 │   ├─ llmclient  OpenAI-compatible SSE client
 │   └─ calibrator input-length calibration
 ├─ stats        percentiles, window aggregation, throughput, and degradation metrics
 ├─ exporter     Word reports, OLE artifacts, interactive result pages, Excel comparison
 └─ store        SQLite persistence and versioned corpus snapshots
```

## Core Features

- **Projects and tasks**: create model-only or local-hardware tests with API endpoint, model, quantization, context length, corpus version, and tester metadata.
- **Six test categories**: ladder, concurrency, prefix-cache cold/hot comparison, long-running stability, quality sampling, and NIAH long-context retrieval.
- **Automation and recovery**: scheduled execution, global concurrency limits, pause/resume, skip, single-item rerun, and full-task retry after interruption.
- **Performance metrics**: TTFT, generation speed, ITL, throughput, token usage, P50/P95/P99 latency, degradation, and capability score.
- **Quality validation**: the pi agent can call file and command tools to generate Excel, PPT, Word, PDF, and HTML artifacts, with optional judge-model and manual review.
- **Reports and exports**: Word reports with embedded OLE artifacts and interactive result pages, plus comparison views, Excel exports, and materialized task packages.
- **Corpus and question bank**: versioned corpora and quality questions, with task-level snapshots so historical tasks retain their original test basis.
- **Security and integration**: bcrypt login, session authentication, API Tokens, encrypted model credentials, REST API access, and CLI Skills integration.

## Acceptance Criteria

| # | Criterion | Verification |
|---|---|---|
| 1 | Complete all selected test items without manual intervention | End-to-end task 1 completes all five selected items |
| 2 | 30K prompt token deviation ≤1% | End-to-end measurement reports pt=30000 (0.00%) |
| 3 | Hot cache TTFT ≤ 50% of cold cache TTFT | End-to-end measurement reports 40ms vs 140ms (3.5x) |
| 4 | Versioned corpus switching | Version snapshots are recorded per task |
| 5 | Complete comparison exports | Eight report sheets and four comparison sheets validated |
| 6 | Recover interrupted tasks | Full-task rerun and historical run lookup verified |

## Real Report Example

A real Word report generated from task 9 is included in the repository:

[Download the task 9 test report](docs/reports/task-9-sample.docx)

The report contains:

- Test project, device, and inference software-stack information;
- Ladder, concurrency, stability, cache, and NIAH results;
- Metric distributions, P50/P95/P99 values, and degradation data;
- pi-agent quality-sampling records;
- OLE-embedded Excel, PPT, Word, PDF, and HTML artifacts;
- An interactive `results.html` result page.

The test-service API key in the sample report is redacted for public release.

## Public Repository Scope

This repository publishes the core product source code, default question banks, a 52-level public synthetic corpus, and corpus-generation tools.

The following material is intentionally excluded:

- `scripts/`: internal verification and debugging scripts containing environment credentials;
- `语料库/*.txt` and `语料库/context_bench/*.txt`: copyrighted Three-Body Problem text corpora;
- `release-*/`, `render_out/`, root-level report and preview files, `*.tar.gz`, `sandbox/`, and `mockups/`.

The default public corpus is stored at:

- `internal/store/seed/corpus.json.gz` as `corpus-public-v1`;
- `语料库/build_public_corpus.py` for reproducible generation;
- `语料库/gen_corpus.py` for synthetic business-text generation.

Default question banks are available at:

- `internal/store/seed/questions.json`;
- `语料库/灵思微数测试平台-实战题库.md`;
- `语料库/灵思微数测试平台-文档生成题库20题.md`.

## License

This project is released under the [MIT License](LICENSE). Bundled third-party components remain subject to their original licenses.
