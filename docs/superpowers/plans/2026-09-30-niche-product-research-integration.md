# Niche Web → product_research `/runs` Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewire the "Nghiên cứu ngách" flow in `web/app.jsx` / `web/niche.jsx` so it drives the real `levelup_ai` `POST /api/v1/product-research/runs` pipeline instead of the two thin `niche-research-service:8084` calls it uses today, without touching `web/niche.jsx` itself.

**Architecture:** `niche.jsx` never changes — it only calls `api(method, path)`. We keep every `addRoute` method+pattern signature in `web/app.jsx` identical and swap what happens *inside* 11 of them: `POST/GET/DELETE /api/niche/sessions[/:id]` and `POST .../run` now talk to `levelup_ai` (queue a job, poll it, cache its `ui` block), and the 6 read-only detail routes (`products`, `sources`, `products/:id/members`, `pricing`, `summary`, `profile`) now serve slices of that cached `ui` block instead of `NICHE_PRODUCTS`/`NICHE_KEYWORDS` mock stores. A new `web/product-research-client.js` module holds the HTTP calls, status mapping, and the client-side session registry (levelup_ai's job store has no "list all jobs" endpoint, so the SPA must remember which job ids it created). Everything else in `web/app.jsx` (`meta`, `targeting*`, `audience/estimate`, `step2/run`, `finance`, `shortlist`, `step3/run`, `suppliers/*`, `step4/run`, `sourcing`, `summarize`, `benchmark/*`) stays exactly as-is — those correspond to product-research features that don't exist server-side yet.

**Tech Stack:** Plain browser JS/JSX (Babel-standalone, no bundler, no npm/test runner in `web/`) served as static files; verification uses the `Claude_Preview` MCP tools (`preview_start` over a static file server, `preview_eval`, `preview_network`) in place of a unit-test runner, since none exists for this directory.

**Spec:** This conversation's reverse-engineering of `levelup_ai/app/features/product_research/{router.py,report_schemas.py,ui_schemas.py,ui_view.py}`, `levelup_ai/app/jobs/store.py`, `levelup_ai/app/transform.py`, and `crawl-ecomerce-golang/web/{app.jsx,niche.jsx}`. No separate written spec doc exists; the "Global Constraints" below are the extracted contract.

## Global Constraints

- Request to create a run: `POST {PR_API_BASE}/api/v1/product-research/runs`, camelCase JSON body `{jobId, keyword, countryCodes}`, 202 response envelope `{"status":202,"data":{"job_id":"...","status":"QUEUED"}}` (`levelup_ai/app/features/product_research/router.py:292-314`, `levelup_ai/app/features/product_research/schemas.py:318-323`).
- Poll: `GET {PR_API_BASE}/api/v1/ai/jobs/{job_id}`, envelope `{"status":200,"data":{"job_id","type","status","result","error"}}` (`levelup_ai/app/features/jobs_router.py:16-31`, `levelup_ai/app/transform.py:41-45` — the outer `status` is the **HTTP status code**, not a string).
- Job `status` values are exactly `QUEUED | RUNNING | SUCCEEDED | FAILED | CANCELLED` (`levelup_ai/app/jobs/store.py:12-16`). Map to the `niche.jsx` session status vocabulary `draft | running | done | error`: `QUEUED,RUNNING → "running"`; `SUCCEEDED → "done"`; `FAILED,CANCELLED → "error"`.
- On `SUCCEEDED`, `data.result` is a `ResearchReport` (camelCase; `levelup_ai/app/features/product_research/report_schemas.py:305-314`). Its `result.ui` field is a `UiBlock` — **snake_case**, shaped to match `niche.jsx` field-for-field, with exactly 7 keys: `session, profile, products, sources, members, pricing, summary` (`levelup_ai/app/features/product_research/ui_schemas.py:346-352`). `members` is `dict[str(product_id) -> {ads, ecom}]`.
- `ui.session.step2_status` and `ui.session.step3_status` are always `"done"` (or `"error"` for step2 if the job's step2 data is unusable) the moment the job is `SUCCEEDED` — the whole pipeline runs in one pass, there is no separate step2/step3 trigger server-side (`levelup_ai/app/features/product_research/ui_view.py:90,93`).
- Default `PR_API_BASE = "http://localhost:3009"` (levelup_ai's `AI_PORT` default, `levelup_ai/app/settings.py:23`) — a single `const` at the top of the new client file, same pattern as the existing `NICHE_API_BASE` constant (`web/app.jsx:1133`).
- Reuse the existing `uid(prefix)` helper (`web/app.jsx:16`) to generate `jobId` client-side — do not add a UUID library.
- Every new/changed `addRoute` handler must keep its existing `(method, pattern)` signature exactly as registered today, so `web/niche.jsx` requires zero changes.

## Review Focus

- **Re-run button corrupts real data.** `niche.jsx:2863` calls `POST /api/niche/sessions/:id/run` on the "Chạy lại phân tích" button. Today's mock handler (`nicheRunStep1`, `web/app.jsx:1191-1194`) overwrites real session fields with fabricated demo data — this is the exact bug this integration must not reproduce. The new `/run` handler must be a pure no-op that returns the current known session state unchanged (Task 4).
- **Polling while the job is still queued/running.** `niche.jsx`'s `useEffect` polls `GET /api/niche/sessions/:id` every 1200ms (`web/niche.jsx:358-367`) until `status !== "running"`. The handler must call `prPollJob`, not `prStartRun`, on every GET, and must not throw while `QUEUED`/`RUNNING` — it must return a valid session-shell object with `status:"running"` so `StepProgress` renders instead of crashing (Task 4).
- **Job FAILED.** `niche.jsx:394-396` renders a "Quay lại" button when `session.status === "error"`; nothing else reads `session.message`/`error` in the failure branch shown, but the handler must still populate a `message` string from `data.error` so future consumers don't get `undefined` (Task 4).
- **Unknown/uncached product id in `members`.** `GET /api/niche/products/:id/members` (`web/app.jsx:1276-1290`) only receives a product id, not a session id — after switching to real data, the handler must resolve the owning job via a product→job index built at cache time, and return `{ads:[], ecom:[]}` (matching today's mock fallback, `web/app.jsx:1283`) if the id isn't found or the job isn't cached yet (Task 5).
- **Page reload loses the in-memory report cache.** The session *list* must survive reload (backed by `localStorage`, Task 3), but the parsed `ui` block is kept in memory only. Reopening a previously "done" session after a reload must re-`prPollJob` (not assume a warm cache) so the detail tabs repopulate instead of showing empty data (Task 4, Task 5).

---

## File Structure

- **Create:** `web/product-research-client.js` — HTTP client (`prStartRun`, `prPollJob`), status mapping table, `localStorage`-backed session registry (`prRegistry*`), in-memory report cache (`prCacheUi`, `prGetCachedUi`) plus the product→job reverse index needed for `members`.
- **Modify:** `web/index.html` — add one `<script>` tag loading the new client before `niche.jsx`.
- **Modify:** `web/app.jsx` — replace the bodies of 11 existing `addRoute` registrations (lines 1154–1391) to use the new client instead of `fetch(NICHE_API_BASE, ...)` / the `NICHE_SESSIONS`/`NICHE_PRODUCTS` mock stores. No other route in the file changes.

---

### Task 1: `product-research-client.js` — HTTP calls + status mapping

**Files:**
- Create: `web/product-research-client.js`

**Interfaces:**
- Produces: `PR_API_BASE` (string const), `PR_STATUS_MAP` (object), `async function prStartRun({ jobId, keyword, countryCodes })` → `Promise<{job_id: string, status: string}>`, `async function prPollJob(jobId)` → `Promise<{job_id, type, status, result, error}>`.

- [ ] **Step 1: Write `web/product-research-client.js` with the two HTTP functions**

```js
const PR_API_BASE = "http://localhost:3009";

const PR_STATUS_MAP = {
  QUEUED: "running", RUNNING: "running",
  SUCCEEDED: "done", FAILED: "error", CANCELLED: "error",
};

async function prStartRun({ jobId, keyword, countryCodes }) {
  const res = await fetch(`${PR_API_BASE}/api/v1/product-research/runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jobId, keyword, countryCodes }),
  });
  const envelope = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((envelope.data && envelope.data.message) || `product-research: ${res.status}`);
  return envelope.data;
}

