# Confluence Server LaTeX Remote Service

Remote renderer for the Confluence Server/DC LaTeX plugin. Accepts raw `.tex` content and returns a PNG image.

## What this service does

- Receives a full LaTeX document as the request body.
- Runs `pdflatex` → `pdfcrop` → `pdftoppm`.
- Returns the rendered PNG as the response body.

This repo is core service logic. Deployment is customer-owned.

## API

- `POST /render-latex`
  - Body: raw LaTeX content (full `.tex` document)
  - Content-Type: `text/plain`
  - Response: `image/png`
- `GET /healthz`
  - Response: `200 ok`

Example:

```bash
curl -X POST --data-binary @samples/plot.tex http://localhost:8080/render-latex --output samples/output.png
```

## Auth (optional)

Set `LATEX_SERVICE_API_KEY` to require a key. Requests must include either:

- `X-API-Key: <key>`
- `Authorization: Bearer <key>`

## Configuration

Environment variables (defaults shown):

- `PORT=8080`
- `LATEX_SERVICE_API_KEY=`
- `MAX_BODY_BYTES=1048576`
- `COMMAND_TIMEOUT=30s`
- `WORKER_LIMIT=numCPU`
- `ALLOW_SHELL_ESCAPE=false`
- `READ_TIMEOUT=30s`
- `READ_HEADER_TIMEOUT=10s`
- `WRITE_TIMEOUT=60s`
- `IDLE_TIMEOUT=60s`

### Limits and timeouts

- `MAX_BODY_BYTES` caps request size.
- `COMMAND_TIMEOUT` applies to the LaTeX render pipeline.
- `WORKER_LIMIT` limits concurrent renders; extra requests return `429`.
- `ALLOW_SHELL_ESCAPE=false` keeps `pdflatex` in no-shell-escape mode. Set `true` to allow shell escape.
- `READ_TIMEOUT`, `READ_HEADER_TIMEOUT`, `WRITE_TIMEOUT`, `IDLE_TIMEOUT` control HTTP timeouts.

### What “untrusted LaTeX” means

LaTeX input comes from users. It can:

- reference local files via `\input`/`\include` (data exposure risk),
- use large constructs that consume CPU/memory (DoS risk),
- create very large outputs.

## Local run

Prereqs:

- `pdflatex` (TeX Live)
- `pdfcrop`
- `pdftoppm` (poppler-utils)

Run:

```bash
go run ./
```

## Docker (customer-owned)

```bash
docker build -t fulstech-latex-service .
docker run -d -p 8080:8080 fulstech-latex-service
```

## Operational guidance (customer-owned)

- Put the service behind auth and rate limiting.
- Run as non-root and read-only filesystem where possible.
- Do not mount host volumes into the container.
- Restrict network egress if feasible.
- Use `/healthz` for probes.

## Responsibilities split

App (this repo):

- Safe defaults (`-no-shell-escape`, timeouts, body size cap, worker cap).
- Health endpoint.
- Clear configuration docs.

Ops/customer:

- Edge auth (API key, JWT, mTLS) and rate limiting.
- Network egress restrictions.
- Container sandboxing (non-root, read-only FS, no host mounts).
- TeX Live file access policy (`texmf.cnf`).

## TeX Live file access control (customer-owned)

TeX Live can restrict file reads/writes via `texmf.cnf`:

- `openin_any` controls reads (`\\input`, `\\include`).
- `openout_any` controls writes (`\\openout`, auxiliary files).

Templates are provided in `docs/texlive/`:

- `texmf.cnf.paranoid` uses `openin_any=p` and `openout_any=p`.
- `texmf.cnf.permissive` uses `openin_any=a` and `openout_any=a`.

How customers can provide config (examples):

- Bake a custom `texmf.cnf` into their TeX Live image.
- Mount a config file and set `TEXMFCNF` to include its directory.

### Defaults and opt-in behavior

If you do **not** provide a custom `texmf.cnf`, TeX Live uses its **built-in defaults**. This service does not change TeX Live’s file access settings by itself.

### Example: mount a custom `texmf.cnf` (preferred)

```bash
docker run -d -p 8080:8080 \
  -v $(pwd)/docs/texlive/texmf.cnf.paranoid:/opt/texlive/texmf.cnf:ro \
  -e TEXMFCNF=/opt/texlive \
  fulstech-latex-service
```

Notes:

- `TEXMFCNF` points to a directory containing `texmf.cnf`.
- The mount path can be any location; keep it read-only.

## Troubleshooting

- `pdflatex error`/`pdfcrop error`/`pdftoppm error`: verify TeX Live and poppler utilities are installed.
- Empty response or 500: check service logs for full command output.
- Large inputs rejected: increase `MAX_BODY_BYTES`.

## Kubernetes example (customer-owned)

A minimal manifest is provided at `examples/k8s/latex-remote-service.yaml`. It is best-effort and not a supported deliverable.

Apply:

```bash
kubectl apply -f examples/k8s/latex-remote-service.yaml
```
