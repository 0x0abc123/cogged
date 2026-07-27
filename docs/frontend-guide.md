# Building a frontend on the Cogged TypeScript client

Guidance for Claude Code when writing an application frontend that uses
[`clients/typescript`](../clients/typescript) (`cogged-client`) as its data-access layer.

Cogged is not a document store with a flexible schema. It is a **fixed-width graph node**: every
node has the same handful of predicates (`s1`–`s4`, `n1`/`n2`, `t1`/`t2`, `b`, …), and the index on
each predicate decides which queries are possible. So the central design act on the frontend is
**assigning your domain object's properties to those slots**, driven by how you intend to query
them — not by what the property "means".

Read this together with:

- [`../CLAUDE.md`](../CLAUDE.md) — backend architecture and the AuthzData access-control model.
- [`../clients/typescript/README.md`](../clients/typescript/README.md) — client API surface,
  pagination, vector search.
- [`../openapi3.yaml`](../openapi3.yaml) — the authoritative request/response shapes.

---

## 1. Before you write any code

Three environment facts that will otherwise cost you a debugging session:

**The server sends no CORS headers.** `cmd/cogged/main.go` sets only `Content-Type`. The client also
sends `Content-Type: application/json` and `Authorization` on *every* request (including GETs),
which forces a CORS preflight. A browser app served from a different origin than the API will
therefore fail. Serve the frontend and the API from the same origin via a dev-server proxy or a
reverse proxy in front of both:

```ts
// vite.config.ts
export default {
  server: {
    proxy: {
      "/auth": "http://localhost:8090",
      "/graph": "http://localhost:8090",
      "/user": "http://localhost:8090",
      "/health": "http://localhost:8090",
    },
  },
};
```

Then `new CoggedClient({ baseUrl: "" })` and everything is same-origin.

**The server sends no `ETag`/`Cache-Control`.** Browser HTTP caching cannot help you. All caching
must be in application state — see §6.

**Shared-node access lives in server memory.** The per-user SGI allowlist (`state/`) is populated
only by `POST /user/nodes/shared` and by a share call, and it is lost on server restart. A session
that wants to touch nodes shared *with* it must call `listNodes("shared")` first — see §7.

---

## 2. Session setup

```ts
import { CoggedClient, CoggedApiError } from "cogged-client";

export const cogged = new CoggedClient({ baseUrl: "" });

// Tokens are bearer tokens with a configurable expiry (auth.tokenexpiry, seconds;
// the shipped cogged.conf.json uses 86400). Persist and restore across reloads:
const saved = sessionStorage.getItem("cogged.token");
if (saved) cogged.setToken(saved);

export async function login(username: string, password: string) {
  const { token } = await cogged.login(username, password);
  if (token) sessionStorage.setItem("cogged.token", token);
  await primeSession(); // see §7
}
```

Wrap every call in one place so a 401 triggers a single refresh-and-retry rather than a refresh per
in-flight request:

```ts
let refreshing: Promise<unknown> | null = null;

export async function withAuth<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (e) {
    if (!(e instanceof CoggedApiError) || e.status !== 401) throw e;
    refreshing ??= cogged.refresh().finally(() => (refreshing = null));
    await refreshing;
    sessionStorage.setItem("cogged.token", cogged.getToken() ?? "");
    return await fn();
  }
}
```

Prefer `sessionStorage` over `localStorage` for the token. The token is a bearer credential and
there is no server-side revocation beyond `logout()` and expiry.

---

## 3. The mental model in one page

A `GraphNode` as it comes back from a query has two halves.

**The authz envelope** — always returned, never invented by you:

| Field | Meaning |
|---|---|
| `uid` | Dgraph id, `0x…`. Stable. Use it as your cache key. |
| `ad` | Opaque signed AuthzData. **This** is what you pass back in requests, not `uid`. |
| `own` | `{ uid }` of the owner. |
| `sgi` | Share-group id; server-assigned, read-only. Sharing grants access to a whole SGI. |
| `r` `w` `o` `i` `d` `s` | Permission bits: read, write, out-edge, in-edge, delete, share. |

`ad` is base64url + `.`, so it is safe to place in a URL path segment (the client already
`encodeURIComponent`s it).

