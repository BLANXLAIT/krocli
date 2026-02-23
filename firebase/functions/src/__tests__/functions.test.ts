/* eslint-disable @typescript-eslint/no-explicit-any */
import { Request, Response } from "express";

// ── Firestore mock ──────────────────────────────────────────────────
const mockDelete = jest.fn().mockResolvedValue(undefined);
const mockUpdate = jest.fn().mockResolvedValue(undefined);
const mockSet = jest.fn().mockResolvedValue(undefined);
const mockGet = jest.fn();
const mockWhere = jest.fn().mockReturnThis();
const mockLimit = jest.fn().mockReturnThis();
const mockDoc = jest.fn();
const mockCollection = jest.fn();

const fakeDb = {
  collection: mockCollection,
};

mockCollection.mockReturnValue({
  where: mockWhere,
  limit: mockLimit,
  get: mockGet,
  doc: mockDoc,
});
mockDoc.mockReturnValue({
  get: mockGet,
  set: mockSet,
  update: mockUpdate,
  delete: mockDelete,
});

jest.mock("firebase-admin/firestore", () => ({
  getFirestore: () => fakeDb,
  FieldValue: { serverTimestamp: () => "SERVER_TS", increment: (n: number) => `INC(${n})` },
  Timestamp: { now: () => ({ toMillis: () => Date.now() }) },
}));

jest.mock("firebase-admin/app", () => ({
  initializeApp: jest.fn(),
}));

// Stub firebase-functions so onRequest just returns the handler
jest.mock("firebase-functions/v2/https", () => ({
  onRequest: (_opts: any, handler: any) => handler,
}));

jest.mock("firebase-functions/params", () => ({
  defineSecret: (name: string) => ({ value: () => `mock-${name}` }),
}));

// ── Helpers ─────────────────────────────────────────────────────────
function makeReq(overrides: Partial<Request> = {}): Request {
  return {
    method: "GET",
    query: {},
    body: {},
    ip: "127.0.0.1",
    ...overrides,
  } as unknown as Request;
}

function makeRes(): Response & { _status: number; _json: any; _body: any; _redirect: string } {
  const res: any = {
    _status: 200,
    _json: null,
    _body: null,
    _redirect: "",
  };
  res.status = jest.fn((code: number) => { res._status = code; return res; });
  res.json = jest.fn((data: any) => { res._json = data; return res; });
  res.send = jest.fn((data: any) => { res._body = data; return res; });
  res.redirect = jest.fn((url: string) => { res._redirect = url; return res; });
  return res;
}

// ── Reset mocks between tests ───────────────────────────────────────
beforeEach(() => {
  jest.clearAllMocks();
  mockCollection.mockReturnValue({
    where: mockWhere,
    limit: mockLimit,
    get: mockGet,
    doc: mockDoc,
  });
  mockDoc.mockReturnValue({
    get: mockGet,
    set: mockSet,
    update: mockUpdate,
    delete: mockDelete,
  });
  mockWhere.mockReturnThis();
  mockLimit.mockReturnThis();
});