async function prPollJob(jobId) {
  const res = await fetch(`${PR_API_BASE}/api/v1/ai/jobs/${jobId}`);
  const envelope = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(`product-research: ${res.status}`);
  return envelope.data;
}
```

- [ ] **Step 2: Verify with a stubbed `fetch` (no live backend needed)**

Use `preview_eval` against `web/index.html` (served statically, see Task 2 setup) to run:

```js
(async () => {
  const originalFetch = window.fetch;
  window.fetch = async (url, opts) => ({
    ok: true,
    json: async () => url.endsWith("/runs")
      ? { status: 202, data: { job_id: "pr_test1", status: "QUEUED" } }
      : { status: 200, data: { job_id: "pr_test1", type: "product_research_crawl", status: "SUCCEEDED", result: { ui: { session: { raw_keyword: "x" } } }, error: null } },
  });
  const started = await prStartRun({ jobId: "pr_test1", keyword: "x", countryCodes: ["NL"] });
  const polled = await prPollJob("pr_test1");
  window.fetch = originalFetch;
  return { started, polledStatus: polled.status, uiSession: polled.result.ui.session };
})();
```

Expected: `started.status === "QUEUED"`, `polledStatus === "SUCCEEDED"`, `uiSession.raw_keyword === "x"`.

- [ ] **Step 3: Commit**

```bash
git add web/product-research-client.js
git commit -m "feat(web): add product-research HTTP client"
```

---

### Task 2: Load the client + verification harness

**Files:**
- Modify: `web/index.html:63-65`

**Interfaces:**
- Consumes: nothing new.
- Produces: `prStartRun`, `prPollJob`, `PR_STATUS_MAP` available as globals before `niche.jsx` and `app.jsx` execute.

- [ ] **Step 1: Add the script tag**

In `web/index.html`, insert immediately before the existing `<script src="i18n.js"></script>` → `<script type="text/babel" ... src="niche.jsx">` sequence (currently lines 63-65):

```html
<script src="i18n.js"></script>
<script src="product-research-client.js"></script>
<script type="text/babel" data-presets="react-classic" src="niche.jsx"></script>
```

- [ ] **Step 2: Start a static server and load the page**

```bash
cd web && python3 -m http.server 8090
```

Add a `.claude/launch.json` entry (`name: "web-static"`, `runtimeExecutable: "python3"`, `runtimeArgs: ["-m","http.server","8090"]`, `port: 8090`) if one doesn't already exist, then `preview_start` with that name.

- [ ] **Step 3: Verify globals are loaded**

`preview_eval`: `typeof prStartRun === "function" && typeof prPollJob === "function" && typeof PR_STATUS_MAP === "object"`

Expected: `true`.

- [ ] **Step 4: Commit**

```bash
git add web/index.html .claude/launch.json
git commit -m "chore(web): load product-research client script"
```

---

### Task 3: Session registry + report cache + product index

**Files:**
- Modify: `web/product-research-client.js`

**Interfaces:**
- Consumes: none.
- Produces: `prRegistryList()` → `Array<{id, raw_keyword, country_codes, created_at, last_status}>`; `prRegistrySave(meta)`; `prRegistryRemove(id)`; `prRegistryGet(id)` → meta or `undefined`; `prCacheUi(jobId, uiBlock)` (also populates the reverse index); `prGetCachedUi(jobId)` → `UiBlock` or `undefined`; `prFindJobIdForProduct(productId)` → `jobId` or `undefined`.

- [ ] **Step 1: Add registry + cache to `web/product-research-client.js`**

```js
const PR_REGISTRY_KEY = "pr_sessions_v1";

