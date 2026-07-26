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
// created_nodes is keyed by the placeholder uid ("$inbox") you supplied.
const created = await cogged.createUserNode({
  node: { uid: "$inbox", ty: "inbox", s1: "Alice's inbox", r: true },
});
const inboxAd = created.created_nodes?.["$inbox"]?.ad; // opaque AuthzData token

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

### Keeping in sync with the spec

Because this package lives in the Cogged monorepo, `npm run gen` reads
`../../openapi3.yaml` directly — there is no spec to fetch or version to pin. A change to
an endpoint updates the handler, the spec (enforced by the Go drift guard in `spec/`), and
the client types (enforced by `check:spec` in CI) in a single PR. When you change the API:

1. update the handler and `openapi3.yaml` (the Go drift test enforces they match),
2. run `npm run gen` here and commit the updated `src/generated/types.ts`,
3. adjust the hand-written client in `src/client.ts` if a new endpoint was added.