**The data payload** — yours to assign, and only returned if you ask for it in `select`:
`ty`, `id`, `p`, `s1`–`s4`, `b`, `n1`, `n2`, `c`, `m`, `t1`, `t2`, `g`, plus `e` (out-edges) and
`vec` (write-only; see the table below).

Structure comes from edges, not from foreign keys: a container node points at its children via `e`,
and a query walks outwards from `root_ids` for `depth` levels (max 20).

---

## 4. The predicate budget

This is the table to design against. "Ops" means what the API can actually express — the backend
compiles `filters` into DQL (`services/db.go`), so a filter is only usable if the Dgraph index in
`services/dbsetup.go` supports the generated function.

| Field | TS type | Dgraph index | Usable filter ops | Sortable | Intended for |
|---|---|---|---|---|---|
| `ty` | `string` | `hash` | `eq` | ✗ | **Type discriminator.** Always set, always select. |
| `id` | `string` | `trigram`, `term` | `has` (substring) | ✗ | Your app-assigned stable external key. |
| `s1` | `string` | `trigram`, `term` | `has` (substring) | ✗ | **The only substring-searchable text field.** |
| `s2` | `string` | `term` | — (see note) | ✗ | Display text you never query on. |
| `s3` | `string` | `hash` | `eq` | ✗ | **Exact-match facet #1** (status, enum, slug). |
| `s4` | `string` | `hash` | `eq` | ✗ | **Exact-match facet #2** (foreign key, category). |
| `p` | `string` | `hash` | — (admins only) | ✗ | **Owner-private.** Not readable or filterable by anyone but the owner and admins — see §8. |
| `b` | `string` | none | — | ✗ | **Overflow.** JSON blob, markdown body, base64. |
| `n1` `n2` | `number` | none | — | ✗ | Numbers you display or compute with only. |
| `c` | ISO string | `datetime(hour)` | `eq` `gt` `lt` `ge` `le` | ✓ | Created — **server-set, read-only.** |
| `m` | ISO string | `datetime(hour)` | `eq` `gt` `lt` `ge` `le` | ✓ | Modified — **server-set**; drives delta sync (§6). |
| `t1` `t2` | ISO string | `datetime(hour)` | `eq` `gt` `lt` `ge` `le` | ✓ | **Your two range-queryable / sortable fields.** |
| `g` | `Geoloc` | `geo` | `geo` block only | ✗ | **Radius search.** Not usable in `filters` — see below. |
| `vec` | `string` | `hnsw(cosine)` | `similar` only | n/a | Embedding. **Not in `select` — write-only.** |
| `e` | `NodeEdgeData[]` | `@reverse` | — | n/a | Out-edges; include in `select` to see structure. |

Consequences worth internalising:

- **You get exactly one substring-search field: `s1`.** Treat it as a denormalised *search
  document*, not as "the title". If users must search title *and* body, put
  `` `${title}\n${body}` `` in `s1` and keep the display copy elsewhere.
- **`has` becomes a case-insensitive regex** (`regexp(s1,/…/i)`) and only when the value is longer
  than 2 characters. Debounce search input and require ≥3 characters before querying.
- **You get exactly two sortable/range-queryable fields: `t1` and `t2`** (plus the server's `c` and
  `m`). A "sort order" or "priority rank" must live in `t1`/`t2` if the server is to sort it —
  `n1`/`n2` are unindexed and cannot be filtered *or* sorted. `order_by` on an unsortable predicate
  is rejected by Dgraph and surfaces as a generic 500-ish `"DB query failed"`.
- **`eq` needs a hash index**, so exact-match facets go in `s3`/`s4`/`ty`. `p` is hash-indexed too
  but the API refuses to filter on it unless you are an admin (see §8), so it is **not** a third
  facet slot — never design a query around it. Dgraph nominally
  permits `eq` against a `term` index, but the semantics are token-based — do not design around
  `eq` on `s1`, `s2`, or `id`.
- **`b`, `n1`, `n2`, `s2` are invisible to queries.** That is fine and it is where most of a
  domain object should live.
- **`g` is queryable, but only through its own request block** (`geo`), never through `filters`.
  Naming `g` in a filter clause or `order_by` is rejected outright. See §5a.

---

## 5. Mapping a domain object to predicates

### The recipe

For each property of your domain type, ask in order:

