# cogged-client

TypeScript client for the [Cogged](../../README.md) API. Request/response **types are
generated from the backend's `openapi3.yaml`** (in this same repo), and a thin, typed
`fetch` client on top handles the auth-token flow and the graph endpoints.

> Types live in `src/generated/types.ts` and are produced by `openapi-typescript`. CI
> regenerates them and fails if they drift from `openapi3.yaml`, so the client can't fall
> behind the API. See "Keeping in sync with the spec" below.

## Install

```bash
npm install cogged-client
```

Requires a runtime with a global `fetch` (Node 18+, Deno, browsers) — or pass your own
via `options.fetch`.

## Usage

```ts
import { CoggedClient, CoggedApiError } from "cogged-client";

const cogged = new CoggedClient({ baseUrl: "http://localhost:8090" });

// Log in — the returned token is stored on the client automatically.
await cogged.login("alice", "s3cr3t-password");

// Create a node under the user, then read the AuthzData the server assigns it.
// createUserNode always keys created_nodes by the literal "new" — the server replaces
// the placeholder uid you supplied. (createNodes, below, keys by your placeholders.)
const created = await cogged.createUserNode({
  node: { uid: "$inbox", ty: "inbox", s1: "Alice's inbox", r: true },
});
const inboxAd = created.created_nodes?.["new"]?.ad; // opaque AuthzData token

// Create a subgraph under that node — here created_nodes IS keyed by the placeholders
// ("$msg1", "$msg2") you supplied.
const msgs = await cogged.createNodes(inboxAd!, {
  nodes: [
    { uid: "$msg1", ty: "msg", s1: "hello", r: true },
    { uid: "$msg2", ty: "msg", s1: "world", r: true },
  ],
});
const msg1Ad = msgs.created_nodes?.["$msg1"]?.ad;

// List the user's own nodes.
const mine = await cogged.listNodes("own", { select: ["id", "ty", "s1"] });

// Share a node with another user (look the user up first to get their token).
const bob = await cogged.getUserByName("bob");
if (bob.user?.ad && inboxAd) {
  await cogged.share({ nodes: [inboxAd], users: [bob.user.ad] });
}

try {
  await cogged.check();
} catch (e) {
  if (e instanceof CoggedApiError && e.status === 401) {
    await cogged.refresh();
  }
}
```

### Pagination

`query()` and `listNodes()` accept optional pagination fields on the `QueryRequest`:
`first` (max results), `offset` (skip N), `after` (a node uid cursor), and `order_by` /
`order_desc` (sort by an indexed predicate; default is uid order). Use `first` + `offset`
for offset-based paging, or `first` + `after` for cursor-based paging.

```ts
// Offset-based: page 3, 20 per page, newest first (order by created time `c`).
const page3 = await cogged.query({
  root_ids: [inboxAd],
  depth: 20,
  filters: { field: "ty", op: "eq", val: "message" },
  select: ["id", "ty", "s1", "c"],
  order_by: "c",
  order_desc: true,
  first: 20,
  offset: 40,
});

// Cursor-based (preferred for deep/infinite scroll): walk pages via the last uid.
async function* allMessages(parent: string) {
  let after: string | undefined;
  for (;;) {
    const res = await cogged.query({
      root_ids: [parent],
      depth: 20,
      filters: { field: "ty", op: "eq", val: "message" },
      select: ["id", "s1"],
      first: 50,
      ...(after ? { after } : {}),
    });
    const nodes = res.result_nodes ?? [];
    if (nodes.length === 0) break;
    yield* nodes;
    after = nodes[nodes.length - 1]!.uid; // cursor = last uid returned
  }
}

// listNodes takes the same pagination fields:
const firstTen = await cogged.listNodes("shared", { select: ["id", "ty"], first: 10 });
```

Notes:
- The server applies read-permission filtering *inside* the query, so pages contain only
  nodes you may read and are not silently shortened — a page shorter than `first` (or empty)
  means the end of the results.
- `order_by` must name an **indexed, sortable** predicate (e.g. `c`, `m`); ordering by a
  hash-only predicate is rejected by the backend. Disallowed field names are ignored.
- `after` (cursor) is cheaper than `offset` for deep paging.

### Vector similarity search

Nodes can carry an embedding in the `vec` field (a string-encoded float array), and
`query()` / `listNodes()` can rank nodes by nearness to a query vector via `similar`.
Embeddings are produced by your own model — Cogged just stores and searches them.

```ts
// Store an embedding on a node (produce the vector with your own embedding model).
await cogged.createUserNode({
  node: { uid: "$doc", ty: "doc", s1: "the quick brown fox", r: true, vec: "[0.12, -0.03, 0.88]" },
});

// Find the 5 nodes most similar to a query embedding (scoped to what you can read).
const hits = await cogged.query({
  similar: { vector: "[0.10, -0.01, 0.90]", top_k: 5 },
  select: ["id", "ty", "s1"],
});
for (const n of hits.result_nodes ?? []) {
  console.log(n.id, n.s1);
}
```

Notes:
- `vec` is a **string-encoded** float array (e.g. `"[0.1,0.2,0.3]"`) — the format Dgraph's
  `float32vector` expects. All vectors compared in one search must have the same dimension.
- Results are filtered to nodes you're allowed to read, so a search can return **fewer than
  `top_k`** hits if some of the nearest vectors belong to other users.
- `top_k` defaults to 10 (capped at 1000); `similar` can be combined with `filters` and `select`.