function prRegistryList() {
  try { return JSON.parse(localStorage.getItem(PR_REGISTRY_KEY) || "[]"); }
  catch { return []; }
}
function prRegistrySave(meta) {
  const list = prRegistryList().filter((m) => m.id !== meta.id);
  list.unshift(meta);
  localStorage.setItem(PR_REGISTRY_KEY, JSON.stringify(list));
}
function prRegistryRemove(id) {
  localStorage.setItem(PR_REGISTRY_KEY, JSON.stringify(prRegistryList().filter((m) => m.id !== id)));
}
function prRegistryGet(id) {
  return prRegistryList().find((m) => m.id === id);
}

const prReportCache = {};
const prProductIndex = {};
function prCacheUi(jobId, uiBlock) {
  prReportCache[jobId] = uiBlock;
  (uiBlock.products || []).forEach((p) => { prProductIndex[String(p.id)] = jobId; });
}
function prGetCachedUi(jobId) { return prReportCache[jobId]; }
function prFindJobIdForProduct(productId) { return prProductIndex[String(productId)]; }
```

- [ ] **Step 2: Verify via `preview_eval`**

```js
(() => {
  localStorage.removeItem("pr_sessions_v1");
  prRegistrySave({ id: "pr_a", raw_keyword: "kw", country_codes: ["NL"], created_at: Date.now(), last_status: "running" });
  const before = prRegistryList().length;
  prCacheUi("pr_a", { products: [{ id: 5 }, { id: 6 }] });
  const foundJob = prFindJobIdForProduct(5);
  prRegistryRemove("pr_a");
  const after = prRegistryList().length;
  return { before, foundJob, after, cached: !!prGetCachedUi("pr_a") };
})();
```

Expected: `before === 1`, `foundJob === "pr_a"`, `after === 0`, `cached === true` (cache removal is scoped separately — registry and cache are independent stores by design).

- [ ] **Step 3: Commit**

```bash
git add web/product-research-client.js
git commit -m "feat(web): add product-research session registry and report cache"
```

---

### Task 4: Rewire session lifecycle routes in `app.jsx`

**Files:**
- Modify: `web/app.jsx:1154-1195`

**Interfaces:**
- Consumes: `prStartRun`, `prPollJob`, `PR_STATUS_MAP`, `prRegistryList`, `prRegistrySave`, `prRegistryRemove`, `prRegistryGet`, `prCacheUi`, `prGetCachedUi` (Tasks 1 & 3).
- Produces: session-shell objects shaped like `UiSession` plus `id` and `country_codes`, consumed by Task 5's handlers and by `niche.jsx` unchanged.

- [ ] **Step 1: Replace `GET /api/niche/sessions` (currently `web/app.jsx:1154-1159`)**

Return `prRegistryList()` mapped to session-shell objects (`id`, `raw_keyword`, `country_codes`, `status: last_status`, `progress: last_status === "done" ? 100 : 0`) — no network call. Delete the old `fetch`+`nichePutReal` body.

- [ ] **Step 2: Replace `POST /api/niche/sessions` (currently `web/app.jsx:1161-1178`)**

Keep the existing validation (`raw_keyword` required, `country_codes` non-empty). Generate `const jobId = uid("pr");`, call `prStartRun({ jobId, keyword: body.raw_keyword.trim(), countryCodes: body.country_codes })`, then `prRegistrySave({ id: jobId, raw_keyword: body.raw_keyword.trim(), country_codes: body.country_codes, created_at: Date.now(), last_status: "running" })`, and return `{ id: jobId, raw_keyword: body.raw_keyword.trim(), country_codes: body.country_codes, status: "running", progress: 0 }`.

- [ ] **Step 3: Replace `GET /api/niche/sessions/:id` (currently `web/app.jsx:1180`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)", async ({ params }) => {
  const id = params[0];
  const meta = prRegistryGet(id) || { id, raw_keyword: "", country_codes: [] };
  const cached = prGetCachedUi(id);
  if (cached && meta.last_status === "done") return { ...cached.session, id, country_codes: meta.country_codes };
  const job = await prPollJob(id);
  const status = PR_STATUS_MAP[job.status] || "running";
  prRegistrySave({ ...meta, last_status: status });
  if (status === "done") {
    prCacheUi(id, job.result.ui);
    return { ...job.result.ui.session, id, country_codes: meta.country_codes };
  }
  if (status === "error") {
    return { id, raw_keyword: meta.raw_keyword, country_codes: meta.country_codes, status: "error", progress: 0, message: (job.error && job.error.message) || "Job thất bại" };
  }
  return { id, raw_keyword: meta.raw_keyword, country_codes: meta.country_codes, status: "running", progress: 0 };
});
```