1. **Do I substring-search it?** → fold it into `s1`.
2. **Do I filter it by exact value?** → `s3`, then `s4`. (Two slots. Spend them carefully.)
3. **Do I filter it by range, or sort by it?** → `t1`, then `t2`. Encode non-dates as dates if you
   must (a rank as an epoch offset), or accept client-side sorting.
4. **Do I search it semantically?** → embed it and write `vec`.
5. **Is it a place I search by proximity?** → `g` (see §5a).
6. **Otherwise** → it goes in the `b` JSON blob.

Then: `ty` is the type name, `id` is your stable app key, and structure is edges.

Two properties competing for `s3` is the signal to **split the type into two nodes** joined by an
edge, rather than to overload a slot.

---

## 5a. Geo: radius search on `g`

`g` stores one GeoJSON point per node and is geo-indexed. It has its own request block rather
than a filter op, because no `filters` operator can express a geo predicate:

```ts
const res = await cogged.query({
  geo: { point: [151.2153, -33.8568], distance: 5000 }, // metres
  filters: { field: "ty", op: "eq", val: "cafe" },
  select: ["id", "s1", "g"],
});
```

**`point` is `[longitude, latitude]`.** Longitude first, per GeoJSON, and the same order the node
stores. Swapping them is the mistake everyone makes once: it either silently searches the wrong
place or, if the latitude lands outside ±90, gets rejected. Write a helper that takes named
arguments and never pass a bare array around your app.

Four things to internalise:

- **It is a radius test, not "nearest N".** Dgraph returns geo matches in **uid order**, cannot
  sort by distance (`order_by: "g"` is rejected — geo values are not sortable) and never returns
  the computed distance. So `{geo: {...}, first: 10}` gives you **an arbitrary 10 inside the
  radius, not the 10 nearest.** If you need nearest-first you must fetch the whole radius and sort
  client-side, which means keeping the radius small enough that the whole set is affordable.
- **At the request level it replaces the root function**, exactly like `similar`. `root_ids` and
  `depth` are ignored when the top-level `geo` is set, and `geo` + `similar` together is an error.
  `filters` and `select` still apply.
- **Access control still applies.** Results are read-filtered like any other query — a geometric
  match on a node you cannot read returns nothing.
- **Distance is capped** at 20,100,000 m (past roughly half the Earth's circumference every point
  matches). `distance` must be > 0.

To display distance in a list, compute it client-side from the returned `g` — the server won't
give it to you.

### Composing proximity with other filters

The block above is a whole-database radius search. To ask "children of *this* node that are also
nearby", put the same `geo` object on a **filter clause** instead — then it is one term among
others and composes with `and`/`or` and with a `root_ids` traversal:

```ts
// Cafes under a specific folder, within 5km.
await cogged.query({
  root_ids: [folderAd],
  depth: 5,
  filters: {
    and: [
      { field: "ty", op: "eq", val: "cafe" },
      { geo: { point: [151.2153, -33.8568], distance: 5000 } },
    ],
  },
  select: ["id", "s1", "g"],
});

// Two cities at once.
filters: { or: [
  { geo: { point: [151.2153, -33.8568], distance: 5000 } },
  { geo: { point: [144.9631, -37.8136], distance: 5000 } },
]}
```

- A geo clause sets **only** `geo` — `field`/`op`/`val` must be left off, and setting both is
  rejected rather than quietly ignored.
- Setting the request-level `geo` *and* a geo clause is allowed, and intersects the two radii.
- Every caveat from above still applies: uid order, no distance returned, no distance sort.

Rule of thumb: **request-level `geo` for "what's near me", a geo clause for "which of *these* are
near me".**

### Keep the mapping in exactly one place

Never scatter `s3` literals through components. One module per domain type, holding the codec and
nothing else.

