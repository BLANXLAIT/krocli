import { onRequest } from "firebase-functions/v2/https";
import { defineSecret } from "firebase-functions/params";
import { getFirestore, FieldValue } from "firebase-admin/firestore";
import * as crypto from "crypto";
import { checkRateLimit } from "./rateLimit";

const krogerClientId = defineSecret("KROGER_CLIENT_ID");

const KROGER_AUTHORIZE_URL = "https://api.kroger.com/v1/connect/oauth2/authorize";
const CALLBACK_URL = "https://us-central1-krocli.cloudfunctions.net/callback";
const VALID_SCOPES = new Set([
  "product.compact",
  "cart.basic:write",
  "profile.compact",
  "coupon.basic",
]);

function validateScope(input: string): string {
  const requested = input.split(/\s+/);
  const valid = requested.filter((s) => VALID_SCOPES.has(s));
  return valid.length > 0 ? valid.join(" ") : "cart.basic:write profile.compact";
}

export const authorize = onRequest(
  {
    cors: true,
    region: "us-central1",
    secrets: [krogerClientId],
  },
  async (req, res) => {
    if (req.method !== "GET") {
      res.status(405).json({ error: "Method not allowed" });
      return;
    }

    const sessionId = req.query.session_id as string | undefined;
    if (!sessionId) {
      res.status(400).json({ error: "session_id is required" });
      return;
    }

    if (!/^[0-9a-f]{16,64}$/.test(sessionId)) {
      res.status(400).json({ error: "session_id must be a hex string 16-64 chars" });
      return;
    }

    const ip = req.ip || "unknown";
    const allowed = await checkRateLimit(ip, "authorize", 5, 60);
    if (!allowed) {
      res.status(429).json({ error: "Rate limit exceeded" });
      return;
    }

    const scope = validateScope((req.query.scope as string) || "cart.basic:write profile.compact");
    const source = (req.query.source as string) || "unknown";
    const state = crypto.randomBytes(16).toString("hex");

    const db = getFirestore();
    await db.collection("sessions").doc(state).set({
      session_id: sessionId,
      state,
      source,
      created_at: FieldValue.serverTimestamp(),
    });

    const clientId = krogerClientId.value();
    const redirectUrl = `${KROGER_AUTHORIZE_URL}?` + new URLSearchParams({
      client_id: clientId,
      redirect_uri: CALLBACK_URL,
      response_type: "code",
      scope,
      state,
    }).toString();

    // nosemgrep: javascript.express.web.tainted-redirect-express.tainted-redirect-express
    res.redirect(redirectUrl);
  }
);