- [ ] **Step 4: Replace `DELETE /api/niche/sessions/:id` (currently `web/app.jsx:1182-1189`)**

```js
addRoute("DELETE", "/api/niche/sessions/([^/]+)", ({ params }) => {
  if (!prRegistryGet(params[0])) throw new Error("Không tìm thấy phiên nghiên cứu ngách");
  prRegistryRemove(params[0]);
  delete prReportCache[params[0]];
  return { ok: true };
});
```

- [ ] **Step 5: Replace `POST /api/niche/sessions/:id/run` (currently `web/app.jsx:1191-1194`) with a no-op**

```js
addRoute("POST", "/api/niche/sessions/([^/]+)/run", async ({ params }) => {
  const meta = prRegistryGet(params[0]);
  if (!meta) throw new Error("Không tìm thấy phiên nghiên cứu ngách");
  return { id: params[0], raw_keyword: meta.raw_keyword, country_codes: meta.country_codes, status: meta.last_status || "running", progress: meta.last_status === "done" ? 100 : 0 };
});
```

- [ ] **Step 6: Verify with `preview_eval` (stub `fetch` as in Task 1 Step 2)**

```js
(async () => {
  const orig = window.fetch;
  let polls = 0;
  window.fetch = async (url) => ({
    ok: true,
    json: async () => url.endsWith("/runs")
      ? { status: 202, data: { job_id: JSON.parse(arguments[1]?.body || "{}").jobId, status: "QUEUED" } }
      : (polls++ === 0
          ? { status: 200, data: { status: "RUNNING", result: null, error: null } }
          : { status: 200, data: { status: "SUCCEEDED", result: { ui: { session: { raw_keyword: "kw", status: "done", step2_status: "done", step3_status: "done" }, products: [], profile: {}, sources: {}, members: {}, pricing: {}, summary: {} } }, error: null } }),
  });
  const created = await api("/api/niche/sessions", { method: "POST", body: { raw_keyword: "kw", country_codes: ["NL"] } });
  const runNoop = await api(`/api/niche/sessions/${created.id}/run`, { method: "POST", body: {} });
  const firstPoll = await api(`/api/niche/sessions/${created.id}`);
  const secondPoll = await api(`/api/niche/sessions/${created.id}`);
  const list = await api("/api/niche/sessions");

  // Simulate a page reload: registry says "done" but the in-memory report cache is gone.
  delete prReportCache[created.id];
  const afterReloadPoll = await api(`/api/niche/sessions/${created.id}`); // must re-poll, not crash

  window.fetch = orig;
  return {
    createdStatus: created.status, runNoopStatus: runNoop.status,
    firstPollStatus: firstPoll.status, secondPollStatus: secondPoll.status,
    listLen: list.length, afterReloadStatus: afterReloadPoll.status,
  };
})();

// Separate stub run for the FAILED path (fresh job id, single-shot fetch):
(async () => {
  const orig = window.fetch;
  window.fetch = async (url) => ({
    ok: true,
    json: async () => url.endsWith("/runs")
      ? { status: 202, data: { job_id: "pr_fail1", status: "QUEUED" } }
      : { status: 200, data: { status: "FAILED", result: null, error: { message: "boom" } } },
  });
  await api("/api/niche/sessions", { method: "POST", body: { raw_keyword: "kw2", country_codes: ["NL"] } });
  const failedSession = await api("/api/niche/sessions/pr_fail1");
  window.fetch = orig;
  return { failedStatus: failedSession.status, failedMessage: failedSession.message };
})();
```