```ts
// domain/task.ts
import type { GraphNode } from "cogged-client";

export const TASK_TY = "task";
export type TaskStatus = "open" | "blocked" | "done";

export interface Task {
  uid?: string;          // set once persisted
  ad?: string;           // authz token; required to update/share
  id: string;            // app key, e.g. "task/01J8..."
  title: string;
  body: string;
  status: TaskStatus;    // -> s3 (eq filter)
  projectId: string;     // -> s4 (eq filter)
  dueAt?: string;        // -> t1 (range + sort)
  estimateHours?: number;// -> n1 (display only)
  tags: string[];        // -> b (never queried)
  assigneeName?: string; // -> s2 (displayed, never queried)
  createdAt?: string;    // <- c  (server)
  updatedAt?: string;    // <- m  (server)
}

/** Fields to request for list views — omit `b` so list payloads stay small. */
export const TASK_LIST_SELECT = ["ty", "id", "s1", "s3", "s4", "t1", "n1", "m"] as const;
/** Detail view additionally needs the blob and the secondary text. */
export const TASK_DETAIL_SELECT = [...TASK_LIST_SELECT, "s2", "b", "c", "e"] as const;

interface TaskBlob {
  body: string;
  tags: string[];
}

/** Domain -> node. Only the payload; the authz envelope is handled by the repository. */
export function encodeTask(t: Task): GraphNode {
  const blob: TaskBlob = { body: t.body, tags: t.tags };
  return {
    ty: TASK_TY,
    id: t.id,
    s1: `${t.title}\n${t.body}`.slice(0, 4096), // the one searchable field
    s2: t.assigneeName,
    s3: t.status,
    s4: t.projectId,
    b: JSON.stringify(blob),
    n1: t.estimateHours,
    t1: t.dueAt,
  };
}

/** Node -> domain. Tolerate every field being absent: `select` decides what arrives. */
export function decodeTask(n: GraphNode): Task {
  const blob: Partial<TaskBlob> = n.b ? safeParse(n.b) : {};
  const s1 = n.s1 ?? "";
  return {
    uid: n.uid,
    ad: n.ad,
    id: n.id ?? "",
    title: s1.split("\n", 1)[0] ?? "",
    body: blob.body ?? "",
    status: (n.s3 as TaskStatus) ?? "open",
    projectId: n.s4 ?? "",
    dueAt: n.t1,
    estimateHours: n.n1,
    tags: blob.tags ?? [],
    assigneeName: n.s2,
    createdAt: n.c,
    updatedAt: n.m,
  };
}

function safeParse(s: string): Partial<TaskBlob> {
  try {
    return JSON.parse(s) as Partial<TaskBlob>;
  } catch {
    return {};
  }
}
```

Notes on this shape:

- `title` is derived from the first line of `s1`, so there is a single source of truth for the
  searchable text. The alternative — title in `s3` and body in `s1` — costs you an exact-match slot
  and makes titles unsearchable-by-substring only if you also duplicate them.
- `decodeTask` must never assume a field is present. `select` is a projection, and a list-view node
  legitimately has no `b`.
- `id` is **not** uniqueness-enforced by Dgraph. If you need uniqueness, generate a ULID/UUID
  client-side and treat collisions as impossible rather than checking.

### Multiple types in one subgraph

Dispatch on `ty` after a single traversal, so one round trip populates several stores:

```ts
export function decodeAny(n: GraphNode) {
  switch (n.ty) {
    case TASK_TY: return { kind: "task" as const, value: decodeTask(n) };
    case PROJECT_TY: return { kind: "project" as const, value: decodeProject(n) };
    default: return null; // unknown ty: ignore, don't throw
  }
}
```

---

## 6. The repository layer

One repository per aggregate. It owns AuthzData, the endpoint choice, and the cache.

### Creating

Two different endpoints, and the response key differs between them — this is the most common
mistake:

| Call | Use for | `created_nodes` key |
|---|---|---|
| `createUserNode({ node })` | A **root** node owned by the user. Gets a **fresh SGI**. | **`"new"`** |
| `createNodes(parentAd, { nodes, reset_sgi })` | A subgraph under an existing parent (needs `o` on the parent). Inherits the parent's SGI unless `reset_sgi: true`. | the `$placeholder` verbatim, e.g. `"$t1"` |

> `createUserNode` ignores the `$placeholder` you send: `UpsertUserNode` (`services/db.go`)
> rewrites the uid to `_:new` and keys the response `"new"` (see `db_integration_test.go:114`).
> The `$placeholder` is still required by `Validate()`, it just doesn't come back to you.

New nodes use `$placeholder` uids, and placeholders may cross-reference each other in `e` to create
a whole subgraph in **one** round trip:

