import type {
  AuthzData,
  ClientConfig,
  CoggedResponseCN,
  CoggedResponseCU,
  CoggedResponseEmpty,
  CoggedResponseRN,
  CoggedResponseRU,
  CreateNodesRequest,
  CreateUserRequest,
  EdgesRequest,
  LoginRequest,
  NodeScope,
  QueryRequest,
  ShareNodesRequest,
  TokenResponse,
  UpdateNodesRequest,
  UserNodeRequest,
  UserResponse,
  UsersRequest,
} from "./types.js";

export interface CoggedClientOptions {
  /** Base URL of the Cogged server, e.g. "http://localhost:8090". */
  baseUrl: string;
  /** Optional bearer token to start authenticated (otherwise call login()). */
  token?: string;
  /** Override the fetch implementation (e.g. for tests or non-global environments). */
  fetch?: typeof fetch;
}

/** Error thrown for non-2xx responses; carries the HTTP status and server message. */
export class CoggedApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "CoggedApiError";
  }
}

type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

/**
 * A thin, typed client for the Cogged API. Types come from the backend's
 * openapi3.yaml (see src/generated/types.ts); this class adds the auth/token flow and
 * ergonomic methods. AuthzData tokens are opaque — read them from responses and pass
 * them back unchanged.
 */
export class CoggedClient {
  private readonly baseUrl: string;
  private readonly fetchImpl: typeof fetch;
  private token: string | undefined;

  constructor(opts: CoggedClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.token = opts.token;
    this.fetchImpl = opts.fetch ?? globalThis.fetch;
    if (typeof this.fetchImpl !== "function") {
      throw new Error("no fetch implementation available; pass one via options.fetch");
    }
  }

  /** Set (or clear, with undefined) the bearer token used for authenticated requests. */
  setToken(token: string | undefined): void {
    this.token = token;
  }

  getToken(): string | undefined {
    return this.token;
  }

  private async request<T>(method: HttpMethod, path: string, body?: unknown): Promise<T> {
    // The server requires Content-Type: application/json on every request (even GETs).
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    const res = await this.fetchImpl(this.baseUrl + path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) {
      throw new CoggedApiError(res.status, text.trim() || res.statusText);
    }
    return (text ? JSON.parse(text) : {}) as T;
  }

  // --- auth ---

  /** Log in and store the returned bearer token for subsequent requests. */
  async login(username: string, password: string): Promise<TokenResponse> {
    const req: LoginRequest = { username, password };
    const res = await this.request<TokenResponse>("POST", "/auth/login", req);
    if (res.token) {
      this.token = res.token;
    }
    return res;
  }

  /** Invalidate the current token server-side and clear it locally. */
  async logout(): Promise<void> {
    await this.request<CoggedResponseEmpty>("POST", "/auth/logout");
    this.token = undefined;
  }

  /** Verify the current token is valid (throws CoggedApiError if not). */
  async check(): Promise<void> {
    await this.request<CoggedResponseEmpty>("GET", "/auth/check");
  }

  /** Renew the current token and store the new one. */
  async refresh(): Promise<TokenResponse> {
    const res = await this.request<TokenResponse>("GET", "/auth/refresh");
    if (res.token) {
      this.token = res.token;
    }
    return res;
  }

  /** Fetch the application-specific client configuration string (unauthenticated). */
  clientConfig(): Promise<ClientConfig> {
    return this.request<ClientConfig>("GET", "/auth/clientconfig");
  }

  // --- admin (superuser role required) ---

  createUser(req: CreateUserRequest): Promise<CoggedResponseCU> {
    return this.request<CoggedResponseCU>("PUT", "/admin/user", req);
  }

  updateUsers(req: UsersRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PATCH", "/admin/users", req);
  }

  // --- graph ---

  /** Query nodes by traversing node→node edges from the given root ids. */
  query(req: QueryRequest): Promise<CoggedResponseRN> {
    return this.request<CoggedResponseRN>("POST", "/graph/nodes", req);
  }

  /** List the users a node has been shared with (requires 's' permission on the node). */
  sharedWith(node: AuthzData): Promise<CoggedResponseRU> {
    return this.request<CoggedResponseRU>("GET", `/graph/sharedwith/${encodeURIComponent(node)}`);
  }

  /** Bulk-update predicates of existing nodes (each node echoes its AuthzData). */
  updateNodes(req: UpdateNodesRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PATCH", "/graph/nodes", req);
  }

  /** Create new nodes as a subgraph linked from the given parent node. */
  createNodes(parent: AuthzData, req: CreateNodesRequest): Promise<CoggedResponseCN> {
    return this.request<CoggedResponseCN>("PUT", `/graph/nodes/${encodeURIComponent(parent)}`, req);
  }

  addEdges(req: EdgesRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PUT", "/graph/edges", req);
  }

  removeEdges(req: EdgesRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PATCH", "/graph/edges", req);
  }

  // --- user ---

  /** Create a node owned by, and linked to, the requesting user. */
  createUserNode(req: UserNodeRequest): Promise<CoggedResponseCN> {
    return this.request<CoggedResponseCN>("PUT", "/user/node", req);
  }

  /** List the requesting user's own nodes ("own") or nodes shared with them ("shared"). */
  listNodes(scope: NodeScope, req: QueryRequest = {}): Promise<CoggedResponseRN> {
    return this.request<CoggedResponseRN>("POST", `/user/nodes/${scope}`, req);
  }

  /** Share node(s) with other user(s). */
  share(req: ShareNodesRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PUT", "/user/share", req);
  }

  /** Un-share node(s) from other user(s). */
  unshare(req: ShareNodesRequest): Promise<CoggedResponseEmpty> {
    return this.request<CoggedResponseEmpty>("PATCH", "/user/share", req);
  }

  /** Look up a user by their dgraph uid (0xNN). */
  getUserByUid(uid: string): Promise<UserResponse> {
    return this.request<UserResponse>("GET", `/user/uid/${encodeURIComponent(uid)}`);
  }

  /** Look up a user by username. */
  getUserByName(username: string): Promise<UserResponse> {
    return this.request<UserResponse>("GET", `/user/name/${encodeURIComponent(username)}`);
  }

  // --- health ---

  health(): Promise<{ status?: string }> {
    return this.request<{ status?: string }>("GET", "/health/status");
  }
}