Expected (first block): `createdStatus === "running"`, `runNoopStatus === "running"` (unchanged — proves the no-op doesn't corrupt state), `firstPollStatus === "running"`, `secondPollStatus === "done"`, `listLen >= 1`, `afterReloadStatus === "done"` (re-polled successfully despite the cleared cache — Review Focus item 5).
Expected (second block): `failedStatus === "error"`, `failedMessage === "boom"` (Review Focus item 3).

- [ ] **Step 7: Commit**

```bash
git add web/app.jsx
git commit -m "feat(web): drive niche session lifecycle from product-research /runs"
```

---

### Task 5: Rewire the 6 detail-read routes to serve cached `ui` sub-blocks

**Files:**
- Modify: `web/app.jsx:1255-1290` (`products`, `sources`, `members`)
- Modify: `web/app.jsx:1320-1324` (`pricing`)
- Modify: `web/app.jsx:1370-1375` (`summary`)
- Modify: `web/app.jsx:1384-1391` (`profile`)

**Interfaces:**
- Consumes: `prGetCachedUi`, `prFindJobIdForProduct` (Task 3).
- Produces: response shapes unchanged from today's mock contract (`{products}`, `{ads,ecom}`, `{products}`, `{keywords,audience,segments}`) so `niche.jsx` reads them the same way.

- [ ] **Step 1: Replace `GET /api/niche/sessions/:id/products` (`web/app.jsx:1255-1261`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)/products", ({ params }) => {
  const ui = prGetCachedUi(params[0]);
  return { products: (ui && ui.products) || [] };
});
```

(Drop the `include_filtered` query handling — the real pipeline's `top_n` products are already filtered server-side; there is no separate "filtered out" list to toggle in `UiProduct`.)

- [ ] **Step 2: Replace `GET /api/niche/sessions/:id/sources` (`web/app.jsx:1263-1274`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)/sources", ({ params }) => {
  const ui = prGetCachedUi(params[0]);
  return (ui && ui.sources) || { ads: [], ecom: [] };
});
```