```ts
async function createTasks(parentAd: string, tasks: Task[]) {
  const nodes = tasks.map((t, i) => ({
    uid: `$t${i}`,
    ...encodeTask(t),
    r: true,  // permission bits are set at creation time
    w: false,
    o: false, i: false, d: false, s: false,
  }));
  const res = await withAuth(() => cogged.createNodes(parentAd, { nodes, reset_sgi: false }));
  return tasks.map((t, i) => ({ ...t, ...pickEnvelope(res.created_nodes?.[`$t${i}`]) }));
}
```

Permission bits (`r`/`w`/`o`/`i`/`d`/`s`) grant access to *other* users who reach the node through a
share; the owner always has full access. Set them at creation and think of them as part of the
document's schema. Omitted bits default to `false`.

Only one level of `e` nesting is honoured per create request, and self-links are rejected.

### Reading

Everything is `query()` (traversal from `root_ids`) or `listNodes(scope)` (the user's own or
shared-with roots, always depth 1).

```ts
async function loadProjectTasks(projectAd: string) {
  const res = await withAuth(() =>
    cogged.query({
      root_ids: [projectAd],
      depth: 3,
      filters: { field: "ty", op: "eq", val: TASK_TY },
      select: [...TASK_LIST_SELECT],
      order_by: "t1",
      order_desc: false,
      first: 50,
    }),
  );
  return (res.result_nodes ?? []).map(decodeTask);
}
```

`filters` is **one level deep only**. Either set `field`/`op`/`val` for a single predicate, or set
`and`/`or` to an array of flat clauses — in which case `field`/`op`/`val` are ignored. (The Go
implementation happens to recurse, but the spec and the generated TS types model only one level;
stay within them.)

```ts
filters: {
  and: [
    { field: "ty", op: "eq", val: TASK_TY },
    { field: "s3", op: "eq", val: "open" },
    { field: "t1", op: "lt", val: new Date().toISOString() },
  ],
}
```

**Project narrowly.** Two `select` tiers per type — a list projection without `b`, and a detail
projection with it — is the single highest-leverage bandwidth optimisation, because `b` is usually
most of the payload. A detail fetch is `query({ root_ids: [ad], depth: 0, select: DETAIL })`.

### Updating

`updateNodes` is a partial upsert: predicates you omit are left untouched. But the request must
**echo the node's authz envelope exactly** — `AuthzDataUnpackNodeSlice` (`models/node.go`) verifies
the `ad` signature *and* checks that `uid`, `own`, `sgi`, and all six permission bits in your
payload match what the token says. A missing `own` or a dropped permission bit fails the whole
request as a 400.

```ts
/** Everything the server insists you echo back on an update. */
function envelopeOf(n: GraphNode) {
  return {
    uid: n.uid, ad: n.ad, own: n.own, sgi: n.sgi,
    r: n.r ?? false, w: n.w ?? false, o: n.o ?? false,
    i: n.i ?? false, d: n.d ?? false, s: n.s ?? false,
  };
}

async function saveTaskStatus(cached: GraphNode, status: TaskStatus) {
  await withAuth(() =>
    cogged.updateNodes({ nodes: [{ ...envelopeOf(cached), s3: status }] }),
  );
}
```

So **cache the raw node alongside the decoded domain object** — you cannot reconstruct the envelope
from your domain type. This is why the cache in §6 stores `GraphNode`, not `Task`.

`updateNodes` takes an array: batch all pending edits from one user interaction into a single call.

### Deleting

**There is no delete endpoint.** The API surface is create / query / update / edges / share. Model
deletion as either:

- **Unlink** — `removeEdges({ subject_ids: [parentAd], outgoing_ids: [childAd] })`, which bumps `m`
  on both endpoints; or
- **Tombstone** — a status value in `s3` (e.g. `"deleted"`) that every query filters out.

Tombstones are strongly preferred, because they are the only form of deletion a delta sync can
observe (§6).

---

## 7. Caching and delta sync

### The cache

Key by `uid`. Store the **raw `GraphNode`** (for the envelope) and derive domain objects on read.

```ts
interface NodeCache {
  nodes: Map<string, GraphNode>;   // uid -> raw node, envelope included
  watermarks: Map<string, string>; // sync scope -> highest `m` seen
}
```