### Geo radius search

Nodes can carry a location in the `g` field, and `query()` / `listNodes()` can return every
node within a radius of a point via `geo`.

```ts
// Store a location. Coordinates are [longitude, latitude] — longitude FIRST.
await cogged.createUserNode({
  node: {
    uid: "$cafe", ty: "cafe", s1: "Corner Roasters", r: true,
    g: { type: "Point", coordinates: [151.2153, -33.8568] },
  },
});

// Every cafe within 5km of the Sydney Opera House (scoped to what you can read).
const nearby = await cogged.query({
  geo: { point: [151.2153, -33.8568], distance: 5000 }, // metres
  filters: { field: "ty", op: "eq", val: "cafe" },
  select: ["id", "s1", "g"],
});
```

Notes:
- **`point` is `[longitude, latitude]`**, per GeoJSON — the same order the node stores.
  Swapping them searches the wrong place, or is rejected if the latitude exceeds ±90.
- This is a radius test, **not "the N nearest"**. Matches come back in uid order, distance is
  neither returned nor sortable (`order_by: "g"` is rejected), so pairing `geo` with `first`
  gives an arbitrary N inside the radius. Sort client-side from the returned `g` if you need
  nearest-first.
- Like `similar`, `geo` replaces the query root: `root_ids`/`depth` are ignored, `filters` and
  `select` still apply, and `geo` + `similar` together is an error.
- `distance` is in metres, must be > 0, and is capped at 20,100,000.
- `g` cannot be used in `filters` or `order_by` — the `geo` block is the only way to query it.

### AuthzData

`AuthzData` (the `ad` field on nodes and users) is an **opaque, server-signed token**.
Read it from a response and pass it back unchanged in later requests (e.g. as the parent
of `createNodes`, or in `share`). Never build or edit one — the server verifies its
signature and will reject tampered values.

### Auth & token handling

`login()` and `refresh()` store the bearer token on the client; `logout()` clears it.
You can also manage it yourself with `setToken()` / `getToken()` (e.g. to persist a
session). Every request sends `Content-Type: application/json`, which the Cogged server
requires on all requests.

## API surface

`login` · `logout` · `check` · `refresh` · `clientConfig` · `createUser` · `updateUsers`
· `query` · `sharedWith` · `updateNodes` · `createNodes` · `addEdges` · `removeEdges` ·
`createUserNode` · `listNodes` · `share` · `unshare` · `getUserByUid` · `getUserByName` ·
`health`. Every DTO type (`QueryRequest`, `GraphNode`, `CoggedResponseRN`, …) is exported.

## Development

```bash
npm install
npm run gen        # regenerate src/generated/types.ts from ../../openapi3.yaml
npm run typecheck  # tsc --noEmit
npm run build      # emit dist/ (js + d.ts)
npm run check:spec # regenerate and fail if the committed types differ from the spec
```

### Releasing

Publishing to npm is automated by [`.github/workflows/npm-publish.yml`](../../.github/workflows/npm-publish.yml),
triggered by a git tag of the form **`cogged-client-v<version>`**. This prefix is
deliberately separate from the Go release tags (`v*`, handled by goreleaser), so the two
release channels never collide.

**One-time setup:** an `NPM_TOKEN` [repository secret](https://github.com/0x0abc123/cogged/settings/secrets/actions)
must exist — an npm automation or granular access token with publish rights to the
`cogged-client` package.

**To cut a release:**

1. **Bump the version** in [`package.json`](./package.json) (e.g. `0.2.0` → `0.2.1`),
   following semver. If the API changed, first make sure the generated types are current
   (`npm run gen` and commit any change).
2. **Merge that to `main`** via a normal PR (CI's `ts-client` job must be green).
3. **Tag and push** — the tag version *must* match `package.json` or the workflow fails:

   ```bash
   git checkout main && git pull
   git tag cogged-client-v0.2.1
   git push origin cogged-client-v0.2.1
   ```

4. **Watch the run** under the repo's Actions tab (`npm-publish` workflow). On success the
   new version appears at <https://www.npmjs.com/package/cogged-client>.

The workflow: `npm ci` → verify the tag matches `package.json` → `check:spec` (re-generate
types and fail if they differ from `openapi3.yaml`) → `npm run build` →
`npm publish --provenance --access public`.

**Troubleshooting:**

- *"tag … does not match package.json version"* — the tag's version and `package.json`
  disagree. Delete the tag (`git push origin :cogged-client-v0.2.1`), fix `package.json` on
  `main`, and re-tag.
- *`check:spec` fails* — the committed `src/generated/types.ts` is stale; run `npm run gen`,
  commit, and re-tag.
- *provenance error* — if npm provenance ever blocks the publish, remove `--provenance`
  from the workflow's publish step and the `id-token: write` permission.
- Publishing the same version twice is rejected by npm — always bump first.

### Keeping in sync with the spec

Because this package lives in the Cogged monorepo, `npm run gen` reads
`../../openapi3.yaml` directly — there is no spec to fetch or version to pin. A change to
an endpoint updates the handler, the spec (enforced by the Go drift guard in `spec/`), and
the client types (enforced by `check:spec` in CI) in a single PR. When you change the API:

1. update the handler and `openapi3.yaml` (the Go drift test enforces they match),
2. run `npm run gen` here and commit the updated `src/generated/types.ts`,
3. adjust the hand-written client in `src/client.ts` if a new endpoint was added.