- [ ] **Step 3: Replace `GET /api/niche/products/:id/members` (`web/app.jsx:1276-1290`)**

```js
addRoute("GET", "/api/niche/products/([^/]+)/members", ({ params }) => {
  const jobId = prFindJobIdForProduct(params[0]);
  const ui = jobId && prGetCachedUi(jobId);
  return (ui && ui.members && ui.members[params[0]]) || { ads: [], ecom: [] };
});
```

- [ ] **Step 4: Replace `GET /api/niche/sessions/:id/pricing` (`web/app.jsx:1320-1324`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)/pricing", ({ params }) => {
  const ui = prGetCachedUi(params[0]);
  return { products: (ui && ui.pricing && ui.pricing.products) || [] };
});
```

- [ ] **Step 5: Replace `GET /api/niche/sessions/:id/summary` (`web/app.jsx:1370-1375`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)/summary", ({ params }) => {
  const ui = prGetCachedUi(params[0]);
  return { products: (ui && ui.summary && ui.summary.products) || [] };
});
```

Leave `POST /api/niche/sessions/:id/summarize` (`web/app.jsx:1377-1382`) untouched — it stays mock (no server-side re-summarize trigger exists).

- [ ] **Step 6: Replace `GET /api/niche/sessions/:id/profile` (`web/app.jsx:1384-1391`)**

