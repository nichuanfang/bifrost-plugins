---
name: bifrost-dockerfile-sync
description: |
  Sync the bifrost-plugins Dockerfile with the official bifrost upstream Dockerfile.
  Use this skill whenever the user wants to update, sync, upgrade, or refresh their
  bifrost-plugins Dockerfile, or mentions "Dockerfile sync", "update Dockerfile",
  "sync bifrost Dockerfile", "upgrade Dockerfile to latest version", or similar.
  Also trigger when the user talks about updating the bifrost version or aligning
  their Dockerfile with upstream changes.
---

# Bifrost Dockerfile Sync

Sync the project's `Dockerfile` against the official bifrost upstream
`/transports/Dockerfile`, applying stage-by-stage transformations.

## Workflow

### Step 1: Read version info

Read `README.md` from the project root. Extract the value of `bifrost-tag`
(e.g. `transports/v1.6.9`). This tag determines which branch of
`github.com/maximhq/bifrost` to fetch.

### Step 2: Fetch the official upstream Dockerfile

Fetch the official Dockerfile from:

```
https://raw.githubusercontent.com/maximhq/bifrost/refs/tags/<bifrost-tag>/transports/Dockerfile
```

If that URL returns 404, try:

```
https://raw.githubusercontent.com/maximhq/bifrost/<bifrost-tag>/transports/Dockerfile
```

Read the current project `Dockerfile` as well. You will compare these two files
stage by stage.

### Step 3: Stage-by-stage reconciliation

The project Dockerfile has five stages. Reconcile each one:
update stages that mirror upstream, preserve project-specific stages,
and adapt COPY commands.

---

#### Stage A: `bifrost-source` (project-specific)

This stage clones the bifrost source — it does **not** exist upstream.
Keep this stage as-is; only update:

- The `ARG BIFROST_TAG` default value at the top of the Dockerfile to match
  the tag from README.md (if it changed).
- The base image (e.g. `alpine:3.23.4`) if the project is using a newer one.

---

#### Stage B: `ui-builder` (mirrors upstream UI build)

This stage mirrors the upstream UI build stage. Read the upstream Dockerfile
and find its UI build stage (usually `AS ui-builder` or similar).

**Transformation rules for every `COPY` instruction:**

Upstream has `COPY <src> <dest>` because its context is the repo root.
But this project has no bifrost source in its build context — it comes from
the `bifrost-source` stage. So:

- `COPY ui/package*.json ./` → `COPY --from=bifrost-source /src/ui/package*.json ./`
- `COPY ui/ ./` or `COPY ui .` → `COPY --from=bifrost-source /src/ui ./`
- Any other `COPY <path> <dest>` where `<path>` refers to a file/dir in the
  bifrost repo under `/transports/` → prefix with `--from=bifrost-source /src/`

**Everything else** (FROM, WORKDIR, RUN npm ci, RUN npm run build, etc.) stays
identical to upstream. If upstream adds or removes a RUN step, follow it.

---

#### Stage C: `builder` (mirrors upstream Go build)

This stage mirrors the upstream Go build stage (usually `AS builder` or similar).

**Transformation rules:**

- `COPY go.mod go.sum ./` or similar → `COPY --from=bifrost-source /src/transports/go.mod /src/transports/go.sum ./`
- `COPY . ./` or `COPY . .` → `COPY --from=bifrost-source /src/transports/ ./`
- UI output copy: If upstream does `COPY --from=ui-builder /app/out ./ui`, adapt to
  `COPY --from=ui-builder /app/out ./bifrost-http/ui` (preserving the existing
  project path convention).
- `RUN go build ...` → keep the flags and ldflags, especially `-X main.Version=v${VERSION}`
  which uses `BIFROST_TAG`.

Follow upstream for: FROM image, RUN apk add packages, ENV settings,
go mod download, and the general build flow. Do not drop any existing
project-specific lines (like `RUN ls`, `RUN cat go.mod`) unless they
are clearly debug leftovers that upstream removed.

---

#### Stage D: `plugin-builder` (project-specific)

This stage compiles `.so` plugin files — it does **not** exist upstream.
Preserve this stage. Only update:

- The `FROM` image to match the `builder` stage's Go image (same Go version).
- The `RUN apk add` packages to match the builder stage if they changed.

The plugin compilation loop and `--mount` cache directives should stay.

---

#### Stage E: Runtime (mirrors upstream runtime)

This is the final stage. Follow the upstream runtime stage structure, but:

- `COPY --from=builder /app/main .` stays as-is (or adapt to match upstream's binary path).
- `COPY --from=builder /app/docker-entrypoint.sh .` stays.
- **Always add** `COPY --from=plugin-builder /app/build/ ./plugins/` — this is
  the key line that loads compiled plugins.
- Preserve project-specific ENV vars: `ARG_APP_PORT`, `ARG_APP_HOST`,
  `ARG_LOG_LEVEL`, `ARG_LOG_STYLE`, `ARG_APP_DIR` and their corresponding
  `ENV` declarations.
- Preserve `ENV GOGC="" GOMEMLIMIT=""`.
- Match upstream for: FROM image, apk packages, HEALTHCHECK, USER, VOLUME,
  ENTRYPOINT, CMD.

---

### Step 4: Write the updated Dockerfile

After reconciling all stages, write the updated `Dockerfile` back to the
project root. Then report a summary:

```
## Sync Summary

- bifrost-tag: transports/vX.Y.Z (was: transports/vA.B.C)
- Upstream Dockerfile: transports/vX.Y.Z

### Changes by stage:
- bifrost-source: <what changed>
- ui-builder: <what changed, e.g. "updated FROM image, aligned COPY commands">
- builder: <what changed>
- plugin-builder: <what changed>
- runtime: <what changed>
```

### Important guidelines

- **Never guess URLs.** Use the exact raw.githubusercontent.com pattern above.
  If both URL patterns fail, report the error and ask the user to verify the tag.
- **Preserve project-specific additions.** The `bifrost-source` and `plugin-builder`
  stages, the `docker-entrypoint.sh` copy, the custom ENV vars — these are
  intentionally added and must not be dropped.
- **When in doubt about a transformation, keep the existing project version**
  and note it in the summary. Prefer under-syncing to breaking the build.
- The upstream Dockerfile is the **only reference** for what the "official"
  stages should look like. Do not invent or guess upstream content.