A "sync scope" is whatever you delta-sync as a unit — typically one root node's subgraph, keyed by
its `uid`. Persist both maps to IndexedDB if you want cross-reload caching; `localStorage` will not
hold a real working set.

AuthzData is derived from `(master secret, user uid, role)`, so a persisted `ad` stays
signature-valid across logins. It becomes invalid if the server's master secret changes.

### Delta sync on `m`

`m` is set server-side on every upsert *and* on every edge add/remove, and the traversal filters the
*result set* rather than the traversal itself — so one query returns every changed node anywhere in
the subgraph:

```ts
async function syncScope(rootAd: string, rootUid: string, cache: NodeCache) {
  const since = cache.watermarks.get(rootUid);
  const clauses = [{ field: "ty", op: "eq", val: TASK_TY }];
  if (since) clauses.push({ field: "m", op: "gt", val: since });

  const res = await withAuth(() =>
    cogged.query({
      root_ids: [rootAd],
      depth: 5,
      filters: clauses.length > 1 ? { and: clauses } : clauses[0],
      select: [...TASK_LIST_SELECT, "e"],
    }),
  );

  const changed = res.result_nodes ?? [];
  for (const n of changed) {
    if (n.uid) cache.nodes.set(n.uid, { ...cache.nodes.get(n.uid), ...n });
  }

  // Advance the watermark to the highest `m` actually received — immune to clock skew,
  // and never skips a node. Bootstrap from the server's `timestamp` on an empty first sync.
  const maxM = changed.reduce<string | undefined>(
    (a, n) => (n.m && (!a || n.m > a) ? n.m : a),
    since,
  );
  cache.watermarks.set(rootUid, maxM ?? res.timestamp ?? new Date().toISOString());
  return changed.length;
}
```

Rules that make this correct:

- **Pass `m` values back verbatim.** They arrive as RFC3339 with nanosecond precision
  (`2021-03-14T05:18:32.8247882Z`). Never reformat or round-trip through `Date` — you will lose
  precision and re-fetch or skip nodes. String comparison on RFC3339-UTC is a valid ordering, which
  is why `n.m > a` above is safe.
- **Take the watermark from data, not from a clock.** `max(m)` over received nodes cannot skip a
  write that landed between the query snapshot and your response handling; a wall-clock watermark
  can. Only bootstrap from `res.timestamp` (the server's clock, not the browser's).
- **Merge, never replace.** `{ ...existing, ...incoming }` preserves fields the current `select`
  didn't request.
- **Deletions are invisible to `m > since`.** Nothing is ever removed server-side, and an unlinked
  child is not "changed" in a way you'll see if you're only reading changed *nodes*. Include `e` in
  the `select` for container nodes and reconcile children against the parent's edge list whenever
  the parent's `m` advances. Or use tombstones, which show up as ordinary changes.
- `m` is indexed at **hour** granularity, so a delta query may scan up to an extra hour of nodes.
  Correctness is unaffected; don't expect the index to narrow below an hour.
- A node whose only change was an edge add/remove will come back with `m` bumped and otherwise
  identical — that is the signal to re-read its `e` list.

### Priming the session

Shared nodes are unreachable until the server's in-memory SGI allowlist knows about them. Call this
after every login *and* after any 401-refresh, and treat it as a prerequisite of any
shared-data view:

```ts
async function primeSession() {
  // Populates the server-side SGI allowlist for this user as a side effect.
  const shared = await cogged.listNodes("shared", { select: ["ty", "id", "s1", "m"] });
  return (shared.result_nodes ?? []);
}
```

Symptom of a missing prime: reads and writes against nodes another user shared with you fail with
400/404 even though the `ad` is valid — the signature verifies but
`state.UsmUserCanAccessSgi` returns false. Retrying `listNodes("shared")` and then the operation is
the correct recovery, and a server restart is the usual cause.

### Cutting request count

- **One deep traversal beats N shallow ones.** `depth` up to 20, and `decodeAny` dispatch on `ty`,
  loads a heterogeneous subgraph in a single call.
- **Batch writes.** `updateNodes` takes an array; `createNodes` creates a whole placeholder-linked
  subgraph at once. Queue edits for a tick and flush together.
