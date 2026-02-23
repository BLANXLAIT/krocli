---
name: supermarket
description: Search grocery products, find store locations, add items to cart, and view profile across all Kroger-family stores — Kroger, Ralphs, Fred Meyer, Harris Teeter, King Soopers, Fry's, QFC, Mariano's, Pick 'n Save, Metro Market, and more. Use when user asks about groceries, food shopping, store locations, or wants to manage their grocery cart.
user-invocable: true
read_when:
  - User asks about grocery products or food items
  - User wants to search for products at a supermarket or grocery store
  - User asks about store locations for Kroger, Ralphs, Fred Meyer, Harris Teeter, King Soopers, Fry's, QFC, Mariano's, or Pick 'n Save
  - User wants to add items to their grocery cart
  - User mentions Kroger, Ralphs, Fred Meyer, Harris Teeter, King Soopers, Fry's, grocery shopping, or food prices
triggers:
  - search kroger
  - search ralphs
  - search fred meyer
  - search harris teeter
  - search king soopers
  - search frys
  - find grocery
  - kroger products
  - ralphs products
  - add to cart
  - grocery stores
  - grocery list
  - food prices
  - supermarket
  - kroger locations
  - kroger login
---

# Supermarket Skill

Search grocery products, find stores, add to cart, and view your profile across all Kroger-family stores (Kroger, Ralphs, Fred Meyer, Harris Teeter, King Soopers, Fry's, QFC, Mariano's, Pick 'n Save, and more) — all through the Kroger API via a hosted OAuth proxy. No API keys or developer accounts needed.

## Architecture

All API calls go through a hosted proxy at `https://us-central1-krocli.cloudfunctions.net` which handles OAuth credentials. The agent never needs a client_id or client_secret.

**Two token types:**
- **Client token** — for public data (products, locations). Obtained automatically.
- **User token** — for personal data (cart, profile). Requires one-time browser login.

## Getting a Client Token

Before searching products or locations, obtain a client token:

```bash
curl -s -X POST https://us-central1-krocli.cloudfunctions.net/tokenClient
```

Response:
```json
{"access_token": "eyJ...", "expires_in": 1800, "token_type": "bearer"}
```

Cache the `access_token` for subsequent requests. It expires in 30 minutes.

## Searching Products

```bash
curl -s -H "Authorization: Bearer ACCESS_TOKEN" \
  -H "Accept: application/json" \
  "https://api.kroger.com/v1/products?filter.term=milk&filter.limit=10"
```

**Query parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `filter.term` | Yes | Search term (e.g. "milk", "organic eggs") |
| `filter.locationId` | No | Store ID for local pricing/availability |
| `filter.limit` | No | Max results (default 10, max 50) |

**Response fields to show the user:**
- `data[].productId` — UPC code
- `data[].description` — Product name
- `data[].brand` — Brand name
- `data[].items[].price.regular` — Price (when locationId provided)
- `data[].items[].price.promo` — Sale price (when available)
- `data[].items[].size` — Package size

## Finding Store Locations

```bash
curl -s -H "Authorization: Bearer ACCESS_TOKEN" \
  -H "Accept: application/json" \
  "https://api.kroger.com/v1/locations?filter.zipCode.near=45202&filter.limit=5"
```

**Query parameters:**
| Parameter | Required | Description |
|-----------|----------|-------------|
| `filter.zipCode.near` | Yes | ZIP code to search near |
| `filter.radiusInMiles` | No | Search radius (default 10) |
| `filter.limit` | No | Max results (default 10) |

**Response fields to show the user:**
- `data[].locationId` — Store ID (use for product pricing)
- `data[].name` — Store name
- `data[].address.addressLine1`, `city`, `state`, `zipCode`
- `data[].phone` — Phone number
- `data[].hours` — Operating hours

## User Authentication (for Cart & Profile)

When the user wants to add items to their cart or view their profile, they need to authenticate with Kroger. This is a one-time browser flow.

### Step 1: Generate a session ID and send the login link

Generate a random hex session ID (16-32 characters) and present the login URL to the user as a clickable link:

```
https://us-central1-krocli.cloudfunctions.net/authorize?session_id=SESSION_ID
```

Tell the user: **"Click this link to log in to your Kroger account. Once you see 'Login successful', come back here and let me know."**

### Step 2: Poll for tokens

After the user says they've logged in, poll for their tokens:

```bash
curl -s "https://us-central1-krocli.cloudfunctions.net/tokenUser?session_id=SESSION_ID"
```

- If `{"status": "pending"}` with HTTP 202: user hasn't finished yet. Wait and retry.
- If HTTP 200: tokens are returned. Cache `access_token` and `refresh_token`.

```json
{
  "access_token": "eyJ...",
  "refresh_token": "abc...",
  "expires_in": 1800,
  "token_type": "bearer"
}
```

### Step 3: Use the user token

The user token is needed for cart and profile endpoints.

## Adding to Cart

Requires user token from authentication above.

```bash
curl -s -X PUT \
  -H "Authorization: Bearer USER_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  "https://api.kroger.com/v1/cart/add" \
  -d '{"items": [{"upc": "0011110838049", "quantity": 1}]}'
```

**Request body:**
```json
{
  "items": [
    {"upc": "PRODUCT_ID", "quantity": 1}
  ]
}
```

HTTP 204 means success (no response body).

## Viewing Profile

Requires user token.

```bash
curl -s -H "Authorization: Bearer USER_ACCESS_TOKEN" \
  -H "Accept: application/json" \
  "https://api.kroger.com/v1/identity/profile"
```

## Refreshing an Expired User Token

If a user token returns 401, refresh it:

```bash
curl -s -X POST \
  -H "Content-Type: application/json" \
  "https://us-central1-krocli.cloudfunctions.net/tokenRefresh" \
  -d '{"refresh_token": "REFRESH_TOKEN"}'
```

Response includes new `access_token` and `refresh_token`. Cache both.

## Token Management Summary

| Token | How to get | Expires | Refresh |
|-------|-----------|---------|---------|
| Client | `POST /tokenClient` | 30 min | Just request a new one |
| User | Browser login flow | 30 min | `POST /tokenRefresh` with refresh_token |

## Error Handling

| HTTP Status | Meaning | Action |
|-------------|---------|--------|
| 401 | Token expired | Refresh or re-obtain token |
| 403 | Forbidden | Token lacks required scope |
| 429 | Rate limited | Wait and retry |
| 400 | Bad request | Check parameters |

## Typical Workflows

### "Search for milk near me"
1. Get client token via `POST /tokenClient`
2. Ask user for ZIP code (or use a previously known one)
3. Find nearest store via locations API
4. Search products with `filter.locationId` for local pricing

### "Add bananas to my Kroger cart"
1. Check if user token is cached; if not, start login flow
2. Search for "bananas" to get the UPC
3. Confirm product with user
4. `PUT /cart/add` with the UPC

### "What Kroger stores are near 90210?"
1. Get client token
2. Search locations with `filter.zipCode.near=90210`
3. Format results with name, address, hours