// ── tokenUser ───────────────────────────────────────────────────────
describe("tokenUser", () => {
  let handler: any;
  beforeAll(() => {
    handler = require("../tokenUser").tokenUser;
  });

  it("returns tokens and deletes session doc afterward", async () => {
    const docRef = { delete: mockDelete };
    mockGet.mockResolvedValueOnce({
      empty: false,
      docs: [{
        data: () => ({
          access_token: "at_123",
          refresh_token: "rt_456",
          expires_in: 1800,
          token_type: "Bearer",
        }),
        ref: docRef,
      }],
    });

    const req = makeReq({ method: "GET", query: { session_id: "abc123" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.json).toHaveBeenCalledWith({
      access_token: "at_123",
      refresh_token: "rt_456",
      expires_in: 1800,
      token_type: "Bearer",
    });
    expect(docRef.delete).toHaveBeenCalled();
  });

  it("returns 202 pending when session not yet completed", async () => {
    mockGet.mockResolvedValueOnce({ empty: true });

    const req = makeReq({ method: "GET", query: { session_id: "abc123" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(202);
    expect(res._json).toEqual({ status: "pending" });
  });

  it("rejects non-GET methods", async () => {
    const req = makeReq({ method: "POST" });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(405);
  });

  it("requires session_id parameter", async () => {
    const req = makeReq({ method: "GET", query: {} as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(400);
  });
});

// ── callback ────────────────────────────────────────────────────────
describe("callback", () => {
  let handler: any;
  beforeAll(() => {
    handler = require("../callback").callback;
  });

  it("rejects sessions older than 5 minutes", async () => {
    const sixMinutesAgo = Date.now() - 6 * 60 * 1000;
    mockGet.mockResolvedValueOnce({
      exists: true,
      data: () => ({
        created_at: { toMillis: () => sixMinutesAgo },
      }),
    });

    const req = makeReq({ query: { code: "authcode", state: "statevalue" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(400);
    expect(res._body).toContain("expired");
    expect(mockDelete).toHaveBeenCalled();
  });

  it("exchanges auth code for tokens and marks session completed", async () => {
    const recentTime = Date.now() - 30 * 1000; // 30 seconds ago
    mockGet.mockResolvedValueOnce({
      exists: true,
      data: () => ({
        created_at: { toMillis: () => recentTime },
        source: "cli",
      }),
    });

    // Mock the Kroger API fetch
    const originalFetch = global.fetch;
    global.fetch = jest.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        access_token: "new_at",
        refresh_token: "new_rt",
        expires_in: 1800,
        token_type: "Bearer",
      }),
    }) as any;

    const req = makeReq({ query: { code: "authcode123", state: "stateabc" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(mockUpdate).toHaveBeenCalledWith(expect.objectContaining({
      access_token: "new_at",
      refresh_token: "new_rt",
      completed: true,
    }));
    expect(res.status).toHaveBeenCalledWith(200);

    global.fetch = originalFetch;
  });

  it("renders success page with trust info", async () => {
    const recentTime = Date.now() - 30 * 1000;
    mockGet.mockResolvedValueOnce({
      exists: true,
      data: () => ({
        created_at: { toMillis: () => recentTime },
        source: "agent",
      }),
    });

    const originalFetch = global.fetch;
    global.fetch = jest.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        access_token: "at", refresh_token: "rt", expires_in: 1800, token_type: "Bearer",
      }),
    }) as any;

    const req = makeReq({ query: { code: "code", state: "state" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(res._body).toContain("Login Successful");
    expect(res._body).toContain("data is secure");
    expect(res._body).toContain("open source");
    expect(res._body).toContain("Go back to your conversation");

    global.fetch = originalFetch;
  });

  it("rejects non-GET methods", async () => {
    const req = makeReq({ method: "POST" });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(405);
  });

  it("requires code and state parameters", async () => {
    const req = makeReq({ query: {} as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(400);
  });
});

// ── authorize ───────────────────────────────────────────────────────
describe("authorize", () => {
  let handler: any;
  beforeAll(() => {
    handler = require("../authorize").authorize;
  });

  it("rejects invalid session_id format", async () => {
    // Too short
    const req1 = makeReq({ query: { session_id: "abc" } as any });
    const res1 = makeRes();
    await handler(req1, res1);
    expect(res1.status).toHaveBeenCalledWith(400);
    expect(res1._json.error).toContain("hex string");

    // Non-hex chars
    const req2 = makeReq({ query: { session_id: "zzzzzzzzzzzzzzzz" } as any });
    const res2 = makeRes();
    await handler(req2, res2);
    expect(res2.status).toHaveBeenCalledWith(400);
  });

  it("accepts valid hex session_id and redirects to Kroger", async () => {
    // Mock rate limit to allow
    const rateLimitMod = require("../rateLimit");
    jest.spyOn(rateLimitMod, "checkRateLimit").mockResolvedValueOnce(true);

    const validHex = "a".repeat(32);
    const req = makeReq({ query: { session_id: validHex } as any });
    const res = makeRes();
    await handler(req, res);

    expect(res.redirect).toHaveBeenCalled();
    expect(res._redirect).toContain("api.kroger.com");
    expect(mockSet).toHaveBeenCalledWith(expect.objectContaining({
      source: "unknown",
    }));
  });

  it("stores source param in session doc", async () => {
    const rateLimitMod = require("../rateLimit");
    jest.spyOn(rateLimitMod, "checkRateLimit").mockResolvedValueOnce(true);

    const validHex = "b".repeat(32);
    const req = makeReq({ query: { session_id: validHex, source: "cli" } as any });
    const res = makeRes();
    await handler(req, res);

    expect(mockSet).toHaveBeenCalledWith(expect.objectContaining({
      source: "cli",
    }));
  });

  it("rejects non-GET methods", async () => {
    const req = makeReq({ method: "POST" });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(405);
  });
});

// ── tokenClient ─────────────────────────────────────────────────────
describe("tokenClient", () => {
  let handler: any;
  beforeAll(() => {
    handler = require("../tokenClient").tokenClient;
  });

  it("rejects non-POST methods", async () => {
    const req = makeReq({ method: "GET" });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(405);
  });

  it("returns client token on POST", async () => {
    const rateLimitMod = require("../rateLimit");
    jest.spyOn(rateLimitMod, "checkRateLimit").mockResolvedValueOnce(true);

    const originalFetch = global.fetch;
    global.fetch = jest.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        access_token: "client_at",
        expires_in: 1800,
        token_type: "Bearer",
      }),
    }) as any;

    const req = makeReq({ method: "POST" });
    const res = makeRes();
    await handler(req, res);

    expect(res.json).toHaveBeenCalledWith({
      access_token: "client_at",
      expires_in: 1800,
      token_type: "Bearer",
    });

    global.fetch = originalFetch;
  });
});

// ── tokenRefresh ────────────────────────────────────────────────────
describe("tokenRefresh", () => {
  let handler: any;
  beforeAll(() => {
    handler = require("../tokenRefresh").tokenRefresh;
  });

  it("requires refresh_token in body", async () => {
    const rateLimitMod = require("../rateLimit");
    jest.spyOn(rateLimitMod, "checkRateLimit").mockResolvedValueOnce(true);

    const req = makeReq({ method: "POST", body: {} });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(400);
    expect(res._json.error).toContain("refresh_token");
  });

  it("rejects non-POST methods", async () => {
    const req = makeReq({ method: "GET" });
    const res = makeRes();
    await handler(req, res);

    expect(res.status).toHaveBeenCalledWith(405);
  });
});

// ── rateLimit ───────────────────────────────────────────────────────
describe("checkRateLimit", () => {
  // Use a fresh import that bypasses the spy
  const { checkRateLimit } = jest.requireActual("../rateLimit") as any;

  it("rejects after max requests in window", async () => {
    // First request: doc doesn't exist → creates it
    mockGet.mockResolvedValueOnce({ exists: false });
    const result1 = await checkRateLimit("1.2.3.4", "test", 1, 60);
    expect(result1).toBe(true);

    // Second request: doc exists, count at max
    const recentStart = Date.now() - 1000;
    mockGet.mockResolvedValueOnce({
      exists: true,
      data: () => ({ count: 1, windowStart: { toMillis: () => recentStart } }),
    });
    const result2 = await checkRateLimit("1.2.3.4", "test", 1, 60);
    expect(result2).toBe(false);
  });

  it("resets after window expires", async () => {
    const oldStart = Date.now() - 61 * 60 * 1000; // well past 60-minute window
    mockGet.mockResolvedValueOnce({
      exists: true,
      data: () => ({ count: 99, windowStart: { toMillis: () => oldStart } }),
    });

    const result = await checkRateLimit("1.2.3.4", "test", 1, 60);
    expect(result).toBe(true);
    expect(mockSet).toHaveBeenCalledWith(expect.objectContaining({ count: 1 }));
  });
});