- **Cursor-paginate long lists** with `first` + `after` (`after` = the last `uid` of the previous
  page). Cheaper than `offset` for deep paging. Read filtering happens inside the query, so a short
  page means the end of results — not a partially filtered page.
- **Sort server-side or paginate, not both-with-a-twist.** If your sort key isn't `c`/`m`/`t1`/`t2`,
  you must sort client-side, which is only correct once the full set is loaded. Design the sort key
  into `t1`/`t2` if the list can be long.
- **Deduplicate in-flight requests** by scope key, so ten components mounting at once produce one
  query.
- Sync on visibility change and user action rather than on a short interval; there is no
  subscription or websocket mechanism.

---

## 8. Sharp edges

- **`p` is owner-private, and enforced on both the read and the query path.**
  - *Reads:* the server strips `p` from every node in a response — including nodes hanging off
    `e`, checked per node — unless the caller owns that node or is a `sys`-role admin
    (`responses.CoggedResponse.AuthzDataPack`). A shared reader who puts `p` in `select` gets the
    node back with `p` **absent, not an error**. Write your codec so a missing `p` is a normal
    case, never a parse failure — the same node will have `p` for its owner and no `p` for
    everyone else.
  - *Queries:* non-admins cannot name `p` in `filters` or `order_by` at all. The whole request is
    rejected with `error: "field 'p' is private and cannot be used in filters or order_by"` before
    it reaches Dgraph. This closes the oracle that stripping alone leaves open — otherwise
    `eq(p, "guess")` would confirm a value by changing which nodes come back, even though the
    value is never returned. It applies to nested `and`/`or` clauses too.

  So `p` is genuinely a private field now, but it is **not a queryable one**: treat it as
  owner-only bookkeeping you fetch alongside a node, never as a search key.
- **`vec` cannot be read back.** It is not in the backend's allowed `select`/filter field list. If
  you need the embedding client-side, store a copy in `b`.
- **A geo search cannot give you "the nearest N".** `near()` matches are returned in uid order and
  distance is neither sortable nor returned, so `first` truncates arbitrarily. See §5a — this is
  the geo equivalent of the `vec` caveat and it catches people out.
- **`has` has no integration-test coverage.** The unit test confirms `has` compiles to
  `regexp(s1,$var)`, but no test runs it against a real Dgraph, and Dgraph's `regexp` normally wants
  a literal pattern. Verify substring search against a live server early rather than building a
  search UI on the assumption.
- **`sgi` is not per-node.** Sharing a node grants read access to *every* node carrying the same
  SGI (subject to `r`). A subgraph created without `reset_sgi` shares one SGI with its parent, so
  the SGI — not the node — is the unit of sharing. Use `reset_sgi: true` when creating a subtree
  that must be shareable independently.
- **`sharedWith` needs `s` permission** on the node, not `r`.
- **A query with no `filters` gets a default `gt(m, epoch)`**, which matches everything with an `m`.
  Harmless, but it means "no filter" is not literally no filter.
- **`depth` is clamped to 20** and `top_k` to 1000, silently.
- **Errors are plain text**, surfaced as `CoggedApiError.status` + message. There are no structured
  error codes; don't pattern-match on message strings beyond the status.
- **Only `sys`-role users may set `root_query`.** An ordinary user's query must have `root_ids`.

---

## 9. Checklist for a new domain type

1. Write down every query the UI needs *before* assigning slots.
2. Assign: substring-searched text → `s1`; ≤2 exact-match facets → `s3`, `s4`; ≤2 range/sort keys →
   `t1`, `t2`; everything else → `b`.
3. If you need a third exact-match facet, split the type across an edge.
4. Set `ty` to the type name; always include it in `select` and in the filter.
5. Generate `id` client-side (ULID/UUID with a type prefix).
6. Write `encode`/`decode` plus `LIST_SELECT` and `DETAIL_SELECT` in one module. `decode` must
   tolerate every field being absent.
7. Decide permission bits at creation time.
8. Repository: `createNodes`/`createUserNode` (mind the `created_nodes` key), `query`,
   `updateNodes` echoing the full envelope, tombstone-delete.
9. Cache raw `GraphNode`s by `uid`; delta-sync with `m > max(m)`; include `e` on containers.
10. Call `listNodes("shared")` at session start.
