import { onRequest } from "firebase-functions/v2/https";
import { defineSecret } from "firebase-functions/params";
import { getFirestore, Timestamp } from "firebase-admin/firestore";

const krogerClientId = defineSecret("KROGER_CLIENT_ID");
const krogerClientSecret = defineSecret("KROGER_CLIENT_SECRET");

const KROGER_TOKEN_URL = "https://api.kroger.com/v1/connect/oauth2/token";
const CALLBACK_URL = "https://us-central1-krocli.cloudfunctions.net/callback";
const SESSION_TTL_MS = 5 * 60 * 1000; // 5 minutes

function errorPage(title: string, message: string): string {
  return `<!DOCTYPE html>
<html><head><title>${title}</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
</head>
<body style="font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f5f5f5">
  <div style="text-align:center;background:white;padding:2rem 3rem;border-radius:12px;box-shadow:0 2px 8px rgba(0,0,0,0.1);max-width:480px">
    <h1 style="color:#dc2626">${title}</h1>
    <p style="color:#666">${message}</p>
  </div>
</body></html>`;
}

function successPage(source: string): string {
  const cliExamples = `
    <div style="text-align:left;background:#1e1e1e;color:#d4d4d4;padding:1rem 1.25rem;border-radius:8px;font-family:'SF Mono',Monaco,Consolas,monospace;font-size:0.85rem;line-height:1.6;overflow-x:auto">
      <div><span style="color:#6a9955">$</span> krocli products search --term <span style="color:#ce9178">"milk"</span></div>
      <div><span style="color:#6a9955">$</span> krocli cart add --upc 0011110838049 --qty 2</div>
      <div><span style="color:#6a9955">$</span> krocli identity profile</div>
    </div>`;

  const agentExamples = `
    <div style="text-align:left;background:#f0f4ff;padding:1rem 1.25rem;border-radius:8px;font-size:0.9rem;line-height:1.8;color:#333">
      <div>&ldquo;Search for organic milk at Ralphs&rdquo;</div>
      <div>&ldquo;Add eggs and bread to my Kroger cart&rdquo;</div>
      <div>&ldquo;Show my Kroger profile&rdquo;</div>
    </div>`;

  let examples: string;
  let subtitle: string;
  if (source === "cli") {
    examples = cliExamples;
    subtitle = "Return to your terminal — you're all set.";
  } else if (source === "agent") {
    examples = agentExamples;
    subtitle = "Go back to your conversation — you're all set.";
  } else {
    examples = cliExamples + `\n    <p style="margin:0.75rem 0 0.5rem;font-size:0.85rem;color:#888">Or ask your AI agent:</p>\n` + agentExamples;
    subtitle = "You can close this tab now.";
  }

  return `<!DOCTYPE html>
<html><head><title>Login Successful - krocli</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
</head>
<body style="font-family:system-ui,-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:linear-gradient(135deg,#f5f7fa 0%,#c3cfe2 100%)">
  <div style="background:white;padding:2.5rem;border-radius:16px;box-shadow:0 4px 24px rgba(0,0,0,0.1);max-width:520px;width:90%">
    <div style="text-align:center;margin-bottom:1.5rem">
      <div style="font-size:3rem;margin-bottom:0.5rem">&#10003;</div>
      <h1 style="margin:0 0 0.25rem;font-size:1.5rem;color:#111">Login Successful</h1>
      <p style="margin:0;color:#666;font-size:0.95rem">${subtitle}</p>
    </div>

    <div style="background:#f0fdf4;border:1px solid #bbf7d0;border-radius:10px;padding:1rem 1.25rem;margin-bottom:1.5rem">
      <h2 style="margin:0 0 0.5rem;font-size:0.9rem;color:#15803d;font-weight:600">Your data is secure</h2>
      <ul style="margin:0;padding:0 0 0 1.1rem;font-size:0.85rem;color:#333;line-height:1.7">
        <li>Session deleted from server after token delivery</li>
        <li>No credentials or passwords stored on proxy</li>
        <li>Login sessions expire after 5 minutes</li>
        <li>Proxy is <a href="https://github.com/BLANXLAIT/krocli/tree/main/firebase/functions/src" style="color:#15803d">fully open source</a></li>
      </ul>
    </div>

    <div style="margin-bottom:1.5rem">
      <h2 style="margin:0 0 0.75rem;font-size:0.9rem;color:#333;font-weight:600">Try it out</h2>
      ${examples}
    </div>

    <div style="text-align:center;padding-top:0.5rem;border-top:1px solid #eee">
      <a href="https://github.com/BLANXLAIT/krocli" style="color:#888;font-size:0.8rem;text-decoration:none">github.com/BLANXLAIT/krocli</a>
    </div>
  </div>
</body></html>`;
}

export const callback = onRequest(
  {
    cors: true,
    region: "us-central1",
    secrets: [krogerClientId, krogerClientSecret],
  },
  async (req, res) => {
    if (req.method !== "GET") {
      res.status(405).send(errorPage("Error", "Method not allowed"));
      return;
    }

    const code = req.query.code as string | undefined;
    const state = req.query.state as string | undefined;

    if (!code || !state) {
      res.status(400).send(errorPage("Error", "Missing code or state parameter."));
      return;
    }

    const db = getFirestore();
    const sessionRef = db.collection("sessions").doc(state);
    const sessionDoc = await sessionRef.get();

    if (!sessionDoc.exists) {
      res.status(400).send(errorPage("Error", "Invalid or expired session."));
      return;
    }

    const session = sessionDoc.data()!;
    const source = (session.source as string) || "unknown";
    const createdAt = session.created_at as Timestamp;
    const age = Date.now() - createdAt.toMillis();
    if (age > SESSION_TTL_MS) {
      await sessionRef.delete();
      res.status(400).send(errorPage("Session Expired", "Your login session has expired. Please try again."));
      return;
    }

    const clientId = krogerClientId.value();
    const clientSecret = krogerClientSecret.value();
    const basicAuth = Buffer.from(`${clientId}:${clientSecret}`).toString("base64");

    try {
      const response = await fetch(KROGER_TOKEN_URL, {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
          Authorization: `Basic ${basicAuth}`,
        },
        body: new URLSearchParams({
          grant_type: "authorization_code",
          code,
          redirect_uri: CALLBACK_URL,
        }),
      });

      if (!response.ok) {
        const text = await response.text();
        console.error("Kroger token exchange failed:", text);
        res.status(502).send(errorPage("Error", "Failed to complete login with Kroger. Please try again."));
        return;
      }

      const data = await response.json();

      await sessionRef.update({
        access_token: data.access_token,
        refresh_token: data.refresh_token,
        expires_in: data.expires_in,
        token_type: data.token_type,
        completed: true,
      });

      res.status(200).send(successPage(source));
    } catch (err) {
      console.error("callback error:", err);
      res.status(500).send(errorPage("Error", "An unexpected error occurred. Please try again."));
    }
  }
);