```js
addRoute("GET", "/api/niche/sessions/([^/]+)/profile", ({ params }) => {
  const ui = prGetCachedUi(params[0]);
  return (ui && ui.profile) || { keywords: [], audience: [], segments: {} };
});
```

- [ ] **Step 7: Verify with `preview_eval`**

```js
(() => {
  prCacheUi("pr_fixture", {
    products: [{ id: 1, name: "P1" }],
    sources: { ads: [{ id: 10 }], ecom: [] },
    members: { "1": { ads: [{ id: 10 }], ecom: [] } },
    pricing: { products: [{ id: 1, p_target: 19.99 }] },
    summary: { products: [{ id: 1, total_score: 8.1 }] },
    profile: { keywords: [{ term: "kw" }], audience: [], segments: {} },
  });
  return Promise.all([
    api("/api/niche/sessions/pr_fixture/products"),
    api("/api/niche/sessions/pr_fixture/sources"),
    api("/api/niche/products/1/members"),
    api("/api/niche/sessions/pr_fixture/pricing"),
    api("/api/niche/sessions/pr_fixture/summary"),
    api("/api/niche/sessions/pr_fixture/profile"),
    api("/api/niche/products/999/members"),
  ]);
})();
```

Expected: products `[{id:1,name:"P1"}]`; sources `{ads:[{id:10}],ecom:[]}`; members-of-1 `{ads:[{id:10}],ecom:[]}`; pricing `[{id:1,p_target:19.99}]`; summary `[{id:1,total_score:8.1}]`; profile has `keywords:[{term:"kw"}]`; members-of-999 (unknown id) `{ads:[],ecom:[]}`.

- [ ] **Step 8: Commit**

```bash
git add web/app.jsx
git commit -m "feat(web): serve niche detail tabs from cached product-research report"
```

---

### Task 6: Manual end-to-end smoke test against live backends

**Files:** none (verification-only task).

**Interfaces:**
- Consumes: everything from Tasks 1-5, plus a running `levelup_ai` (`CRAWL_DATABASE_URL` configured, arq worker for queue `arq:pr` running) and a running crawl `api-gateway`.

- [ ] **Step 1: Start `levelup_ai` API + its `ProductResearchWorkerSettings` arq worker, and the crawl repo's `api-gateway` + Postgres, per each repo's own run instructions.**

- [ ] **Step 2: `preview_start` the static `web/` server, open the SPA, click "Nghiên cứu ngách mới", pick a country, type a keyword, click "Bắt đầu nghiên cứu".**

- [ ] **Step 3: Use `preview_network` to confirm the exact call sequence: one `POST /api/v1/product-research/runs`, then repeated `GET /api/v1/ai/jobs/:id` every ~1200ms, zero calls to `niche-research-service:8084`.**

Expected: no request to port `8084` appears at all during this flow.

- [ ] **Step 4: Wait for the job to reach `SUCCEEDED`, confirm the drawer auto-advances (`niche.jsx:363`), then open the session detail page and click through Vùng 1/2/3/Tổng hợp tabs.**

Expected: `products`, `sources`, `pricing`, `summary`, `profile` tabs render real data (no `picsum.photos` placeholder images, no random mock numbers) matching what was in the job's `result.ui`.

- [ ] **Step 5: Click "Chạy lại phân tích" and confirm via `preview_network`/`preview_eval` that no new `/runs` call fires and the displayed data does not change (documents the accepted no-op limitation from Global Constraints).**

- [ ] **Step 6: Reload the page, reopen the same session from the list, and confirm it re-fetches via `GET /jobs/:id` and repopulates the tabs (validates the Review Focus item on cache loss after reload).**

- [ ] **Step 7: Record results in the PR description; no commit for this task.**
