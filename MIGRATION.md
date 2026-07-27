# Migrating Dgraph: v22 → v25

Cogged **v2.0.0** requires **Dgraph v25** (it was v22.0.2 through v1.0.x). The bump is a
real backing-store migration, not a drop-in binary swap.

## Why you can't just point v25 at the old data

Dgraph does **not** guarantee on-disk data-format compatibility across major versions — the
posting-list/encoding format can change, so starting a v25 Alpha against a v22 `p`/`w` data
directory is unsupported and can fail or corrupt. The supported path is to **export** the
logical data from v22 and **import** it into a fresh v25 cluster. Cogged's schema also gained
the `vec` (`float32vector`, HNSW) predicate, which only exists in Dgraph v24+, so v22 cannot
run the v2.0.0 schema at all.

The logical data is fully portable — the export format (RDF/JSON + schema) is
version-independent. You do **not** need to step through v23/v24; export from v22 and load
straight into v25.

## Before you start

- **Back up** the v22 data directories and keep the export archive.
- **Test the whole flow on a copy** of production first, and validate a few queries against
  v25 before cutting over.
- Plan for read-only or downtime during the export/cutover so no writes are lost.

## 1. Export from the v22 cluster

Trigger an export via the Alpha's admin endpoint (adjust host/port):

```bash
curl -s -H "Content-Type: application/json" -X POST http://<v22-alpha>:8080/admin \
  -d '{"query":"mutation { export(input: {format: \"rdf\"}) { response { message code } } }"}'
```

This writes, per data group, into the Alpha's `export/` directory (or the configured export
location / object store):

- `g01.rdf.gz` — the data
- `g01.schema.gz` — the DQL schema
- `g01.gql_schema.gz` — the GraphQL schema (unused by Cogged; safe to ignore)

Collect the `.rdf.gz` and `.schema.gz` files.

> Cogged uses a DQL schema that it re-applies on startup (see below), so the exact schema
> file is not strictly required — but loading it preserves predicate types during import.

## 2. Stand up a fresh v25 cluster

```bash
docker pull dgraph/standalone:v25.3.1
# for production, run separate `dgraph zero` + `dgraph alpha`; standalone is fine for dev/test
```

Start it against **empty** data directories.

## 3. Import the data into v25

Pick one:

**Bulk loader — fastest, initial import into a *new* cluster only:**

```bash
dgraph bulk -f g01.rdf.gz -s g01.schema.gz --zero <v25-zero>:5080 --out ./out
# then place ./out/0/p into the Alpha's data directory before starting the Alpha
```

**Live loader — loads into an already-running cluster:**

```bash
dgraph live -f g01.rdf.gz -s g01.schema.gz --alpha <v25-alpha>:9080 --zero <v25-zero>:5080
```

Use the bulk loader for large datasets (millions+ of triples) and a one-shot migration; use
the live loader for smaller datasets or when the target Alpha is already serving.

> Check `dgraph bulk --help` / `dgraph live --help` on your exact v25 build for current flag
> names — they occasionally change between releases.

## 4. Point Cogged at the new cluster

Update Cogged's Dgraph target (any one of):

- `cogged.conf.json`: `db.host` / `db.port` (or a full `db.connstr`, e.g.
  `dgraph://host:9080?sslmode=disable`),
- the `-dh` / `-dp` CLI flags, or
- `COGGED_CONFIG_FILE` pointing at the right config.

On startup, Cogged's `MaybeUpdateSchema` reconciles the schema: it detects that the schema
version marker doesn't match and re-applies the full current schema via `Alter`, which adds
the new `vec` predicate (and any others) on top of your imported data. This is additive and
idempotent for predicates that already match.

## 5. After migration

- Existing nodes have **no embedding** — the `vec` predicate is empty until you backfill
  vectors (see the vector-search docs in [`docs/about.md`](./docs/about.md#vector-similarity-search)).
  Everything else works unchanged.
- If you use **ACL** (enterprise), run the `dgraph upgrade` CLI to migrate ACL data
  structures across versions. OSS / no-ACL deployments don't need it.

## Rollback

Keep the v22 data directories and the export archive until you've verified v25 in
production. To roll back, point Cogged (a v1.0.x build) back at the untouched v22 cluster.

## References

- [Dgraph upgrade guide](https://docs.dgraph.io/cli/upgrade/)
- [Initial import / bulk loader](https://docs.dgraph.io/migration/bulk-loader/)
