/* product-research-client.js — HTTP client for levelup_ai's product_research pipeline.
   See docs/superpowers/plans/2026-09-30-niche-product-research-integration.md */

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
