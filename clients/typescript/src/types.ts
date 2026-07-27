import type { components } from "./generated/types.js";

type Schemas = components["schemas"];

/**
 * AuthzData is an opaque, server-signed token that identifies a node or user and
 * encodes its owner/permissions. Treat it as a black box: read it from a response
 * (the `ad` field) and pass it back verbatim in later requests. Never construct or
 * mutate one — the server verifies the embedded signature.
 */
export type AuthzData = string;

// --- request DTOs ---
export type LoginRequest = Schemas["LoginRequest"];
export type CreateUserRequest = Schemas["CreateUserRequest"];
export type UsersRequest = Schemas["UsersRequest"];
export type QueryRequest = Schemas["QueryRequest"];
export type QueryRequestClause = Schemas["QueryRequestClause"];
export type UpdateNodesRequest = Schemas["UpdateNodesRequest"];
export type CreateNodesRequest = Schemas["CreateNodesRequest"];
export type EdgesRequest = Schemas["EdgesRequest"];
export type UserNodeRequest = Schemas["UserNodeRequest"];
export type ShareNodesRequest = Schemas["ShareNodesRequest"];

// --- response DTOs ---
export type TokenResponse = Schemas["TokenResponse"];
export type UserResponse = Schemas["UserResponse"];
export type ClientConfig = Schemas["ClientConfig"];
export type CoggedResponseRN = Schemas["CoggedResponseRN"];
export type CoggedResponseRU = Schemas["CoggedResponseRU"];
export type CoggedResponseEmpty = Schemas["CoggedResponseEmpty"];

/** A created node as returned in created_nodes (uid, owner, permissions, AuthzData). */
export type NodeEdgeData = Schemas["NodeEdgeData"];

/**
 * Response from the node-create endpoints. The keys of created_nodes depend on which
 * endpoint you called: createNodes (PUT /graph/nodes/{parent}) echoes back the
 * $placeholder uids you supplied, while createUserNode (PUT /user/node) always uses the
 * single literal key "new" — the server substitutes its own placeholder for the one
 * node it creates, so the placeholder you sent is not echoed back.
 */
export type CoggedResponseCN = Schemas["CoggedResponseCN"];

/**
 * Response from PUT /admin/user: created_uids maps the server-assigned placeholder to
 * the new user's uid. That key is always the literal "newuser" for this endpoint.
 */
export type CoggedResponseCU = Schemas["CoggedResponseCU"];

// --- model types ---
export type GraphNode = Schemas["GraphNode"];
export type GraphNodeNew = Schemas["GraphNodeNew"];
export type GraphUser = Schemas["GraphUserDTO"];

/** The scope for POST /user/nodes/{scope}. */
export type NodeScope = "own" | "shared";

// Re-export the raw generated paths/components for advanced use.
export type { components, paths } from "./generated/types.js";
