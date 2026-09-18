/* Winora — Enterprise SPA (React + Tailwind, no build).
   Design: Linear / Stripe / Shopify Admin. Light mode, primary #2563EB.
   Lưu ý: JSX biên dịch classic (React.createElement) — cấu hình ở index.html. */
const { useState, useEffect, useRef, useMemo, useCallback } = React;
const ChartLib = window.Chart; // giữ tham chiếu Chart.js (tránh trùng tên với component)


/* ===================== MOCK API LAYER (no backend) =====================
   Replaces the real fetch()-based api() with an in-memory, pattern-matched
   mock so the whole SPA (Dashboard/Niche/Benchmark/Products/Landing/Ads)
   is fully clickable without a Go backend running.
   ========================================================================= */
const MOCK_LATENCY = 120;
function delay(v) { return new Promise((r) => setTimeout(() => r(v), MOCK_LATENCY)); }
let __uidSeq = 1;
function uid(prefix) { return `${prefix}_${(__uidSeq++).toString(36)}${Math.random().toString(36).slice(2, 6)}`; }

const ROUTES = [];
/* pattern: string, no ^$ needed. addRoute("GET", "/api/niche/sessions/([^/]+)", handler) */
function addRoute(method, pattern, handler) {
  ROUTES.push({ method: method.toUpperCase(), re: new RegExp(`^${pattern}$`), handler });
}

async function api(path, opts = {}) {
  const method = (opts.method || "GET").toUpperCase();
  const [rawPath, qs] = path.split("?");
  const query = Object.fromEntries(new URLSearchParams(qs || ""));
  for (const r of ROUTES) {
    if (r.method !== method) continue;
    const m = rawPath.match(r.re);
    if (m) {
      await delay();
      try {
        return r.handler({ params: m.slice(1), query, body: opts.body || {} });
      } catch (e) {
        throw new Error(e && e.message ? e.message : String(e));
      }
    }
  }
  await delay();
  console.warn("[mock-api] no handler for", method, path);
  return {};
}

/* ============================ /api/products ============================ */
const PRODUCTS_DB = [
  { id: "p1", name: "Áo polo nam công sở", category: "Áo nam", material: "Cotton cá sấu", description: "Polo form suông, thấm hút mồ hôi tốt.", cost_price: 95000, sell_price: 219000, status: "active", total_stock: 480, total_sold: 1320, variant_count: 6 },
  { id: "p2", name: "Set đồ bộ nữ mặc nhà", category: "Đồ bộ nữ", material: "Cotton lụa", description: "Set áo quần dài tay, form rộng.", cost_price: 78000, sell_price: 189000, status: "active", total_stock: 610, total_sold: 940, variant_count: 8 },
  { id: "p3", name: "Quần jean nam ống suông", category: "Quần nam", material: "Denim co giãn", description: "Jean xanh trơn, form basic.", cost_price: 135000, sell_price: 299000, status: "active", total_stock: 260, total_sold: 505, variant_count: 5 },
  { id: "p4", name: "Đồng phục nhóm/lớp cotton", category: "Đồng phục", material: "Cotton 65/35", description: "In theo yêu cầu, đặt số lượng lớn.", cost_price: 60000, sell_price: 129000, status: "draft", total_stock: 0, total_sold: 88, variant_count: 4 },
  { id: "p5", name: "Áo polo đoàn thanh niên", category: "Đồng phục", material: "Cá sấu Việt Thắng", description: "Thêu logo theo yêu cầu.", cost_price: 88000, sell_price: 175000, status: "active", total_stock: 320, total_sold: 610, variant_count: 6 },
  { id: "p6", name: "Váy suông linen nữ", category: "Váy nữ", material: "Linen pha", description: "Form suông, mát, dễ phối đồ.", cost_price: 102000, sell_price: 259000, status: "archived", total_stock: 40, total_sold: 210, variant_count: 4 },
];
let VARIANTS_DB = {
  p1: [
    { id: "v1", product_id: "p1", color: "Trắng", size: "M", stock: 80, sold: 220 },
    { id: "v2", product_id: "p1", color: "Trắng", size: "L", stock: 70, sold: 260 },
    { id: "v3", product_id: "p1", color: "Xanh navy", size: "M", stock: 90, sold: 300 },
    { id: "v4", product_id: "p1", color: "Xanh navy", size: "L", stock: 60, sold: 240 },
    { id: "v5", product_id: "p1", color: "Đen", size: "M", stock: 100, sold: 180 },
    { id: "v6", product_id: "p1", color: "Đen", size: "L", stock: 80, sold: 120 },
  ],
  p2: [
    { id: "v7", product_id: "p2", color: "Hồng phấn", size: "Freesize", stock: 300, sold: 480 },
    { id: "v8", product_id: "p2", color: "Xám", size: "Freesize", stock: 310, sold: 460 },
  ],
};
function productSummary(p) {
  const vs = VARIANTS_DB[p.id] || [];
  const total_stock = vs.length ? vs.reduce((s, v) => s + Number(v.stock || 0), 0) : p.total_stock;
  const total_sold = vs.length ? vs.reduce((s, v) => s + Number(v.sold || 0), 0) : p.total_sold;
  const variant_count = vs.length || p.variant_count;
  return { ...p, total_stock, total_sold, variant_count };
}
addRoute("GET", "/api/products", () => {
  const products = PRODUCTS_DB.map(productSummary);
  const categories = [...new Set(PRODUCTS_DB.map((p) => p.category).filter(Boolean))];
  return {
    products,
    categories,
    stats: {
      products: products.length,
      total_stock: products.reduce((s, p) => s + p.total_stock, 0),
      total_sold: products.reduce((s, p) => s + p.total_sold, 0),
    },
  };
});
addRoute("POST", "/api/products", ({ body }) => {
  const p = { id: uid("p"), status: "active", total_stock: 0, total_sold: 0, variant_count: 0, ...body };
  PRODUCTS_DB.push(p);
  return productSummary(p);
});
addRoute("PUT", "/api/products/([^/]+)", ({ params, body }) => {
  const idx = PRODUCTS_DB.findIndex((p) => p.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy sản phẩm");
  PRODUCTS_DB[idx] = { ...PRODUCTS_DB[idx], ...body };
  return productSummary(PRODUCTS_DB[idx]);
});
addRoute("DELETE", "/api/products/([^/]+)", ({ params }) => {
  PRODUCTS_DB = PRODUCTS_DB.filter((p) => p.id !== params[0]);
  delete VARIANTS_DB[params[0]];
  return { ok: true };
});
addRoute("GET", "/api/products/([^/]+)/variants", ({ params }) => VARIANTS_DB[params[0]] || []);
addRoute("POST", "/api/products/([^/]+)/variants", ({ params, body }) => {
  const v = { id: uid("v"), product_id: params[0], stock: 0, sold: 0, ...body };
  VARIANTS_DB[params[0]] = [...(VARIANTS_DB[params[0]] || []), v];
  return v;
});
addRoute("PUT", "/api/variants/([^/]+)", ({ params, body }) => {
  for (const pid in VARIANTS_DB) {
    const idx = VARIANTS_DB[pid].findIndex((v) => v.id === params[0]);
    if (idx !== -1) {
      VARIANTS_DB[pid][idx] = { ...VARIANTS_DB[pid][idx], ...body };
      return VARIANTS_DB[pid][idx];
    }
  }
  throw new Error("Không tìm thấy biến thể");
});
addRoute("DELETE", "/api/variants/([^/]+)", ({ params }) => {
  for (const pid in VARIANTS_DB) {
    VARIANTS_DB[pid] = VARIANTS_DB[pid].filter((v) => v.id !== params[0]);
  }
  return { ok: true };
});

/* ============================ /api/landing ============================ */
let LANDING_DB = [
  { id: "l1", title: "Polo nam công sở - Sale 30%", slug: "polo-nam-cong-so", product_name: "Áo polo nam công sở", status: "published", views: 12500, conversions: 640, revenue: 140160000, headline: "Polo cá sấu chuẩn công sở", subheadline: "Thấm hút, không nhăn, form chuẩn", body: "Chất liệu cotton cá sấu cao cấp, form suông trẻ trung, phù hợp đi làm và dạo phố.", cta_text: "Mua ngay", theme: "light" },
  { id: "l2", title: "Set đồ bộ mặc nhà siêu mềm", slug: "set-do-bo-mac-nha", product_name: "Set đồ bộ nữ mặc nhà", status: "published", views: 8900, conversions: 410, revenue: 77490000, headline: "Mềm mại cả ngày dài", subheadline: "Chất cotton lụa mát rượi", body: "Form rộng thoải mái, nhiều màu lựa chọn, giặt máy không xù lông.", cta_text: "Đặt ngay", theme: "light" },
  { id: "l3", title: "Đồng phục nhóm - Đặt sỉ", slug: "dong-phuc-nhom", product_name: "Đồng phục nhóm/lớp cotton", status: "draft", views: 320, conversions: 6, revenue: 774000, headline: "In logo theo yêu cầu", subheadline: "Giá tốt cho đơn từ 20 áo", body: "Nhận đặt may đồng phục lớp, nhóm, công ty. Giao nhanh 5-7 ngày.", cta_text: "Nhận báo giá", theme: "dark" },
];
addRoute("GET", "/api/landing", () => LANDING_DB);
addRoute("POST", "/api/landing", ({ body }) => {
  const p = { id: uid("l"), slug: (body.title || "trang-moi").toLowerCase().replace(/[^a-z0-9]+/g, "-"), status: "draft", views: 0, conversions: 0, revenue: 0, ...body };
  LANDING_DB.push(p);
  return p;
});
addRoute("PUT", "/api/landing/([^/]+)", ({ params, body }) => {
  const idx = LANDING_DB.findIndex((p) => p.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy landing page");
  LANDING_DB[idx] = { ...LANDING_DB[idx], ...body };
  return LANDING_DB[idx];
});
addRoute("DELETE", "/api/landing/([^/]+)", ({ params }) => {
  LANDING_DB = LANDING_DB.filter((p) => p.id !== params[0]);
  return { ok: true };
});
addRoute("POST", "/api/landing/([^/]+)/publish", ({ params }) => {
  const idx = LANDING_DB.findIndex((p) => p.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy landing page");
  LANDING_DB[idx].status = "published";
  return LANDING_DB[idx];
});
addRoute("GET", "/api/landing/([^/]+)/preview", ({ params }) => {
  const p = LANDING_DB.find((x) => x.id === params[0]);
  return `<html><body style="font-family:sans-serif;padding:24px"><h1>${p ? p.headline : ""}</h1><p>${p ? p.subheadline : ""}</p></body></html>`;
});

/* ============================ /api/ads ============================ */
function buildAdsDaily(n, spendTotal, revTotal) {
  const out = [];
  const start = new Date(); start.setDate(start.getDate() - n);
  for (let i = 0; i < n; i++) {
    const d = new Date(start); d.setDate(d.getDate() + i);
    const w = 0.6 + Math.random() * 0.8;
    out.push({ date: d.toISOString().slice(0, 10), spend: Math.round((spendTotal / n) * w), revenue: Math.round((revTotal / n) * w) });
  }
  return out;
}
let CAMPAIGNS_DB = [
  { id: "c1", name: "TikTok - Áo polo nam hè", status: "active", objective: "Conversions", daily_budget: 800000, kpi: { roas: 3.42, spend: 18500000, revenue: 63270000, ctr: 2.1, cpa: 42000, purchases: 452 }, ai_verdict: "scale", ai_reasoning: "ROAS ổn định trên 3.0 trong 7 ngày liên tiếp, CPA thấp hơn giá vốn. Đề xuất tăng ngân sách thêm 20-30%." },
  { id: "c2", name: "Facebook - Set đồ bộ nữ", status: "active", objective: "Conversions", daily_budget: 500000, kpi: { roas: 1.15, spend: 12300000, revenue: 14145000, ctr: 0.9, cpa: 88000, purchases: 161 }, ai_verdict: "kill", ai_reasoning: "ROAS dưới ngưỡng hoà vốn (1.3) trong 5 ngày liên tiếp, chi phí trên mỗi đơn tăng dần." },
  { id: "c3", name: "Shopee Ads - Quần jean nam", status: "paused", objective: "Traffic", daily_budget: 300000, kpi: { roas: 2.05, spend: 8200000, revenue: 16810000, ctr: 1.4, cpa: 61000, purchases: 134 }, ai_verdict: "hold" },
  { id: "c4", name: "TikTok - Đồng phục nhóm", status: "active", objective: "Reach", daily_budget: 400000, kpi: { roas: 0.87, spend: 9600000, revenue: 8352000, ctr: 0.6, cpa: 121000, purchases: 69 }, ai_verdict: "kill" },
  { id: "c5", name: "Facebook - Polo đoàn thanh niên", status: "active", objective: "Conversions", daily_budget: 600000, kpi: { roas: 2.78, spend: 14000000, revenue: 38920000, ctr: 1.8, cpa: 51000, purchases: 274 }, ai_verdict: "optimize", ai_reasoning: "Hiệu quả tốt nhưng tần suất hiển thị (frequency) đang tăng cao, nên làm mới creative." },
];
addRoute("GET", "/api/ads/campaigns", () => CAMPAIGNS_DB);
addRoute("POST", "/api/ads/campaigns", ({ body }) => {
  const c = { id: uid("c"), status: "active", kpi: { roas: 0, spend: 0, revenue: 0, ctr: 0, cpa: 0, purchases: 0 }, ...body };
  CAMPAIGNS_DB.push(c);
  return c;
});
addRoute("PUT", "/api/ads/campaigns/([^/]+)", ({ params, body }) => {
  const idx = CAMPAIGNS_DB.findIndex((c) => c.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy campaign");
  CAMPAIGNS_DB[idx] = { ...CAMPAIGNS_DB[idx], ...body };
  return CAMPAIGNS_DB[idx];
});
addRoute("GET", "/api/ads/campaigns/([^/]+)/metrics", ({ params }) => {
  const c = CAMPAIGNS_DB.find((x) => x.id === params[0]);
  if (!c) throw new Error("Không tìm thấy campaign");
  return { kpi: c.kpi, daily: buildAdsDaily(14, c.kpi.spend, c.kpi.revenue) };
});
addRoute("POST", "/api/ads/campaigns/([^/]+)/evaluate", ({ params }) => {
  const c = CAMPAIGNS_DB.find((x) => x.id === params[0]);
  if (!c) throw new Error("Không tìm thấy campaign");
  const verdict = c.kpi.roas >= 2.5 ? "scale" : c.kpi.roas >= 1.5 ? "optimize" : c.kpi.roas >= 1.1 ? "hold" : "kill";
  const reasoning = {
    scale: "ROAS vượt ngưỡng mục tiêu, hiệu quả chi tiêu tốt. Đề xuất tăng ngân sách 20-30% và nhân bản tập khách hàng tương tự (lookalike).",
    optimize: "Hiệu quả khá nhưng còn dư địa cải thiện creative/targeting để tăng ROAS trước khi scale ngân sách.",
    hold: "ROAS quanh ngưỡng hoà vốn, nên giữ nguyên ngân sách và theo dõi thêm 3-5 ngày trước khi quyết định.",
    kill: "ROAS dưới ngưỡng hoà vốn nhiều ngày liên tiếp, chi phí trên mỗi đơn cao. Đề xuất tắt hoặc làm lại creative/targeting.",
  }[verdict];
  c.ai_verdict = verdict; c.ai_reasoning = reasoning;
  return { verdict, reasoning };
});

/* ============================ dashboard/overview (harmless extras) ============================ */
addRoute("GET", "/api/dashboard", () => ({}));
addRoute("GET", "/api/overview", () => ({}));
/* ============================================================================
   NICHE MOCK API — dữ liệu giả lập cho màn "Nghiên cứu ngách" (niche.jsx).
   File này được nạp (as <script>) TRƯỚC khi app.jsx định nghĩa hàm api(), và
   giả định các hàm sau đã tồn tại ở scope toàn cục:
     - function delay(v)            -> Promise trả `v` sau ~120ms
     - function uid(prefix)         -> id ngắn, duy nhất
     - function addRoute(method, pattern, handler)
           handler({ params, query, body }) -> giá trị trả về (không phải Promise)
           hoặc throw new Error(msg) để mô phỏng lỗi 4xx/5xx.
   Toàn bộ hằng số / hàm ở đây nằm ở scope toàn cục của trang (không bọc IIFE)
   nên được đặt tên với tiền tố NICHE_ / niche* để tránh đụng độ với app.jsx.
   ========================================================================== */

/* --------------------------- 0. Tiện ích nhỏ --------------------------- */
function nicheNowIso(offsetMin) {
  return new Date(Date.now() - (offsetMin || 0) * 60000).toISOString();
}
function nicheClamp(v, a, b) { return Math.max(a, Math.min(b, v)); }
function nicheRound1(v) { return Math.round(v * 10) / 10; }

const NICHE_COUNTRY_CURRENCY = {
  NL: "EUR", DE: "EUR", BE: "EUR", FR: "EUR", ES: "EUR", IT: "EUR", PT: "EUR",
  PL: "PLN", CZ: "CZK", SE: "SEK",
  US: "USD", CA: "CAD", GB: "GBP",
  VN: "VND", SG: "SGD",
};
const NICHE_COUNTRY_NAME = {
  NL: "Hà Lan", DE: "Đức", BE: "Bỉ", FR: "Pháp", ES: "Tây Ban Nha", IT: "Ý", PT: "Bồ Đào Nha",
  PL: "Ba Lan", CZ: "Séc", SE: "Thuỵ Điển",
  US: "Mỹ", CA: "Canada", GB: "Anh",
  VN: "Việt Nam", SG: "Singapore",
};

const NICHE_REGION_TREE = [
  {
    key: "eu", label: "Châu Âu",
    regions: [
      { key: "west_eu", label: "Tây Âu", countries: [
        { code: "NL", name: "Hà Lan", currency: "EUR" },
        { code: "DE", name: "Đức", currency: "EUR" },
        { code: "BE", name: "Bỉ", currency: "EUR" },
        { code: "FR", name: "Pháp", currency: "EUR" },
      ] },
      { key: "south_eu", label: "Nam Âu", countries: [
        { code: "ES", name: "Tây Ban Nha", currency: "EUR" },
        { code: "IT", name: "Ý", currency: "EUR" },
        { code: "PT", name: "Bồ Đào Nha", currency: "EUR" },
      ] },
      { key: "east_eu", label: "Đông Âu", countries: [
        { code: "PL", name: "Ba Lan", currency: "PLN" },
        { code: "CZ", name: "Séc", currency: "CZK" },
      ] },
      { key: "north_eu", label: "Bắc Âu", countries: [
        { code: "SE", name: "Thuỵ Điển", currency: "SEK" },
      ] },
    ],
  },
  {
    key: "na", label: "Bắc Mỹ",
    regions: [
      { key: "na_main", label: "Bắc Mỹ", countries: [
        { code: "US", name: "Mỹ", currency: "USD" },
        { code: "CA", name: "Canada", currency: "CAD" },
      ] },
    ],
  },
  {
    key: "uk", label: "Vương quốc Anh",
    regions: [
      { key: "uk_main", label: "Vương quốc Anh", countries: [
        { code: "GB", name: "Anh", currency: "GBP" },
      ] },
    ],
  },
  {
    key: "asia", label: "Châu Á",
    regions: [
      { key: "sea", label: "Đông Nam Á", countries: [
        { code: "VN", name: "Việt Nam", currency: "VND" },
        { code: "SG", name: "Singapore", currency: "SGD" },
      ] },
    ],
  },
];

/* --------------------------- 1. /api/niche/meta --------------------------- */
const NICHE_META = {
  volume_source: "demo",         // chưa cấu hình Google Ads API
  audience_source: "demo",       // chưa có token Meta Marketing API
  ai_enabled: true,              // dùng Claude cho các nhận định
  region_tree: NICHE_REGION_TREE,
  steps: [
    { key: "keywords", label: "Bộ từ khoá bản địa", until: 34 },
    { key: "trend", label: "Lượt tìm kiếm 12 tháng & mùa vụ", until: 67 },
    { key: "audience", label: "Tệp đối tượng & chấm điểm", until: 100 },
  ],
  step2_stages: [
    { key: "ads_scan", label: "1. Spy quảng cáo trên Meta Ad Library", until: 25 },
    { key: "ecom_scan", label: "2. Quét sản phẩm trên sàn TMĐT", until: 55 },
    { key: "match", label: "3. Ghép nối Ads ↔ sàn theo cụm SKU", until: 80 },
    { key: "score", label: "4. Chấm điểm S2 & tệp đối tượng ads", until: 100 },
  ],
  step3_stages: [
    { key: "collect", label: "1. Mở link ads & link sàn lấy giá thực tế", until: 40 },
    { key: "benchmark", label: "2. Lấy benchmark CPM/CPC/CTR Meta", until: 75 },
    { key: "score", label: "3. Tính giá bán mục tiêu & chấm điểm S3", until: 100 },
  ],
  step4_stages: [
    { key: "crawl", label: "1. Cào nhà cung cấp trên 1688 / Taobao / Alibaba", until: 45 },
    { key: "match", label: "2. So khớp ảnh & từ khoá sản phẩm", until: 75 },
    { key: "score", label: "3. So giá niêm yết với trần giá vốn", until: 100 },
  ],
  pricing: { cogs_share: 0.4, ads_share: 0.3, assumed_cvr: 0.02, fx_source: "static" },
  sourcing: { supplier_source: "demo", logistics_good: 0.08,
    weights: { cogs: 0.5, logistics: 0.3, leadtime: 0.2 } },
  audience_bands: [
    { key: "too_narrow", min: 0, max: 100000, note: "Quá hẹp — CPM sẽ tăng vọt, khó duy trì chiến dịch." },
    { key: "narrow", min: 100000, max: 500000, note: "Hơi hẹp — chỉ phù hợp giai đoạn test." },
    { key: "ideal", min: 500000, max: 5000000, note: "Vùng lý tưởng để chạy CBO ổn định." },
    { key: "wide", min: 5000000, max: 20000000, note: "Rộng — cần thêm lớp lọc để giữ CPA thấp." },
    { key: "broad", min: 20000000, max: null, note: "Quá rộng — nên bật Advantage+ thay vì detailed targeting." },
  ],
  total_scoring: [
    { key: "seasonality", label: "Mùa vụ", weight: 0.15 },
    { key: "audience_quality", label: "Chất lượng tệp", weight: 0.15 },
    { key: "ads_match", label: "Khớp tệp ads", weight: 0.15 },
    { key: "appeal", label: "Sức hút sản phẩm", weight: 0.25 },
    { key: "price_margin", label: "Dư địa giá", weight: 0.15 },
    { key: "sourcing", label: "Nguồn hàng", weight: 0.15 },
  ],
};

/* ====================================================================
   2. DEMO SESSION — "áo polo nam công sở" (NL · DE · BE)
   ==================================================================== */
const DEMO_SID = "niche_demo_1";

/* ---- 2.1 Bộ từ khoá bản địa + lượt tìm kiếm 12 tháng ---- */
function nicheMonthly(base, bumpMonths, bumpPct) {
  // 12 tháng gần nhất tính lùi từ hôm nay (không quan trọng năm chính xác với demo)
  const out = [];
  const now = new Date();
  for (let i = 11; i >= 0; i--) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const m = d.getMonth() + 1, y = d.getFullYear();
    const bump = (bumpMonths || []).includes(m) ? (bumpPct || 0.35) : 0;
    const wobble = Math.sin(i * 1.7) * 0.06;
    out.push({ month: m, year: y, searches: Math.round(base * (1 + bump + wobble)) });
  }
  return out;
}
const NICHE_KEYWORDS_DEMO = [
  { id: "kw_1", keyword: "heren poloshirt business", translation: "áo polo nam công sở (từ khoá Hà Lan)",
    avg_monthly_searches: 14800, monthly_volumes: nicheMonthly(14800, [8, 9], 0.4) },
  { id: "kw_2", keyword: "polo shirt herren büro", translation: "áo polo nam công sở (từ khoá Đức)",
    avg_monthly_searches: 12600, monthly_volumes: nicheMonthly(12600, [8, 9], 0.32) },
  { id: "kw_3", keyword: "smart polo shirt men", translation: "áo polo lịch sự cho nam (từ khoá tiếng Anh, dùng chung NL/DE/BE)",
    avg_monthly_searches: 8700, monthly_volumes: nicheMonthly(8700, [11, 12], 0.25) },
  { id: "kw_4", keyword: "poloshirt heren katoen strijkvrij", translation: "áo polo nam vải cotton chống nhăn",
    avg_monthly_searches: 4400, monthly_volumes: nicheMonthly(4400, [8, 9], 0.5) },
];

/* ---- 2.2 Tệp đối tượng Meta + các lát cắt targeting ---- */
function nicheAudienceCriteria() {
  return [
    { key: "size", label: "Quy mô tệp", weight: 0.3, score: 8.5,
      note: "1,1 – 1,6 triệu người — nằm gọn trong vùng lý tưởng để chạy CBO." },
    { key: "match", label: "Độ khớp persona", weight: 0.3, score: 8.0,
      note: "Sở thích/hành vi đều xoay quanh dân văn phòng, ăn khớp với sản phẩm." },
    { key: "overlap", label: "Trùng lặp giữa các lát cắt", weight: 0.2, score: 7.5,
      note: "Một phần \"Business casual fashion\" trùng với \"Online fashion shoppers\", chấp nhận được." },
    { key: "stability", label: "Ổn định theo thời gian", weight: 0.2, score: 8.2,
      note: "Không phụ thuộc sự kiện thời vụ ngắn hạn, tệp bền vững quanh năm." },
  ];
}
function nicheAudienceQuality() {
  const c = nicheAudienceCriteria();
  const score = nicheRound1(c.reduce((s, x) => s + x.score * x.weight, 0));
  return {
    score, label: score >= 8 ? "Tệp tốt, sẵn sàng chạy CBO" : score >= 5.5 ? "Tệp ổn, cần thu hẹp thêm" : "Tệp yếu",
    criteria: c,
    actions: [
      "Loại bớt độ tuổi 18-24 vì tỉ lệ mua hàng công sở thấp.",
      "Thử tách riêng chiến dịch cho nam/nữ để so hiệu suất trước khi gộp Advantage+.",
      "Bổ sung lookalike 1% từ danh sách khách đã mua để mở rộng khi tệp bắt đầu bão hoà.",
    ],
  };
}
const NICHE_SEGMENTS_DEMO = {
  country: [
    { id: "seg_c_nl", kind: "country", label: "Netherlands", external_id: "NL", size_upper: 14200000, selected: true },
    { id: "seg_c_de", kind: "country", label: "Germany", external_id: "DE", size_upper: 68300000, selected: true },
    { id: "seg_c_be", kind: "country", label: "Belgium", external_id: "BE", size_upper: 8600000, selected: true },
  ],
  age: [
    { id: "seg_a_2534", kind: "age", label: "25-34", size_upper: 2100000, selected: true },
    { id: "seg_a_3544", kind: "age", label: "35-44", size_upper: 1950000, selected: true },
    { id: "seg_a_4554", kind: "age", label: "45-54", size_upper: 1400000, selected: false },
    { id: "seg_a_1824", kind: "age", label: "18-24", size_upper: 980000, selected: false },
  ],
  gender: [
    { id: "seg_g_m", kind: "gender", label: "Nam", size_upper: 45000000, selected: true },
    { id: "seg_g_f", kind: "gender", label: "Nữ", size_upper: 46000000, selected: false },
  ],
  interest: [
    { id: "seg_i_business_casual", kind: "interest", label: "Business casual fashion",
      label_vi: "Thời trang công sở lịch sự", size_upper: 3200000, share: 0.035, selected: true },
    { id: "seg_i_office_wear", kind: "interest", label: "Office wear",
      label_vi: "Trang phục đi làm", size_upper: 2400000, share: 0.028, selected: true },
    { id: "seg_i_menswear", kind: "interest", label: "Men's fashion",
      label_vi: "Thời trang nam", size_upper: 9800000, share: 0.09, selected: false },
  ],
  behavior: [
    { id: "seg_b_online_shoppers", kind: "behavior", label: "Engaged shoppers – fashion",
      label_vi: "Mua sắm online — thời trang", size_upper: 5600000, share: 0.05, selected: true },
    { id: "seg_b_frequent_travelers", kind: "behavior", label: "Frequent business travelers",
      label_vi: "Thường xuyên công tác", size_upper: 1100000, share: 0.012, selected: false },
  ],
  demographic: [
    { id: "seg_d_whitecollar", kind: "demographic", label: "Job title: Office / admin",
      label_vi: "Chức danh: Nhân viên văn phòng", size_upper: 4100000, share: 0.037, selected: true },
    { id: "seg_d_managers", kind: "demographic", label: "Job title: Managers",
      label_vi: "Chức danh: Quản lý", size_upper: 1600000, share: 0.015, selected: false },
  ],
};
const NICHE_AUDIENCE_DEMO = [
  { platform: "meta", lower_bound: 1100000, upper_bound: 1600000, verdict: "ideal",
    note: "Tệp nằm trong vùng lý tưởng — đủ rộng để CBO tối ưu, đủ hẹp để giữ CPM thấp.",
    source: "demo", quality: nicheAudienceQuality() },
];
const NICHE_TARGETING_POOL = [
  { label: "Corporate fashion", label_vi: "Thời trang doanh nghiệp", kind: "interest", size_upper: 1800000 },
  { label: "Smart casual style", label_vi: "Phong cách smart casual", kind: "interest", size_upper: 2600000 },
  { label: "LinkedIn (website)", label_vi: "Người dùng LinkedIn", kind: "behavior", size_upper: 6200000 },
  { label: "Small business owners", label_vi: "Chủ doanh nghiệp nhỏ", kind: "demographic", size_upper: 2000000 },
  { label: "Remote hybrid workers", label_vi: "Nhân viên làm việc hybrid", kind: "behavior", size_upper: 3300000 },
  { label: "Job title: HR", label_vi: "Chức danh: Nhân sự", kind: "demographic", size_upper: 900000 },
  { label: "Job title: Sales", label_vi: "Chức danh: Kinh doanh", kind: "demographic", size_upper: 2700000 },
  { label: "Formal wear", label_vi: "Trang phục lịch sự / vest", kind: "interest", size_upper: 4100000 },
];

/* ---- 2.3 Sản phẩm tiềm năng (Vùng 2/3/4) ---- */
function nicheImg(seed) { return `https://picsum.photos/seed/niche-polo-${seed}/300/300`; }
function adItem(id, guess, page, platform, kw, days, reach, copy, cluster, status) {
  return { id, product_guess: guess, page_name: page, platform, matched_keyword: kw,
    active_days: days, reach, ad_copy: copy, ad_link: "https://facebook.com/ads/library/?id=" + id,
    cluster_id: cluster, ai_status: status || "valid" };
}
function ecomItem(id, name, platform, rating, reviews, sold, price, currency, kw, cluster, kept, url) {
  return { id, name, image_url: nicheImg(id), platform, rating, reviews_count: reviews,
    sales_volume: sold, price, currency, matched_keyword: kw, cluster_id: cluster,
    kept: kept !== false, url: url || "https://" + platform + ".com/item/" + id };
}
function supplierItem(id, platform, name, title, moq, years, repeat, rating, matchType, matchScore,
                       listedUsd, listedLocal, currency, trust) {
  return { id, platform, supplier_name: name, title, moq, years_active: years, repeat_rate: repeat,
    rating, match_type: matchType, match_score: matchScore, listed_cogs_usd: listedUsd,
    listed_cogs: listedLocal, currency, trust_score: trust,
    url: "https://" + platform + ".com/supplier/" + id };
}
function priceItem(id, source, shop, platform, raw, usd, local, currency, by, url) {
  return { id, source, shop_name: shop, platform, raw_text: raw, price_usd: usd,
    price_local: local, currency, extracted_by: by, url };
}
function scoreCriteria(vals) {
  // vals: { seasonality, audience_quality, ads_match, appeal, price_margin, sourcing } (null = missing)
  const labels = { seasonality: "Mùa vụ", audience_quality: "Chất lượng tệp", ads_match: "Khớp tệp ads",
    appeal: "Sức hút sản phẩm", price_margin: "Dư địa giá", sourcing: "Nguồn hàng" };
  const weights = { seasonality: 0.15, audience_quality: 0.15, ads_match: 0.15,
    appeal: 0.25, price_margin: 0.15, sourcing: 0.15 };
  const notes = {
    seasonality: "Theo đánh giá mùa vụ chung của ngách ở Bước 1.",
    audience_quality: "Theo điểm tệp đối tượng chung của ngách ở Bước 1.",
    ads_match: "So khớp tệp thực tế của ads với tệp mục tiêu Bước 1.",
    appeal: "Từ rating, lượt mua và số lượng ads/shop đang bán.",
    price_margin: "Từ S3.1 (dư địa giá bán) và S3.2 (dung lượng trần giá vốn).",
    sourcing: "Từ S4 — landed cost so với trần giá vốn & thời gian giao hàng.",
  };
  const criteria = Object.keys(labels).map((key) => ({
    key, label: labels[key], weight: weights[key],
    score: vals[key] == null ? null : vals[key], missing: vals[key] == null, note: notes[key],
  }));
  const coverage = nicheRound1(criteria.filter((c) => !c.missing)
    .reduce((s, c) => s + c.weight, 0));
  return { criteria, coverage };
}
function totalFromBreakdown(bd) {
  const present = bd.criteria.filter((c) => !c.missing);
  const wsum = present.reduce((s, c) => s + c.weight, 0) || 1;
  return nicheRound1(present.reduce((s, c) => s + c.score * c.weight, 0) / wsum);
}

const NICHE_PRODUCTS_DEMO = [
  {
    id: "p_polo_1", rank: 1, name: "Áo polo nam vải cá sấu phối cổ tương phản",
    image_url: nicheImg(1), matched_keyword: "heren poloshirt business", match_status: "matched_both",
    match_bonus: 1.2, ad_status: "valid",
    ads_active_days: 62, ad_reach: 185000, ads_count: 14,
    ecom_platform: "bol", revenue: 42000, currency: "EUR", sales_volume: 3800, shops_count: 9,
    rating: 4.6, reviews_count: 1240, price: 24.95,
    aud_age_top: "25-34", aud_age_share: 0.34, aud_gender_top: "male", aud_gender_share: 0.71,
    aud_match: "match", aud_match_note: "Tệp ads tập trung đúng nam 25-44 tuổi — khớp persona nhân viên văn phòng.",
    aud_breakdown: { ages: [{ label: "18-24", share: 0.09 }, { label: "25-34", share: 0.34 },
      { label: "35-44", share: 0.31 }, { label: "45-54", share: 0.18 }, { label: "55+", share: 0.08 }],
      genders: [{ label: "male", share: 0.71 }, { label: "female", share: 0.29 }] },
    ad_countries: ["NL", "DE", "BE"],
    s21: 9.0, s22: 8.6, s2: 9.1,
    pros: ["Vải mềm mát, không nhăn cả ngày làm việc", "Form ôm vừa, lên dáng công sở"],
    cons: ["Size hơi nhỏ hơn bảng size công bố", "Màu xanh navy dễ phai sau 20 lần giặt"],
    pain_points: "Khách hàng công sở khó tìm áo polo vừa lịch sự vừa co giãn thoải mái cả ngày ngồi máy lạnh.",
    ad_reason: "Cùng cụm ảnh sản phẩm + cùng khoảng giá 24-27€ xuất hiện ở cả ads và trang bol.com.",
    shortlisted: true,
    p_min: 19.9, p_avg: 26.4, p_target: 29.9, p_max: 34.5, cogs_max: 11.96, cpa_target_max: 8.97,
    cpa_estimated: 7.4, ads_feasible: true, s31: 8.5, s32: 8.0, s3: 8.3, price_points: 5,
    pricing_source: "claude",
    pricing_note: "Giá đối thủ tập trung 24-30€. Đặt P_target 29,9€ vẫn nằm trong nửa trên của dải, đủ biên cho ads.",
    cpm_benchmark: 9.8, ctr_benchmark: 0.014, cpc_benchmark: 0.7,
    prices: [
      priceItem("pr1", "ecom", "bol.com", "bol", "Poloshirt heren slim fit - navy - 24,95€", 26.9, 24.95, "EUR", "regex", "https://bol.com/p/1"),
      priceItem("pr2", "ads_landing", "OfficeWear NL", "landing", "Premium heren polo — nu 27,99€ i.p.v. 39,99€", 30.1, 27.99, "EUR", "claude", "https://officewear.nl/polo"),
      priceItem("pr3", "ecom", "Zalando", "bol", "Polo business heren - 32,00€", 34.4, 32.0, "EUR", "regex", "https://zalando.nl/p/1"),
      priceItem("pr4", "ecom", "Amazon.de", "amazon", "Herren Poloshirt Business - 21,90€", 23.6, 21.9, "EUR", "demo", "https://amazon.de/dp/1"),
      priceItem("pr5", "ads_landing", "Berlin Menswear", "landing", "Business Polo Bundle 2x — 54€", 29.2, 27.0, "EUR", "claude", "https://berlinmenswear.de/bundle"),
    ],
    suppliers: [
      supplierItem("sup1", "1688", "Dongguan Junyi Garment", "Nam polo cá sấu cotton pha, in logo OEM", 50, 6, 0.82, 4.7, "image", 0.93, 8.4, 58, "CNY", 91),
      supplierItem("sup2", "alibaba", "Guangzhou Feiman Apparel Co.", "Men's pique polo, custom embroidery", 100, 4, 0.71, 4.5, "keyword", 0.85, 9.1, 9.1, "USD", 84),
      supplierItem("sup3", "eu_stock", "EuroTex Warehouse (kho NL)", "Polo cotton EU stock, ship 24h nội địa", 20, 3, 0.6, 4.3, "keyword", 0.7, 12.5, 12.5, "EUR", 78),
    ],
    negotiated_cogs: 8.6, logistics_fee: 1.9, lead_time_days: 4, sourcing_note: "Đã chốt MOQ 200, giá theo lô lớn.",
    quoted_by: "Mai — Sourcing", supplier_id: "sup1",
    playbook_source: "claude",
    playbook: {
      angle: "\"Polo công sở mềm mát cả ngày\" — nhấn vào chất liệu co giãn, không nhăn, phù hợp môi trường máy lạnh.",
      strengths: ["Điểm S2 cao nhất ngách nhờ khớp cả 2 nguồn dữ liệu", "Landed cost thấp hơn trần 3,36$, biên an toàn", "Rating 4.6★ với hơn 1.200 đánh giá — độ tin cậy cao"],
      weaknesses: ["Size chart cần ghi rõ hơn để giảm tỉ lệ đổi trả", "Cạnh tranh ở NL khá đông (9 shop cùng bán)"],
      actions: ["Chạy test creative video 15s nhấn chất vải trước khi scale ngân sách", "Thêm bảng size chi tiết bằng cm lẫn EU size trên landing page", "Đàm phán thêm MOQ 500 để hạ giá vốn xuống dưới 8$"],
    },
  },
  {
    id: "p_polo_2", rank: 2, name: "Áo polo nam tay dài công sở chống nhăn",
    image_url: nicheImg(2), matched_keyword: "polo shirt herren büro", match_status: "matched_both",
    match_bonus: 0.8, ad_status: "valid",
    ads_active_days: 48, ad_reach: 132000, ads_count: 9,
    ecom_platform: "amazon", revenue: 31000, currency: "EUR", sales_volume: 2600, shops_count: 7,
    rating: 4.4, reviews_count: 860, price: 27.5,
    aud_age_top: "35-44", aud_age_share: 0.31, aud_gender_top: "male", aud_gender_share: 0.68,
    aud_match: "match", aud_match_note: "Đúng nhóm tuổi và giới tính mục tiêu, thị trường chủ yếu là Đức.",
    aud_breakdown: { ages: [{ label: "18-24", share: 0.07 }, { label: "25-34", share: 0.28 },
      { label: "35-44", share: 0.31 }, { label: "45-54", share: 0.24 }, { label: "55+", share: 0.1 }],
      genders: [{ label: "male", share: 0.68 }, { label: "female", share: 0.32 }] },
    ad_countries: ["DE", "NL"],
    s21: 8.5, s22: 8.3, s2: 8.7,
    pros: ["Tay dài phù hợp mùa lạnh vẫn lịch sự", "Vải chống nhăn thật, không cần ủi"],
    cons: ["Giá hơi cao so với polo tay ngắn cùng loại"],
    pain_points: "Khách ở Đức/Hà Lan cần polo tay dài đi làm mùa thu-đông nhưng vẫn giữ vẻ smart-casual.",
    ad_reason: "Ảnh sản phẩm và mô tả \"strijkvrij / bügelfrei\" khớp giữa ads và trang Amazon.de.",
    shortlisted: true,
    p_min: 22.9, p_avg: 28.6, p_target: 31.9, p_max: 36.9, cogs_max: 12.76, cpa_target_max: 9.57,
    cpa_estimated: 8.1, ads_feasible: true, s31: 8.0, s32: 7.8, s3: 7.9, price_points: 4,
    pricing_source: "claude",
    pricing_note: "Dải giá 23-37€, chọn P_target 31,9€ để vẫn còn dư địa quảng cáo cho thị trường Đức vốn CPM cao hơn NL.",
    cpm_benchmark: 11.2, ctr_benchmark: 0.012, cpc_benchmark: 0.93,
    prices: [
      priceItem("pr6", "ecom", "Amazon.de", "amazon", "Herren Poloshirt langarm bügelfrei - 27,50€", 29.6, 27.5, "EUR", "regex", "https://amazon.de/dp/2"),
      priceItem("pr7", "ads_landing", "BüroMode DE", "landing", "Langarm Polo Business — 33,99€", 36.6, 33.99, "EUR", "claude", "https://buromode.de/polo-langarm"),
      priceItem("pr8", "ecom", "Otto.de", "otto", "Polo Shirt langarm Herren - 24,90€", 26.8, 24.9, "EUR", "regex", "https://otto.de/p/2"),
      priceItem("pr9", "ecom", "bol.com", "bol", "Heren poloshirt lange mouw - 25,95€", 27.9, 25.95, "EUR", "demo", "https://bol.com/p/2"),
    ],
    suppliers: [
      supplierItem("sup4", "1688", "Nanjing Ruili Textile", "Long sleeve pique polo, anti-wrinkle fabric", 80, 5, 0.75, 4.6, "image", 0.9, 9.4, 66, "CNY", 88),
      supplierItem("sup5", "taobao", "杭州凡凡服饰", "男士长袖POLO衫 商务免烫", 60, 3, 0.66, 4.4, "keyword", 0.8, 8.7, 62, "CNY", 80),
    ],
    negotiated_cogs: 9.4, logistics_fee: 2.1, lead_time_days: 5, sourcing_note: "MOQ 150, cần đặt cọc 30%.",
    quoted_by: "Mai — Sourcing", supplier_id: "sup4",
    playbook_source: "claude",
    playbook: {
      angle: "\"Không cần ủi, vẫn chỉn chu\" — tập trung thị trường Đức mùa thu-đông.",
      strengths: ["S2 cao thứ 2 ngách, dữ liệu khớp cả ads lẫn sàn", "Landed cost 11,5$ dưới trần 12,76$"],
      weaknesses: ["CPM thị trường Đức cao hơn NL ~15%", "Ít lựa chọn màu hơn đối thủ Otto.de"],
      actions: ["Test riêng ngân sách theo từng nước để so sánh CPA NL vs DE", "Thêm 2 màu ghi/đen để đa dạng hoá catalogue"],
    },
  },
  {
    id: "p_polo_3", rank: 3, name: "Bộ áo polo + quần kaki set công sở",
    image_url: nicheImg(3), matched_keyword: "smart polo shirt men", match_status: "ecom_only",
    match_bonus: 0, ad_status: "valid",
    ads_active_days: null, ad_reach: null, ads_count: 0,
    ecom_platform: "etsy", revenue: 18500, currency: "EUR", sales_volume: 1150, shops_count: 4,
    rating: 4.7, reviews_count: 410, price: 54.0,
    aud_age_top: null, aud_age_share: null, aud_gender_top: null, aud_gender_share: null,
    aud_match: "unknown", aud_match_note: "Chưa có ads chạy trả phí nên chưa xác định được tệp thực tế.",
    aud_breakdown: null, ad_countries: [],
    s21: null, s22: 8.8, s2: 8.4,
    pros: ["Mua trọn set tiện lợi, phối đồ nhanh buổi sáng", "Chất liệu kaki co giãn nhẹ"],
    cons: ["Giá set khá cao so với mua lẻ", "Không lẻ size cho từng món"],
    pain_points: "Dân văn phòng bận rộn muốn có set đồng bộ mặc ngay, khỏi mất thời gian phối đồ.",
    ad_reason: null,
    shortlisted: true,
    p_min: 42.0, p_avg: 55.0, p_target: 62.0, p_max: 69.0, cogs_max: 24.8, cpa_target_max: 18.6,
    cpa_estimated: null, ads_feasible: null, s31: 8.6, s32: 7.4, s3: 8.0, price_points: 3,
    pricing_source: "claude",
    pricing_note: "Chưa có ads nên P_target dựa hoàn toàn vào dải giá sàn Etsy — vẫn còn nhiều dư địa vì đây là SKU set combo.",
    cpm_benchmark: null, ctr_benchmark: null, cpc_benchmark: null,
    prices: [
      priceItem("pr10", "ecom", "Etsy", "etsy", "Men's Polo + Chino Set, office ready - 54,00€", 58.1, 54.0, "EUR", "regex", "https://etsy.com/listing/3"),
      priceItem("pr11", "ecom", "Etsy", "etsy", "Smart Casual Set for Men - 65,00€", 69.9, 65.0, "EUR", "regex", "https://etsy.com/listing/4"),
      priceItem("pr12", "ecom", "Not On The High Street", "amazon", "Business Casual Bundle - 45,00€", 48.4, 45.0, "GBP", "demo", "https://notonthehighstreet.com/p/3"),
    ],
    suppliers: [
      supplierItem("sup6", "alibaba", "Xiamen Trendy Wear", "Polo + chino trousers combo set OEM", 30, 2, 0.55, 4.2, "keyword", 0.68, 21.5, 21.5, "USD", 70),
    ],
    negotiated_cogs: null, logistics_fee: null, lead_time_days: null, sourcing_note: null,
    quoted_by: null, supplier_id: null,
    playbook_source: "claude",
    playbook: {
      angle: "\"Mặc là đi làm ngay\" — set đồng bộ polo + kaki, tiết kiệm thời gian buổi sáng.",
      strengths: ["Biên lợi nhuận cao vì bán theo set (AOV cao)", "Chưa bị cạnh tranh ads, còn trống thị trường paid"],
      weaknesses: ["Chưa có dữ liệu ads để ước lượng CPA thực tế", "Rủi ro tồn kho nếu lệch tỉ lệ size áo/quần"],
      actions: ["Chạy thử nghiệm ads đầu tiên với ngân sách nhỏ để có benchmark CPA", "Đàm phán MOQ tách rời áo/quần để giảm rủi ro tồn kho lệch size"],
    },
  },
  {
    id: "p_polo_4", rank: 4, name: "Áo polo nam form slimfit thêu logo tối giản",
    image_url: nicheImg(4), matched_keyword: "heren poloshirt business", match_status: "matched_both",
    match_bonus: 0.6, ad_status: "valid",
    ads_active_days: 35, ad_reach: 88000, ads_count: 7,
    ecom_platform: "aliexpress", revenue: 15200, currency: "EUR", sales_volume: 2100, shops_count: 11,
    rating: 4.2, reviews_count: 1560, price: 16.9,
    aud_age_top: "25-34", aud_age_share: 0.4, aud_gender_top: "male", aud_gender_share: 0.77,
    aud_match: "partial", aud_match_note: "Tệp ads hơi trẻ hơn (đông 18-24) so với tệp mục tiêu 25-44 của Bước 1.",
    aud_breakdown: { ages: [{ label: "18-24", share: 0.22 }, { label: "25-34", share: 0.4 },
      { label: "35-44", share: 0.24 }, { label: "45-54", share: 0.1 }, { label: "55+", share: 0.04 }],
      genders: [{ label: "male", share: 0.77 }, { label: "female", share: 0.23 }] },
    ad_countries: ["NL", "BE"],
    s21: 7.6, s22: 7.4, s2: 8.0,
    pros: ["Giá rẻ, dễ mua thử", "Nhiều màu lựa chọn"],
    cons: ["Chất vải mỏng hơn kỳ vọng so với giá 27€ trên landing", "Form slimfit hơi bó với người tầm 80kg+"],
    pain_points: "Khách trẻ mới đi làm muốn polo giá rẻ nhưng vẫn nhìn \"đủ lịch sự\" để mặc công sở.",
    ad_reason: "Cùng logo thêu nhỏ ở ngực trái và cùng bảng 6 màu giữa ads và gian hàng AliExpress.",
    shortlisted: true,
    p_min: 12.9, p_avg: 19.4, p_target: 22.9, p_max: 27.0, cogs_max: 9.16, cpa_target_max: 6.87,
    cpa_estimated: 7.9, ads_feasible: false, s31: 6.8, s32: 6.2, s3: 6.6, price_points: 6,
    pricing_source: "claude",
    pricing_note: "CPA ước tính đã vượt trần vì giá bán thấp — cần tăng AOV bằng combo 2 áo hoặc nâng giá bán lên 25-26€.",
    cpm_benchmark: 9.1, ctr_benchmark: 0.009, cpc_benchmark: 0.99,
    prices: [
      priceItem("pr13", "ecom", "AliExpress", "aliexpress", "Men Slim Fit Polo Embroidery Logo - 16,90€", 18.2, 16.9, "EUR", "regex", "https://aliexpress.com/item/4"),
      priceItem("pr14", "ads_landing", "MinimalMenswear", "landing", "Slimfit Polo — was 34,99 now 24,99€", 26.9, 24.99, "EUR", "claude", "https://minimalmenswear.com/polo"),
      priceItem("pr15", "ecom", "Shopee SG", "shopee", "Men Polo Slimfit Embroidery - S$18.90", 14.0, 18.9, "SGD", "demo", "https://shopee.sg/p/4"),
    ],
    suppliers: [
      supplierItem("sup7", "1688", "Foshan Xinyu Clothing", "Slimfit polo embroidery logo custom", 100, 7, 0.79, 4.5, "image", 0.88, 5.4, 38, "CNY", 85),
      supplierItem("sup8", "taobao", "东莞简约服饰厂", "男士修身POLO刺绣logo定制", 200, 5, 0.7, 4.3, "keyword", 0.76, 4.9, 35, "CNY", 79),
    ],
    negotiated_cogs: 5.6, logistics_fee: 1.4, lead_time_days: 6, sourcing_note: "Đã âm giá xuống dưới trần nhưng CPA vẫn là rủi ro chính.",
    quoted_by: "Huy — Sourcing", supplier_id: "sup7",
    playbook_source: "claude",
    playbook: {
      angle: "\"Polo thêu logo tối giản, giá sinh viên mới đi làm\" — định vị entry-level trong ngách.",
      strengths: ["Giá vốn rất thấp, dễ đàm phán thêm", "Số lượng ads + shop cao cho thấy nhu cầu thật"],
      weaknesses: ["CPA ước tính vượt trần — biên lợi nhuận mỏng", "Tệp ads lệch trẻ hơn tệp mục tiêu"],
      actions: ["Test bán combo 2 áo để tăng AOV, kéo CPA về dưới trần", "Thắt lại targeting về đúng nhóm 25-44 tuổi"],
    },
  },
  {
    id: "p_polo_5", rank: 5, name: "Áo polo nữ công sở phối màu thanh lịch",
    image_url: nicheImg(5), matched_keyword: "smart polo shirt men", match_status: "ads_only",
    match_bonus: 0, ad_status: "valid",
    ads_active_days: 29, ad_reach: 61000, ads_count: 5,
    ecom_platform: null, revenue: null, currency: "EUR", sales_volume: null, shops_count: 0,
    rating: null, reviews_count: null, price: null,
    aud_age_top: "25-34", aud_age_share: 0.29, aud_gender_top: "female", aud_gender_share: 0.83,
    aud_match: "partial", aud_match_note: "Ads đang target nữ giới — lệch khỏi persona nam công sở ban đầu nhưng vẫn cùng ngách polo công sở.",
    aud_breakdown: { ages: [{ label: "18-24", share: 0.12 }, { label: "25-34", share: 0.29 },
      { label: "35-44", share: 0.33 }, { label: "45-54", share: 0.2 }, { label: "55+", share: 0.06 }],
      genders: [{ label: "female", share: 0.83 }, { label: "male", share: 0.17 }] },
    ad_countries: ["NL", "DE"],
    s21: 7.2, s22: null, s2: 7.6,
    pros: [], cons: [],
    pain_points: "Chưa có dữ liệu review vì chưa tìm thấy sản phẩm khớp trên sàn TMĐT.",
    ad_reason: "Không tìm thấy sản phẩm khớp ảnh/tên trên các sàn đã quét.",
    shortlisted: true,
    p_min: null, p_avg: null, p_target: 27.9, p_max: null, cogs_max: 11.16, cpa_target_max: 8.37,
    cpa_estimated: 6.9, ads_feasible: true, s31: 6.0, s32: null, s3: 7.0, price_points: 1,
    pricing_source: "claude",
    pricing_note: "Chỉ có 1 điểm giá từ landing ads — dùng tạm để ước tính P_target, cần thêm dữ liệu trước khi scale.",
    cpm_benchmark: 10.5, ctr_benchmark: 0.016, cpc_benchmark: 0.66,
    prices: [
      priceItem("pr16", "ads_landing", "LadyOffice Store", "landing", "Dames poloshirt kantoor — 27,90€", 30.0, 27.9, "EUR", "claude", "https://ladyoffice.nl/polo"),
    ],
    suppliers: [],
    negotiated_cogs: null, logistics_fee: null, lead_time_days: null, sourcing_note: null,
    quoted_by: null, supplier_id: null,
    playbook_source: "claude",
    playbook: {
      angle: "\"Bản nữ của polo công sở\" — mở rộng ngách sang khách hàng nữ chưa được khai thác.",
      strengths: ["Khoảng trống thị trường — chưa thấy shop nào bán trên sàn đã quét"],
      weaknesses: ["Chưa xác thực được nguồn hàng vì thiếu dữ liệu sàn TMĐT", "Chỉ có 1 điểm giá tham chiếu, độ tin cậy thấp"],
      actions: ["Tìm thủ công 2-3 nhà cung cấp polo nữ trên 1688 để xác thực trần giá vốn", "Chạy thêm 1-2 tuần thu thập ads để có nhiều điểm giá hơn"],
    },
  },
  {
    id: "p_polo_6", rank: 6, name: "Áo polo nam cotton lạnh chống nhăn cao cấp",
    image_url: nicheImg(6), matched_keyword: "poloshirt heren katoen strijkvrij", match_status: "matched_both",
    match_bonus: 0.5, ad_status: "valid",
    ads_active_days: 21, ad_reach: 44000, ads_count: 4,
    ecom_platform: "bol", revenue: 9800, currency: "EUR", sales_volume: 540, shops_count: 3,
    rating: 4.8, reviews_count: 190, price: 32.0,
    aud_age_top: "35-44", aud_age_share: 0.36, aud_gender_top: "male", aud_gender_share: 0.74,
    aud_match: "match", aud_match_note: "Tệp ads khớp tốt nhóm tuổi trung niên, đúng khách hàng mục tiêu cao cấp.",
    aud_breakdown: { ages: [{ label: "25-34", share: 0.21 }, { label: "35-44", share: 0.36 },
      { label: "45-54", share: 0.28 }, { label: "55+", share: 0.15 }],
      genders: [{ label: "male", share: 0.74 }, { label: "female", share: 0.26 }] },
    ad_countries: ["NL"],
    s21: 8.1, s22: 8.4, s2: 7.3,
    pros: ["Vải cotton lạnh mát tay, cao cấp hơn mặt bằng chung", "Đường may chắc chắn, không xù lông sau giặt"],
    cons: ["Giá cao nên đơn hàng còn ít", "Chỉ có 3 màu cơ bản"],
    pain_points: "Nhóm khách trung niên sẵn sàng chi nhiều hơn cho chất lượng nhưng khó tìm polo \"cao cấp thật\" giữa rừng hàng giá rẻ.",
    ad_reason: "Cùng nhãn vải \"cotton lạnh cao cấp\" và mức giá 30-34€ giữa ads và bol.com.",
    shortlisted: true,
    p_min: 28.0, p_avg: 33.5, p_target: 36.9, p_max: 42.0, cogs_max: 14.76, cpa_target_max: 11.07,
    cpa_estimated: null, ads_feasible: null, s31: 8.8, s32: null, s3: 7.9, price_points: 2,
    pricing_source: "claude",
    pricing_note: "Dữ liệu ads còn ít (4 ads) nên benchmark CPA chưa đủ tin cậy — P_target dựa chủ yếu vào dải giá sàn.",
    cpm_benchmark: null, ctr_benchmark: null, cpc_benchmark: null,
    prices: [
      priceItem("pr17", "ecom", "bol.com", "bol", "Premium heren poloshirt koelkatoen - 32,00€", 34.4, 32.0, "EUR", "regex", "https://bol.com/p/6"),
      priceItem("pr18", "ads_landing", "Atelier Heren", "landing", "Cooling Cotton Polo — 39,95€", 43.0, 39.95, "EUR", "claude", "https://atelierheren.nl/polo"),
    ],
    suppliers: [
      supplierItem("sup9", "alibaba", "Suzhou Coolfeel Textile", "Cooling cotton polo premium finish", 50, 8, 0.85, 4.8, "image", 0.91, 13.9, 13.9, "USD", 92),
    ],
    negotiated_cogs: null, logistics_fee: null, lead_time_days: null, sourcing_note: null,
    quoted_by: null, supplier_id: null,
    playbook_source: "claude",
    playbook: {
      angle: "\"Polo cao cấp cho dân văn phòng khó tính\" — định vị premium, giá cao hơn mặt bằng chung.",
      strengths: ["Rating cao nhất ngách (4.8★)", "Biên lợi nhuận tốt nhờ định vị premium"],
      weaknesses: ["Chưa đàm phán được nhà cung cấp — s4 chưa có điểm", "Dữ liệu ads mỏng, benchmark CPA chưa chắc chắn"],
      actions: ["Đội mua hàng liên hệ Suzhou Coolfeel để lấy báo giá thực tế", "Chạy thêm creative để tăng dữ liệu ads trước khi scale"],
    },
  },
  {
    id: "p_polo_7", rank: 7, name: "Áo polo nam basic combo 5 màu",
    image_url: nicheImg(7), matched_keyword: "smart polo shirt men", match_status: "matched_both",
    match_bonus: 0.3, ad_status: "valid",
    ads_active_days: 18, ad_reach: 30000, ads_count: 3,
    ecom_platform: "aliexpress", revenue: 6400, currency: "EUR", sales_volume: 890, shops_count: 6,
    rating: 3.9, reviews_count: 720, price: 21.0,
    aud_age_top: "18-24", aud_age_share: 0.3, aud_gender_top: "male", aud_gender_share: 0.66,
    aud_match: "mismatch", aud_match_note: "Ads đang tiếp cận nhóm tuổi trẻ hơn hẳn tệp mục tiêu 25-44 của ngách.",
    aud_breakdown: { ages: [{ label: "18-24", share: 0.3 }, { label: "25-34", share: 0.27 },
      { label: "35-44", share: 0.2 }, { label: "45-54", share: 0.15 }, { label: "55+", share: 0.08 }],
      genders: [{ label: "male", share: 0.66 }, { label: "female", share: 0.34 }] },
    ad_countries: ["NL"],
    s21: 6.4, s22: 6.0, s2: 7.0,
    pros: ["Giá combo rẻ, mua 5 áo tiện thay đổi"],
    cons: ["Chất vải khá mỏng, rating chỉ 3.9★", "Đường chỉ ở cổ dễ bung sau vài lần giặt"],
    pain_points: "Khách muốn nhiều lựa chọn màu giá rẻ, sẵn sàng đánh đổi chất lượng.",
    ad_reason: "Cùng ảnh combo 5 màu và mô tả sản phẩm giữa ads và gian hàng AliExpress.",
    shortlisted: false,
    playbook_source: "claude",
    playbook: {
      angle: "\"5 màu dùng cả tuần\" — định vị giá rẻ, số lượng nhiều.",
      strengths: ["Giá vốn thấp, dễ tiếp cận khách mới"],
      weaknesses: ["Tệp ads lệch hẳn khỏi persona mục tiêu", "Rating dưới 4.0 — rủi ro hoàn trả cao"],
      actions: ["Không ưu tiên scale — cần cải thiện chất lượng vải trước", "Nếu tiếp tục, thắt lại targeting về đúng 25-44 tuổi"],
    },
  },
  {
    id: "p_polo_8", rank: 8, name: "Áo polo nam cổ bẻ big size 2XL-4XL",
    image_url: nicheImg(8), matched_keyword: "polo shirt herren büro", match_status: "ecom_only",
    match_bonus: 0, ad_status: "irrelevant",
    ads_active_days: 12, ad_reach: 9000, ads_count: 2,
    ecom_platform: "walmart", revenue: 4100, currency: "USD", sales_volume: 320, shops_count: 2,
    rating: 4.1, reviews_count: 260, price: 19.99,
    aud_age_top: null, aud_age_share: null, aud_gender_top: null, aud_gender_share: null,
    aud_match: "unknown", aud_match_note: "Ads bị AI lọc vì quảng cáo dịch vụ may đo, không phải sản phẩm may sẵn.",
    aud_breakdown: null, ad_countries: [],
    s21: null, s22: 6.6, s2: 6.6,
    pros: ["Có size lớn hiếm nơi khác bán"], cons: ["Ít mẫu mã, chỉ 3 màu"],
    pain_points: "Khách ngoại cỡ khó tìm polo công sở vừa vặn.",
    ad_reason: "Ads quảng cáo dịch vụ may đo theo yêu cầu — AI đánh giá sai ngách, không tính vào ghép nối.",
    shortlisted: false,
    playbook_source: "claude",
    playbook: {
      angle: "\"Big size không lo chật\" — ngách nhỏ nhưng ít cạnh tranh.",
      strengths: ["Ít đối thủ khai thác size lớn"],
      weaknesses: ["Dữ liệu ads bị lọc vì sai ngách — thiếu benchmark thật", "Thị trường US, khác 3 nước mục tiêu ban đầu"],
      actions: ["Tìm lại từ khoá ads đúng ngách \"big and tall polo\" thay vì may đo", "Xác nhận lại thị trường mục tiêu trước khi đầu tư thêm"],
    },
  },
  {
    id: "p_polo_9", rank: 9, name: "Áo polo nam thêu tên theo yêu cầu (custom)",
    image_url: nicheImg(9), matched_keyword: "heren poloshirt business", match_status: "ads_only",
    match_bonus: 0, ad_status: "policy_risk",
    ads_active_days: 8, ad_reach: 15000, ads_count: 2,
    ecom_platform: null, revenue: null, currency: "EUR", sales_volume: null, shops_count: 0,
    rating: null, reviews_count: null, price: null,
    aud_age_top: "25-34", aud_age_share: 0.33, aud_gender_top: "male", aud_gender_share: 0.7,
    aud_match: "unknown", aud_match_note: "Ads bị gắn cờ rủi ro chính sách (cam kết \"giao trong 24h\" không kiểm chứng được).",
    aud_breakdown: null, ad_countries: ["NL"],
    s21: null, s22: null, s2: 6.1,
    pros: [], cons: [],
    pain_points: "Chưa đủ dữ liệu vì ads bị lọc do rủi ro chính sách quảng cáo.",
    ad_reason: "Nội dung ads cam kết thời gian giao hàng không kiểm chứng được, dễ vi phạm chính sách Meta.",
    shortlisted: false,
    playbook_source: "claude",
    playbook: {
      angle: "\"Polo thêu tên cá nhân hoá\" — tiềm năng nhưng ads hiện tại có rủi ro chính sách.",
      strengths: ["Yếu tố cá nhân hoá tạo khác biệt so với polo đại trà"],
      weaknesses: ["Ads bị AI lọc vì rủi ro chính sách", "Chưa có dữ liệu sàn TMĐT để đối chiếu"],
      actions: ["Viết lại ad copy bỏ cam kết thời gian giao hàng cụ thể", "Tìm thêm ads/sản phẩm khác trong sub-ngách cá nhân hoá"],
    },
  },
];

/* Sinh score_breakdown + total_score cho từng sản phẩm demo dựa trên dữ liệu đã có */
function nicheBuildSummary(session, p) {
  const seasonality = session.step1_score;
  const audienceQ = (NICHE_AUDIENCE_DEMO.find((a) => a.platform === "meta") || {}).quality;
  const audQuality = audienceQ ? audienceQ.score : null;
  const adsMatchScore = { match: 9, partial: 6, mismatch: 3, unknown: null }[p.aud_match || "unknown"];
  const appeal = p.s2 != null ? nicheRound1(p.s2) : null;
  const priceMargin = (p.s31 != null && p.s32 != null) ? nicheRound1((p.s31 + p.s32) / 2)
    : (p.s31 != null ? p.s31 : null);
  const sourcing = p.s4 != null ? p.s4 : null;
  const bd = scoreCriteria({
    seasonality, audience_quality: audQuality, ads_match: adsMatchScore,
    appeal, price_margin: priceMargin, sourcing,
  });
  p.score_breakdown = bd;
  p.total_score = totalFromBreakdown(bd);
  return p;
}

/* ====================================================================
   3. Phiên demo hoàn chỉnh
   ==================================================================== */
const NICHE_SESSIONS = [
  {
    id: DEMO_SID,
    raw_keyword: "áo polo nam công sở",
    countries: [{ code: "NL", name: "Hà Lan" }, { code: "DE", name: "Đức" }, { code: "BE", name: "Bỉ" }],
    country_code: "NL", country_name: "Hà Lan",
    status: "done", progress: 100, stage: "Hoàn tất", message: null,
    step1_score: 8.1,
    avg_monthly_searches: 10125,
    demand_type: "evergreen", demand_label: "Nhu cầu ổn định quanh năm — evergreen",
    kw_source: "claude", volume_source: "demo", ai_source: "claude",
    peak_months: [8, 9], low_months: [7],
    trend_direction: "stable", volatility: "low", volatility_label: "Biến động thấp",
    fluctuation_ratio: 0.28, cv: 0.18,
    seasonality_note: "Nhu cầu tăng nhẹ vào tháng 8-9 (mùa quay lại văn phòng sau hè) và chững lại tháng 7 (cao điểm nghỉ hè châu Âu). Nhìn chung đây là ngách evergreen, phù hợp chạy ads quanh năm thay vì chỉ tập trung 1-2 đợt cao điểm.",
    ai_summary: "Ngách áo polo công sở nam có dung lượng tìm kiếm ổn định ở cả 3 thị trường NL/DE/BE, ít phụ thuộc mùa vụ — phù hợp xây kênh dài hạn hơn là đánh nhanh rút gọn.",
    ai_risks: [
      "Cạnh tranh cao ở phân khúc giá rẻ (dưới 20€) do nhiều seller AliExpress tham gia.",
      "CPM thị trường Đức cao hơn Hà Lan khoảng 15%, cần tách ngân sách theo nước.",
      "Một số ads bị gắn cờ rủi ro chính sách vì cam kết thời gian giao hàng — cần rà lại ad copy.",
    ],
    ai_actions: [
      "Ưu tiên scale các SKU đã khớp cả 2 nguồn dữ liệu (matched_both) trước.",
      "Tách creative riêng cho nhóm khách nữ (SKU polo nữ) để test thị trường ngách phụ.",
      "Chốt nhà cung cấp cho 2-3 SKU đầu bảng trước khi mở rộng thêm SKU mới.",
    ],
    cogs_share: 0.38, ads_share: 0.32,
    step2_status: "done", step2_progress: 100, step2_stage: "Hoàn tất", step2_message: null,
    step2_score: 9.1, last_step2_at: nicheNowIso(60 * 5),
    ads_source: "meta_ad_library", ecom_source: "multi_platform",
    step3_status: "done", step3_progress: 100, step3_stage: "Hoàn tất",
    step3_message: "2/9 SKU không lấy được đủ điểm giá đối thủ (ít hơn 2 link khả dụng) — số liệu các SKU này mang tính tham khảo.",
    step3_score: 8.3, last_step3_at: nicheNowIso(60 * 3),
    price_source: "scraper", benchmark_source: "meta_api",
    step4_status: "done", step4_progress: 100, step4_stage: "Hoàn tất", step4_message: null,
    step4_score: 8.4, last_step4_at: nicheNowIso(60 * 1),
    warehouse: "EU", target_moq: 50,
    last_run_at: nicheNowIso(60 * 8),
  },
];

const NICHE_PRODUCTS = { [DEMO_SID]: NICHE_PRODUCTS_DEMO };
const NICHE_KEYWORDS = { [DEMO_SID]: NICHE_KEYWORDS_DEMO };
const NICHE_AUDIENCE = { [DEMO_SID]: NICHE_AUDIENCE_DEMO };
const NICHE_SEGMENTS = { [DEMO_SID]: JSON.parse(JSON.stringify(NICHE_SEGMENTS_DEMO)) };

// tính điểm tổng ban đầu cho toàn bộ SKU demo (giả lập đã bấm "Chấm điểm tổng")
NICHE_PRODUCTS_DEMO.forEach((p) => nicheBuildSummary(NICHE_SESSIONS[0], p));

/* ====================================================================
   4. Sinh dữ liệu cho phiên MỚI do người dùng tự tạo trong lúc demo
   ==================================================================== */
function nicheGenericProducts(session) {
  const cur = NICHE_COUNTRY_CURRENCY[(session.countries[0] || {}).code] || "EUR";
  const names = [
    "Bản phối màu chủ đạo", "Phiên bản vải cao cấp", "Combo tiết kiệm 2 món",
    "Bản form rộng thoải mái", "Phiên bản size lớn", "Bản basic giá tốt",
    "Phiên bản thêu logo tối giản", "Bản giới hạn theo mùa",
  ];
  const platforms = ["aliexpress", "bol", "etsy", "amazon", "allegro", "shopee"];
  return names.map((suffix, i) => {
    const id = `p_${session.id}_${i + 1}`;
    const s2 = nicheRound1(8.6 - i * 0.45);
    const rating = nicheRound1(4.7 - i * 0.08);
    const price = nicheRound1(15 + i * 3.4);
    return {
      id, rank: i + 1, name: `${session.raw_keyword} — ${suffix}`,
      image_url: nicheImg(`${session.id}-${i + 1}`), matched_keyword: session.raw_keyword,
      match_status: i % 3 === 0 ? "matched_both" : i % 3 === 1 ? "ads_only" : "ecom_only",
      match_bonus: i % 3 === 0 ? 0.6 : 0, ad_status: "valid",
      ads_active_days: 15 + i * 6, ad_reach: 90000 - i * 8000, ads_count: 8 - i,
      ecom_platform: platforms[i % platforms.length], revenue: 20000 - i * 1800, currency: cur,
      sales_volume: 2000 - i * 150, shops_count: 8 - i, rating, reviews_count: 900 - i * 70, price,
      aud_age_top: i % 2 === 0 ? "25-34" : "35-44", aud_age_share: 0.3, aud_gender_top: "male",
      aud_gender_share: 0.65, aud_match: i % 3 === 0 ? "match" : "partial",
      aud_match_note: "Ước tính từ dữ liệu demo — chưa có ads thật cho phiên này.",
      aud_breakdown: { ages: [{ label: "18-24", share: 0.15 }, { label: "25-34", share: 0.32 },
        { label: "35-44", share: 0.28 }, { label: "45-54", share: 0.17 }, { label: "55+", share: 0.08 }],
        genders: [{ label: "male", share: 0.65 }, { label: "female", share: 0.35 }] },
      ad_countries: session.countries.map((c) => c.code),
      s21: nicheRound1(s2 - 0.2), s22: nicheRound1(s2 - 0.4), s2,
      pros: ["Chất lượng ổn trong tầm giá", "Ảnh sản phẩm rõ ràng, dễ tạo creative"],
      cons: ["Chưa có nhiều review để đánh giá độ bền"],
      pain_points: "Khách hàng trong ngách này ưu tiên sự tiện lợi và giá hợp lý.",
      ad_reason: "Ghép nối tự động theo cụm ảnh & từ khoá (dữ liệu demo).",
      shortlisted: i < 5,
    };
  });
}

function nicheEnsureProducts(sessionId) {
  if (!NICHE_PRODUCTS[sessionId]) {
    const s = NICHE_SESSIONS.find((x) => x.id === sessionId);
    NICHE_PRODUCTS[sessionId] = s ? nicheGenericProducts(s) : [];
  }
  return NICHE_PRODUCTS[sessionId];
}
function nicheEnsureKeywords(sessionId) {
  if (!NICHE_KEYWORDS[sessionId]) {
    const s = NICHE_SESSIONS.find((x) => x.id === sessionId);
    const kw = s ? s.raw_keyword : "sản phẩm ngách";
    NICHE_KEYWORDS[sessionId] = [
      { id: uid("kw"), keyword: kw + " (từ khoá bản địa 1)", translation: kw,
        avg_monthly_searches: 6200, monthly_volumes: nicheMonthly(6200, [10, 11], 0.3) },
      { id: uid("kw"), keyword: kw + " (từ khoá bản địa 2)", translation: kw,
        avg_monthly_searches: 3100, monthly_volumes: nicheMonthly(3100, [10, 11], 0.25) },
    ];
  }
  return NICHE_KEYWORDS[sessionId];
}
function nicheEnsureSegments(sessionId) {
  if (!NICHE_SEGMENTS[sessionId]) {
    const s = NICHE_SESSIONS.find((x) => x.id === sessionId);
    const countries = (s && s.countries) || [{ code: "NL", name: "Hà Lan" }];
    NICHE_SEGMENTS[sessionId] = {
      country: countries.map((c) => ({ id: uid("seg"), kind: "country", label: c.name,
        external_id: c.code, size_upper: 10000000, selected: true })),
      age: [
        { id: uid("seg"), kind: "age", label: "25-34", size_upper: 1800000, selected: true },
        { id: uid("seg"), kind: "age", label: "35-44", size_upper: 1500000, selected: true },
      ],
      gender: [
        { id: uid("seg"), kind: "gender", label: "Nam", size_upper: 20000000, selected: true },
        { id: uid("seg"), kind: "gender", label: "Nữ", size_upper: 20500000, selected: false },
      ],
      interest: [
        { id: uid("seg"), kind: "interest", label: "Online shoppers", label_vi: "Mua sắm online",
          size_upper: 4000000, share: 0.04, selected: true },
      ],
      behavior: [], demographic: [],
    };
  }
  return NICHE_SEGMENTS[sessionId];
}
function nicheEnsureAudience(sessionId) {
  if (!NICHE_AUDIENCE[sessionId]) {
    NICHE_AUDIENCE[sessionId] = [
      { platform: "meta", lower_bound: 400000, upper_bound: 700000, verdict: "ideal",
        note: "Ước tính demo dựa trên các lát cắt mặc định.", source: "demo",
        quality: nicheAudienceQuality() },
    ];
  }
  return NICHE_AUDIENCE[sessionId];
}

/* ====================================================================
   5. Hàm nghiệp vụ dùng chung cho route handler
   ==================================================================== */
function nicheFindSession(id) {
  const s = NICHE_SESSIONS.find((x) => x.id === id);
  if (!s) throw new Error("Không tìm thấy phiên nghiên cứu ngách");
  return s;
}
function nicheRunStep1(session) {
  session.status = "done"; session.progress = 100; session.stage = "Hoàn tất"; session.message = null;
  session.step1_score = session.step1_score || nicheRound1(6.5 + Math.random() * 2.5);
  session.avg_monthly_searches = session.avg_monthly_searches
    || Math.round(2000 + Math.random() * 12000);
  session.demand_type = session.demand_type || "evergreen";
  session.demand_label = session.demand_label || "Nhu cầu ổn định quanh năm — evergreen";
  session.kw_source = "claude"; session.volume_source = "demo"; session.ai_source = "claude";
  session.peak_months = session.peak_months || [11, 12];
  session.low_months = session.low_months || [7];
  session.trend_direction = session.trend_direction || "stable";
  session.volatility = session.volatility || "low";
  session.volatility_label = session.volatility_label || "Biến động thấp";
  session.fluctuation_ratio = session.fluctuation_ratio ?? 0.3;
  session.cv = session.cv ?? 0.2;
  session.seasonality_note = session.seasonality_note
    || "Số liệu demo: nhu cầu khá ổn định quanh năm, không có đợt tăng đột biến rõ rệt.";
  session.ai_summary = session.ai_summary || "Đây là dữ liệu demo do chưa cấu hình Google Ads API / Meta token thật.";
  session.ai_risks = session.ai_risks || ["Dữ liệu hiện là demo — cần cấu hình API thật trước khi ra quyết định lớn."];
  session.ai_actions = session.ai_actions || ["Chạy thử với ngân sách nhỏ trước khi scale."];
  nicheEnsureKeywords(session.id);
  nicheEnsureSegments(session.id);
  nicheEnsureAudience(session.id);
  session.last_run_at = nicheNowIso(0);
  return session;
}
function nicheRunStep2(session) {
  session.step2_status = "done"; session.step2_progress = 100; session.step2_stage = "Hoàn tất";
  session.step2_message = null;
  const products = nicheEnsureProducts(session.id);
  session.step2_score = Math.max(...products.map((p) => p.s2 || 0));
  session.ads_source = "meta_ad_library"; session.ecom_source = "multi_platform";
  session.last_step2_at = nicheNowIso(0);
  return session;
}
function nicheRunStep3(session) {
  const products = nicheEnsureProducts(session.id);
  const shortlisted = products.filter((p) => p.shortlisted);
  shortlisted.forEach((p, i) => {
    if (p.p_target != null) return; // giữ nguyên nếu đã có (phiên demo)
    const cogsPct = session.cogs_share ?? 0.4, adsPct = session.ads_share ?? 0.3;
    const pMin = nicheRound1(p.price ? p.price * 0.9 : 15 + i * 2);
    const pMax = nicheRound1(pMin * 1.6);
    const pAvg = nicheRound1((pMin + pMax) / 2);
    const pTarget = nicheRound1(pAvg * 1.1);
    p.p_min = pMin; p.p_avg = pAvg; p.p_target = pTarget; p.p_max = pMax;
    p.cogs_max = nicheRound1(pTarget * cogsPct);
    p.cpa_target_max = nicheRound1(pTarget * adsPct);
    p.cpm_benchmark = nicheRound1(8 + Math.random() * 4);
    p.ctr_benchmark = nicheRound1(0.01 + Math.random() * 0.01);
    p.cpc_benchmark = nicheRound1(0.5 + Math.random() * 0.6);
    p.cpa_estimated = nicheRound1(p.cpc_benchmark / 0.02);
    p.ads_feasible = p.cpa_estimated <= p.cpa_target_max;
    p.s31 = nicheRound1(6 + Math.random() * 3);
    p.s32 = nicheRound1(6 + Math.random() * 3);
    p.s3 = nicheRound1((p.s31 + p.s32) / 2);
    p.price_points = 2;
    p.pricing_source = "claude";
    p.pricing_note = "Số liệu demo — ước tính từ mức giá niêm yết của sản phẩm.";
    p.prices = [
      priceItem(uid("pr"), "ecom", "Shop demo", p.ecom_platform || "aliexpress",
        `${p.name} - ${p.price || pMin}`, pAvg, p.price || pAvg, p.currency || "EUR", "demo", "#"),
      priceItem(uid("pr"), "ads_landing", "Landing demo", "landing",
        `${p.name} — giá ưu đãi`, pMax, pMax, p.currency || "EUR", "demo", "#"),
    ];
  });
  session.step3_status = "done"; session.step3_progress = 100; session.step3_stage = "Hoàn tất";
  session.step3_message = null;
  session.step3_score = shortlisted.length ? Math.max(...shortlisted.map((p) => p.s3 || 0)) : null;
  session.price_source = "demo"; session.benchmark_source = "reference";
  session.last_step3_at = nicheNowIso(0);
  return session;
}
function nicheRunStep4(session, warehouse, targetMoq) {
  const products = nicheEnsureProducts(session.id).filter((p) => p.shortlisted);
  products.forEach((p) => {
    if (p.suppliers && p.suppliers.length) return;
    p.suppliers = [
      supplierItem(uid("sup"), "1688", "NCC demo 1688", p.name + " OEM", 100, 4, 0.7, 4.4,
        "image", 0.85, nicheRound1((p.cogs_max || 10) * 0.6), nicheRound1((p.cogs_max || 10) * 4.2),
        "CNY", 82),
      supplierItem(uid("sup"), "alibaba", "NCC demo Alibaba", p.name + " wholesale", 200, 3, 0.6, 4.2,
        "keyword", 0.75, nicheRound1((p.cogs_max || 10) * 0.7), nicheRound1((p.cogs_max || 10) * 0.7),
        "USD", 76),
    ];
    p.listed_cogs_min = Math.min(...p.suppliers.map((x) => x.listed_cogs_usd));
    p.listed_cogs_avg = nicheRound1(p.suppliers.reduce((s, x) => s + x.listed_cogs_usd, 0) / p.suppliers.length);
    p.suppliers_count = p.suppliers.length;
    p.s4_est = nicheRound1(p.listed_cogs_avg <= (p.cogs_max || 999) ? 8 : 4);
  });
  session.step4_status = "done"; session.step4_progress = 100; session.step4_stage = "Hoàn tất";
  session.step4_message = null;
  const scored = products.filter((p) => p.s4 != null);
  session.step4_score = scored.length ? Math.max(...scored.map((p) => p.s4)) : null;
  session.warehouse = warehouse || session.warehouse || "EU";
  session.target_moq = targetMoq || session.target_moq || 50;
  session.last_step4_at = nicheNowIso(0);
  return session;
}
function nicheRecomputeQuote(p, session) {
  const landed = (p.negotiated_cogs || 0) + (p.logistics_fee || 0);
  p.landed_cost = landed || null;
  p.over_ceiling = !!(p.cogs_max && landed > p.cogs_max);
  p.logistics_share = p.p_target ? nicheRound1((p.logistics_fee || 0) / p.p_target) : null;
  const cfg = NICHE_META.sourcing;
  p.s41_cogs = p.cogs_max ? nicheRound1(nicheClamp(10 - ((landed - p.cogs_max) / p.cogs_max) * 10, 1, 10)) : null;
  p.s42_logistics = p.logistics_share != null
    ? nicheRound1(nicheClamp(10 - (p.logistics_share / cfg.logistics_good) * 3, 1, 10)) : null;
  p.s43_leadtime = p.lead_time_days != null
    ? nicheRound1(nicheClamp(10 - Math.max(0, p.lead_time_days - 5) * 0.8, 1, 10)) : null;
  const parts = [
    [p.s41_cogs, cfg.weights.cogs], [p.s42_logistics, cfg.weights.logistics], [p.s43_leadtime, cfg.weights.leadtime],
  ].filter(([v]) => v != null);
  p.s4 = parts.length ? nicheRound1(parts.reduce((s, [v, w]) => s + v * w, 0) / parts.reduce((s, [, w]) => s + w, 0)) : null;
  nicheBuildSummary(session, p);
}

/* ====================================================================
   6. Đăng ký route
   ==================================================================== */

/* ---- Meta & danh sách phiên ---- */
addRoute("GET", "/api/niche/meta", () => NICHE_META);

/* Step 1 (list + create/run) đã ghép API thật (cmd/niche-research-service).
   Các endpoint step2-4/audience/targeting/... vẫn mock — nichePutReal() điền
   đủ field mặc định "draft" cho các phần đó để trang chi tiết không vỡ khi
   mở 1 session đến từ backend thật. Đổi NICHE_API_BASE khi deploy nơi khác. */
const NICHE_API_BASE = "http://localhost:8084";

function nichePutReal(real) {
  const existing = NICHE_SESSIONS.find((x) => x.id === real.id);
  const session = existing || {
    step2_status: "draft", step2_progress: 0, step2_stage: null, step2_message: null, step2_score: null,
    last_step2_at: null, ads_source: null, ecom_source: null,
    step3_status: "draft", step3_progress: 0, step3_stage: null, step3_message: null, step3_score: null,
    last_step3_at: null, price_source: null, benchmark_source: null,
    step4_status: "draft", step4_progress: 0, step4_stage: null, step4_message: null, step4_score: null,
    last_step4_at: null, warehouse: null, target_moq: null,
    cogs_share: null, ads_share: null, message: null,
  };
  Object.assign(session, real, {
    peak_months: real.peak_months || [], low_months: real.low_months || [],
    ai_risks: real.ai_risks || [], ai_actions: real.ai_actions || [],
  });
  if (!existing) NICHE_SESSIONS.unshift(session);
  return session;
}

addRoute("GET", "/api/niche/sessions", async () => {
  const res = await fetch(`${NICHE_API_BASE}/api/niche/sessions`);
  if (!res.ok) throw new Error(`niche-research-service: ${res.status}`);
  const real = await res.json();
  return real.map(nichePutReal);
});

addRoute("POST", "/api/niche/sessions", async ({ body }) => {
  if (!body || !body.raw_keyword || !body.raw_keyword.trim()) {
    throw new Error("Thiếu keyword ngách");
  }
  if (!body.country_codes || !body.country_codes.length) {
    throw new Error("Chọn ít nhất 1 thị trường");
  }
  const res = await fetch(`${NICHE_API_BASE}/api/niche/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ raw_keyword: body.raw_keyword.trim(), country_codes: body.country_codes }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || `niche-research-service: ${res.status}`);
  }
  return nichePutReal(await res.json());
});

addRoute("GET", "/api/niche/sessions/([^/]+)", ({ params }) => nicheFindSession(params[0]));

addRoute("DELETE", "/api/niche/sessions/([^/]+)", ({ params }) => {
  const idx = NICHE_SESSIONS.findIndex((x) => x.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy phiên nghiên cứu ngách");
  NICHE_SESSIONS.splice(idx, 1);
  delete NICHE_PRODUCTS[params[0]]; delete NICHE_KEYWORDS[params[0]];
  delete NICHE_AUDIENCE[params[0]]; delete NICHE_SEGMENTS[params[0]];
  return { ok: true };
});

addRoute("POST", "/api/niche/sessions/([^/]+)/run", ({ params }) => {
  const s = nicheFindSession(params[0]);
  return nicheRunStep1(s);
});

/* ---- Vùng 1 — tệp đối tượng & targeting ---- */
addRoute("POST", "/api/niche/sessions/([^/]+)/audience/estimate", ({ params, body }) => {
  const s = nicheFindSession(params[0]);
  const segs = nicheEnsureSegments(s.id);
  const all = Object.values(segs).flat();
  const ids = new Set((body && body.selected_ids) || []);
  const targetItems = all.filter((x) => x.kind !== "country" && x.kind !== "age" && x.kind !== "gender");
  const selectedTargets = targetItems.filter((x) => ids.has(x.id));
  const countrySize = segs.country.reduce((sum, c) => sum + c.size_upper, 0);
  const ageOn = segs.age.filter((x) => ids.has(x.id));
  const genderOn = segs.gender.filter((x) => ids.has(x.id));
  let base = countrySize;
  base *= ageOn.length ? Math.min(0.9, ageOn.length * 0.22) : 0.6;
  base *= genderOn.length ? Math.min(0.85, genderOn.length * 0.45) : 1;
  if (selectedTargets.length) {
    const shareAvg = selectedTargets.reduce((sSum, x) => sSum + (x.share || 0.03), 0) / selectedTargets.length;
    base *= nicheClamp(shareAvg * (1 + selectedTargets.length * 0.15), 0.01, 0.4);
  }
  const lower = Math.round(base * 0.75);
  const upper = Math.round(base * 1.15);
  const band = NICHE_META.audience_bands.find((b) => base >= b.min && (b.max == null || base < b.max))
    || NICHE_META.audience_bands[2];
  return {
    platform: "meta", lower_bound: lower, upper_bound: upper, verdict: band.key,
    note: band.note, source: "demo", quality: nicheAudienceQuality(),
  };
});

addRoute("GET", "/api/niche/sessions/([^/]+)/targeting/search", ({ params, query }) => {
  const s = nicheFindSession(params[0]);
  const segs = nicheEnsureSegments(s.id);
  const existing = new Set([...segs.interest, ...segs.behavior, ...segs.demographic].map((x) => x.label));
  const q = (query.q || "").toLowerCase().trim();
  const results = NICHE_TARGETING_POOL.filter((x) => !existing.has(x.label)
    && (!q || x.label.toLowerCase().includes(q) || (x.label_vi || "").toLowerCase().includes(q)));
  return { results };
});

addRoute("POST", "/api/niche/sessions/([^/]+)/targeting", ({ params, body }) => {
  const s = nicheFindSession(params[0]);
  const segs = nicheEnsureSegments(s.id);
  const kind = (body && body.kind) || "interest";
  const group = segs[kind] ? kind : "interest";
  const item = { id: uid("seg"), kind: group, label: body.label, label_vi: body.label_vi || null,
    size_upper: body.size_upper || 1000000, share: body.share || 0.03, selected: true };
  segs[group].push(item);
  return item;
});

addRoute("DELETE", "/api/niche/sessions/([^/]+)/targeting/([^/]+)", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const segs = nicheEnsureSegments(s.id);
  ["interest", "behavior", "demographic"].forEach((k) => {
    segs[k] = segs[k].filter((x) => x.id !== params[1]);
  });
  return { ok: true };
});

/* ---- Vùng 2 — sản phẩm tiềm năng ---- */
addRoute("GET", "/api/niche/sessions/([^/]+)/products", ({ params, query }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id);
  const includeFiltered = query.include_filtered === "true" || query.include_filtered === true;
  const list = includeFiltered ? products : products.filter((p) => p.ad_status === "valid" || !p.ad_status);
  return { products: list.slice().sort((a, b) => a.rank - b.rank) };
});

addRoute("GET", "/api/niche/sessions/([^/]+)/sources", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id);
  const ads = products.filter((p) => p.ads_count).map((p, i) => adItem(
    `ad_${p.id}`, p.name, `${p.name.split(" ").slice(0, 2).join(" ")} Store`,
    i % 2 ? "Facebook" : "Facebook + Instagram", p.matched_keyword, p.ads_active_days,
    p.ad_reach, p.pricing_note || p.pain_points, `cl_${p.id}`, p.ad_status));
  const ecom = products.filter((p) => p.ecom_platform).map((p) => ecomItem(
    `ec_${p.id}`, p.name, p.ecom_platform, p.rating, p.reviews_count, p.sales_volume,
    p.price, p.currency, p.matched_keyword, `cl_${p.id}`, true, "#"));
  return { ads, ecom };
});

addRoute("GET", "/api/niche/products/([^/]+)/members", ({ params }) => {
  const id = params[0];
  let found = null;
  Object.values(NICHE_PRODUCTS).forEach((list) => {
    const p = list.find((x) => x.id === id);
    if (p) found = p;
  });
  if (!found) return { ads: [], ecom: [] };
  const ads = found.ads_count ? [adItem(`m_ad_${id}`, found.name, "Trang demo", "Facebook",
    found.matched_keyword, found.ads_active_days, found.ad_reach, found.pain_points, `cl_${id}`, found.ad_status)] : [];
  const ecom = found.ecom_platform ? [ecomItem(`m_ec_${id}`, found.name, found.ecom_platform,
    found.rating, found.reviews_count, found.sales_volume, found.price, found.currency,
    found.matched_keyword, `cl_${id}`, true, "#")] : [];
  return { ads, ecom };
});

addRoute("POST", "/api/niche/sessions/([^/]+)/step2/run", ({ params }) => {
  const s = nicheFindSession(params[0]);
  return nicheRunStep2(s);
});

/* ---- Vùng 3 — định giá & tài chính ---- */
addRoute("POST", "/api/niche/sessions/([^/]+)/finance", ({ params, body }) => {
  const s = nicheFindSession(params[0]);
  if (body) {
    if (body.cogs_share != null) s.cogs_share = body.cogs_share;
    if (body.ads_share != null) s.ads_share = body.ads_share;
  }
  return s;
});

addRoute("POST", "/api/niche/sessions/([^/]+)/shortlist", ({ params, body }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id);
  const ids = new Set((body && body.product_ids) || []);
  products.forEach((p) => { p.shortlisted = ids.has(p.id); });
  return { ok: true, count: ids.size };
});

addRoute("POST", "/api/niche/sessions/([^/]+)/step3/run", ({ params }) => {
  const s = nicheFindSession(params[0]);
  return nicheRunStep3(s);
});

addRoute("GET", "/api/niche/sessions/([^/]+)/pricing", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id).filter((p) => p.shortlisted);
  return { products };
});

/* ---- Vùng 4 — nguồn hàng & báo giá ---- */
addRoute("GET", "/api/niche/suppliers/([^/]+)/quote-hint", ({ params }) => {
  let found = null;
  Object.values(NICHE_PRODUCTS).forEach((list) => list.forEach((p) => {
    (p.suppliers || []).forEach((x) => { if (x.id === params[0]) found = x; });
  }));
  if (!found) throw new Error("Không tìm thấy nhà cung cấp");
  return {
    negotiated_cogs: found.listed_cogs_usd,
    logistics_fee: nicheRound1(found.listed_cogs_usd * 0.18),
    lead_time_days: found.platform === "eu_stock" ? 2 : found.platform === "1688" ? 5 : 10,
  };
});

addRoute("POST", "/api/niche/products/([^/]+)/quote", ({ params, body }) => {
  const id = params[0];
  let found = null; let ownerSession = null;
  Object.entries(NICHE_PRODUCTS).forEach(([sid, list]) => {
    const p = list.find((x) => x.id === id);
    if (p) { found = p; ownerSession = NICHE_SESSIONS.find((x) => x.id === sid); }
  });
  if (!found) throw new Error("Không tìm thấy sản phẩm");
  found.supplier_id = body.supplier_id ?? found.supplier_id;
  found.negotiated_cogs = body.negotiated_cogs ?? found.negotiated_cogs;
  found.logistics_fee = body.logistics_fee ?? found.logistics_fee;
  found.lead_time_days = body.lead_time_days ?? found.lead_time_days;
  found.sourcing_note = body.note ?? found.sourcing_note;
  found.quoted_by = body.quoted_by ?? found.quoted_by;
  nicheRecomputeQuote(found, ownerSession);
  return found;
});

addRoute("POST", "/api/niche/sessions/([^/]+)/step4/run", ({ params, body }) => {
  const s = nicheFindSession(params[0]);
  return nicheRunStep4(s, body && body.warehouse, body && body.target_moq);
});

addRoute("GET", "/api/niche/sessions/([^/]+)/sourcing", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id).filter((p) => p.shortlisted);
  return { products };
});

/* ---- Vùng 4 (tổng hợp) & hồ sơ ngách ---- */
addRoute("GET", "/api/niche/sessions/([^/]+)/summary", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id);
  products.forEach((p) => nicheBuildSummary(s, p));
  return { products: products.slice().sort((a, b) => (b.total_score || 0) - (a.total_score || 0)) };
});

addRoute("POST", "/api/niche/sessions/([^/]+)/summarize", ({ params }) => {
  const s = nicheFindSession(params[0]);
  const products = nicheEnsureProducts(s.id);
  products.forEach((p) => nicheBuildSummary(s, p));
  return { products: products.slice().sort((a, b) => (b.total_score || 0) - (a.total_score || 0)) };
});

addRoute("GET", "/api/niche/sessions/([^/]+)/profile", ({ params }) => {
  const s = nicheFindSession(params[0]);
  return {
    keywords: nicheEnsureKeywords(s.id),
    audience: nicheEnsureAudience(s.id),
    segments: nicheEnsureSegments(s.id),
  };
});
/* ===================================================================
   MOCK API — Benchmark & Sản phẩm tiềm năng (window.BenchmarkPage)
   Nguồn sự thật: web/benchmark.jsx — mọi field bên dưới khớp với những gì
   component đọc ra (session.*, keyword.*, product.*, evaluation.*, meta.*).

   Giả định (assumptions — xem báo cáo cuối cùng để biết chi tiết):
   - meta.countries: danh sách thị trường target, dùng cho <Select> tạo phiên.
   - /keywords/expand trả { keywords, source }; toggle PUT trả object keyword
     đã cập nhật nhưng UI không dùng response (chỉ optimistic update) nên ta
     vẫn trả bản ghi mới cho an toàn.
   - /scrape khởi động một tiến trình "running" giả lập tiến độ theo thời gian
     thực (progress tăng dần mỗi lần GET session), rồi tự chuyển "done".
   - /products?query trả { stats, products } — stats tính từ tập kept
     (không noise trừ khi include_noise=1) sau khi áp bộ lọc min_days_active/sort.
   - parse-upload trả { products: [...] } — mỗi phần tử theo đúng shape EMPTY_ROW
     dùng ở AddProductDrawer (name, source_url, cogs, price_expected, variants,
     material, note, image_url, media_count).
   - enrich-url trả { ok, name, source_url, image_url, media_count, variants,
     material, note, price, scrape_source, warning } — theo đúng cách đọc ở
     benchmark.jsx dòng ~761-770.
   - evaluate trả { results: [{id, ...}] } — mỗi kết quả là 1 evaluation đầy đủ
     (được lưu vào BENCHMARK_EVALUATIONS ngay), UI chỉ cần results[].id.
   =================================================================== */

/* --------------------------- META --------------------------- */
const BENCHMARK_META = {
  ads_source: "demo",              // "meta_api" | "demo" — chưa cấu hình META_ACCESS_TOKEN
  ai_enabled: true,                // "Claude (vision + text)" khi true, ngược lại rule engine
  relevance_threshold: 0.62,
  countries: [
    { code: "VN", name: "Việt Nam", currency: "VND" },
    { code: "NL", name: "Hà Lan", currency: "EUR" },
    { code: "DE", name: "Đức", currency: "EUR" },
    { code: "FR", name: "Pháp", currency: "EUR" },
    { code: "GB", name: "Anh", currency: "GBP" },
    { code: "PL", name: "Ba Lan", currency: "PLN" },
    { code: "US", name: "Hoa Kỳ", currency: "USD" },
    { code: "SA", name: "Ả Rập Xê Út", currency: "SAR" },
    { code: "AE", name: "UAE", currency: "AED" },
  ],
};

/* ----------------------- helpers cho seed ----------------------- */
function bmIso(daysAgo, hour = 9, minute = 0) {
  const d = new Date();
  d.setUTCDate(d.getUTCDate() - daysAgo);
  d.setUTCHours(hour, minute, 0, 0);
  return d.toISOString();
}
function bmPic(seed, w = 400, h = 400) {
  return `https://picsum.photos/seed/${seed}/${w}/${h}`;
}

/* --------------------------- KEYWORDS --------------------------- */
const BM_KEYWORDS_1 = [
  { id: "kw_1", session_id: "bm_demo_1", keyword: "áo thun local brand", kind: "seed", enabled: 1, volume: 40500, trend: 12, note: "Từ khoá gốc do người dùng nhập" },
  { id: "kw_2", session_id: "bm_demo_1", keyword: "áo thun oversize unisex", kind: "synonym", enabled: 1, volume: 27100, trend: 8, note: "Biến thể đồng nghĩa — AI mở rộng" },
  { id: "kw_3", session_id: "bm_demo_1", keyword: "áo thun in hình streetwear", kind: "feature", enabled: 1, volume: 18300, trend: 22, note: "Nhấn mạnh tính năng in hình" },
  { id: "kw_4", session_id: "bm_demo_1", keyword: "áo thun cotton form rộng", kind: "feature", enabled: 1, volume: 9800, trend: -3, note: "Chất liệu + form dáng" },
  { id: "kw_5", session_id: "bm_demo_1", keyword: "mua áo thun local brand giá rẻ", kind: "cta", enabled: 1, volume: 6400, trend: 15, note: "Cụm chốt đơn có yếu tố giá" },
  { id: "kw_6", session_id: "bm_demo_1", keyword: "áo thun trơn basic", kind: "synonym", enabled: 0, volume: 15200, trend: -6, note: "Đã tắt — quá rộng, ít liên quan local brand" },
  { id: "kw_7", session_id: "bm_demo_1", keyword: "local brand việt nam áo thun hot trend", kind: "cta", enabled: 1, volume: 5100, trend: 31, note: "Cụm dài, độ liên quan cao" },
];

/* --------------------------- PRODUCTS (kho benchmark) --------------------------- */
const BM_PRODUCTS_1 = [
  { id: "bp_1", session_id: "bm_demo_1", product_name: "Áo thun local brand DTF in hình rồng phong thuỷ", page_name: "Kaiz Studio", page_id: "1029384756",
    image_url: bmPic("bmp1"), price: 189000, currency: "VND", days_active: 42, reach: 128500, relevance_score: 0.91,
    matched_keyword: "áo thun local brand", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2021-03-12", page_ads_active: 6, landing_page_url: "https://kaizstudio.vn/products/dtf-rong-phong-thuy",
    media_urls: [{ type: "image", url: bmPic("bmp1a"), thumbnail: bmPic("bmp1a", 160, 160) }, { type: "video", url: bmPic("bmp1v", 480, 854), thumbnail: bmPic("bmp1v", 160, 160) }],
    ad_copies: ["Áo thun local brand chuẩn form Hàn — in DTF sắc nét không bong tróc sau 50 lần giặt. Mua 2 tặng 1 túi tote!",
      "Hot trend rồng phong thuỷ 2026 — giới hạn 500 chiếc, đặt ngay kẻo hết size."],
    page_other_ads: [{ product_name: "Áo thun local brand form rộng basic đen", image_url: bmPic("bmp1o1"), days_active: 30, reach: 61200, price: 159000 },
      { product_name: "Hoodie local brand unisex", image_url: bmPic("bmp1o2"), days_active: 18, reach: 40500, price: 349000 }],
    landing_structure: [{ block: "Hero", detail: "Ảnh sản phẩm full màn hình + CTA 'Mua ngay' nổi bật" },
      { block: "Bảng size", detail: "Size chart quy đổi S-XXL kèm video hướng dẫn đo" },
      { block: "Đánh giá", detail: "32 review kèm ảnh khách hàng thực tế" }] },
  { id: "bp_2", session_id: "bm_demo_1", product_name: "Áo thun oversize unisex form Hàn tay lỡ", page_name: "Metan Wear", page_id: "1029384757",
    image_url: bmPic("bmp2"), price: 215000, currency: "VND", days_active: 65, reach: 210300, relevance_score: 0.88,
    matched_keyword: "áo thun oversize unisex", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2020-08-01", page_ads_active: 9, landing_page_url: "https://metanwear.com/oversize-tay-lo",
    media_urls: [{ type: "image", url: bmPic("bmp2a"), thumbnail: bmPic("bmp2a", 160, 160) }],
    ad_copies: ["Oversize chuẩn Hàn, vải cotton 100% co giãn 4 chiều — mặc mát cả ngày dài."],
    page_other_ads: [{ product_name: "Quần jogger unisex", image_url: bmPic("bmp2o1"), days_active: 22, reach: 33100, price: 259000 }],
    landing_structure: [{ block: "Hero", detail: "Video ngắn 15s người mẫu mặc thực tế" }] },
  { id: "bp_3", session_id: "bm_demo_1", product_name: "Áo thun in hình streetwear graffiti", page_name: "Urban Saigon", page_id: "1029384758",
    image_url: bmPic("bmp3"), price: 249000, currency: "VND", days_active: 19, reach: 87600, relevance_score: 0.79,
    matched_keyword: "áo thun in hình streetwear", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2023-01-20", page_ads_active: 4, landing_page_url: "https://urbansaigon.shop/graffiti-tee",
    media_urls: [{ type: "image", url: bmPic("bmp3a"), thumbnail: bmPic("bmp3a", 160, 160) }, { type: "image", url: bmPic("bmp3b"), thumbnail: bmPic("bmp3b", 160, 160) }],
    ad_copies: ["Streetwear graffiti — độc bản, không đụng hàng. Freeship đơn từ 2 áo."],
    page_other_ads: [], landing_structure: [{ block: "Bộ sưu tập", detail: "Lưới 6 mẫu graffiti khác nhau, lọc theo size" }] },
  { id: "bp_4", session_id: "bm_demo_1", product_name: "Combo 3 áo thun cotton form rộng basic", page_name: "Cotton House VN", page_id: "1029384759",
    image_url: bmPic("bmp4"), price: 399000, currency: "VND", days_active: 51, reach: 156200, relevance_score: 0.84,
    matched_keyword: "áo thun cotton form rộng", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2019-11-05", page_ads_active: 11, landing_page_url: "https://cottonhouse.vn/combo-3-basic",
    media_urls: [{ type: "video", url: bmPic("bmp4v", 480, 854), thumbnail: bmPic("bmp4v", 160, 160) }],
    ad_copies: ["Combo 3 áo basic mặc quanh năm — vải cotton 220gsm dày dặn, không xù lông."],
    page_other_ads: [{ product_name: "Áo ba lỗ basic", image_url: bmPic("bmp4o1"), days_active: 40, reach: 71000, price: 99000 }],
    landing_structure: [] },
  { id: "bp_5", session_id: "bm_demo_1", product_name: "Áo thun local brand hot trend logo thêu", page_name: "Nomad Studio", page_id: "1029384760",
    image_url: bmPic("bmp5"), price: 229000, currency: "VND", days_active: 8, reach: 32100, relevance_score: 0.86,
    matched_keyword: "local brand việt nam áo thun hot trend", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2024-06-14", page_ads_active: 3, landing_page_url: "https://nomadstudio.vn/logo-theu",
    media_urls: [{ type: "image", url: bmPic("bmp5a"), thumbnail: bmPic("bmp5a", 160, 160) }],
    ad_copies: ["Logo thêu nổi tinh xảo — local brand xu hướng 2026, giới hạn số lượng."],
    page_other_ads: [], landing_structure: [{ block: "Hero", detail: "Ảnh cận cảnh chi tiết thêu" }] },
  { id: "bp_6", session_id: "bm_demo_1", product_name: "Ốp lưng iPhone 15 chống sốc trong suốt", page_name: "TechCase Store", page_id: "1029384761",
    image_url: bmPic("bmp6"), price: 79000, currency: "VND", days_active: 25, reach: 45200, relevance_score: 0.21,
    matched_keyword: "áo thun trơn basic", is_noise: 1, manual_override: 0, brand_type: "off_niche", noise_reason: "Sản phẩm không liên quan ngành hàng thời trang (phụ kiện điện thoại)",
    page_created: "2022-02-02", page_ads_active: 15, landing_page_url: "https://techcase.store/iphone15-case",
    media_urls: [{ type: "image", url: bmPic("bmp6a"), thumbnail: bmPic("bmp6a", 160, 160) }],
    ad_copies: ["Ốp lưng chống sốc 4 góc, trong suốt không ố vàng."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_7", session_id: "bm_demo_1", product_name: "Áo thun local brand tay ngắn basic trơn 100% cotton", page_name: "Basic Club", page_id: "1029384762",
    image_url: bmPic("bmp7"), price: 149000, currency: "VND", days_active: 73, reach: 302100, relevance_score: 0.93,
    matched_keyword: "áo thun local brand", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2018-05-09", page_ads_active: 20, landing_page_url: "https://basicclub.vn/tron-basic",
    media_urls: [{ type: "image", url: bmPic("bmp7a"), thumbnail: bmPic("bmp7a", 160, 160) }, { type: "video", url: bmPic("bmp7v", 480, 854), thumbnail: bmPic("bmp7v", 160, 160) }],
    ad_copies: ["Best-seller 3 năm liền — áo basic trơn 100% cotton form chuẩn mọi dáng người.",
      "Giặt máy thoải mái, không nhăn không xù — bảo hành phai màu 6 tháng."],
    page_other_ads: [{ product_name: "Quần short basic", image_url: bmPic("bmp7o1"), days_active: 60, reach: 120400, price: 189000 },
      { product_name: "Áo polo basic", image_url: bmPic("bmp7o2"), days_active: 55, reach: 98200, price: 219000 }],
    landing_structure: [{ block: "Hero", detail: "Banner 'Best-seller' + đếm ngược flash sale" },
      { block: "So sánh chất liệu", detail: "Bảng so sánh cotton 100% với vải pha" }] },
  { id: "bp_8", session_id: "bm_demo_1", product_name: "Nồi chiên không dầu 5L đa năng", page_name: "Gia Dụng Việt", page_id: "1029384763",
    image_url: bmPic("bmp8"), price: 890000, currency: "VND", days_active: 12, reach: 22300, relevance_score: 0.08,
    matched_keyword: "áo thun cotton form rộng", is_noise: 1, manual_override: 0, brand_type: "off_niche", noise_reason: "Ads trùng khớp keyword ngẫu nhiên nhưng thuộc ngành gia dụng",
    page_created: "2021-09-30", page_ads_active: 8, landing_page_url: "https://giadungviet.vn/noi-chien-5l",
    media_urls: [], ad_copies: ["Nồi chiên không dầu 5L — nấu ăn healthy cho cả gia đình."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_9", session_id: "bm_demo_1", product_name: "Áo thun oversize unisex phối màu block", page_name: "Metan Wear", page_id: "1029384757",
    image_url: bmPic("bmp9"), price: 235000, currency: "VND", days_active: 33, reach: 98700, relevance_score: 0.81,
    matched_keyword: "áo thun oversize unisex", is_noise: 0, manual_override: 1, brand_type: null, noise_reason: null,
    page_created: "2020-08-01", page_ads_active: 9, landing_page_url: "https://metanwear.com/phoi-mau-block",
    media_urls: [{ type: "image", url: bmPic("bmp9a"), thumbnail: bmPic("bmp9a", 160, 160) }],
    ad_copies: ["Phối màu block cá tính — nổi bật giữa đám đông."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_10", session_id: "bm_demo_1", product_name: "Áo thun in hình streetwear typography", page_name: "Urban Saigon", page_id: "1029384758",
    image_url: bmPic("bmp10"), price: 259000, currency: "VND", days_active: 5, reach: 15100, relevance_score: 0.77,
    matched_keyword: "áo thun in hình streetwear", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2023-01-20", page_ads_active: 4, landing_page_url: "https://urbansaigon.shop/typography-tee",
    media_urls: [], ad_copies: ["Typography tee mới ra mắt — pre-order giảm 20%."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_11", session_id: "bm_demo_1", product_name: "Áo thun local brand form suông unisex phản quang", page_name: "Nomad Studio", page_id: "1029384760",
    image_url: bmPic("bmp11"), price: 269000, currency: "VND", days_active: 28, reach: 61900, relevance_score: 0.85,
    matched_keyword: "mua áo thun local brand giá rẻ", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2024-06-14", page_ads_active: 3, landing_page_url: "https://nomadstudio.vn/phan-quang",
    media_urls: [{ type: "image", url: bmPic("bmp11a"), thumbnail: bmPic("bmp11a", 160, 160) }],
    ad_copies: ["Chi tiết phản quang độc quyền — nổi bật khi chụp đèn flash ban đêm."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_12", session_id: "bm_demo_1", product_name: "Áo thun local brand tay lỡ in chữ tối giản", page_name: "Basic Club", page_id: "1029384762",
    image_url: bmPic("bmp12"), price: 169000, currency: "VND", days_active: 44, reach: 143800, relevance_score: 0.89,
    matched_keyword: "áo thun local brand", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2018-05-09", page_ads_active: 20, landing_page_url: "https://basicclub.vn/toi-gian",
    media_urls: [{ type: "image", url: bmPic("bmp12a"), thumbnail: bmPic("bmp12a", 160, 160) }],
    ad_copies: ["In chữ tối giản — phong cách quiet luxury đang lên ngôi."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_13", session_id: "bm_demo_1", product_name: "Giày sneaker nữ đế độn 5cm", page_name: "ShoeMood", page_id: "1029384764",
    image_url: bmPic("bmp13"), price: 349000, currency: "VND", days_active: 15, reach: 27600, relevance_score: 0.12,
    matched_keyword: "áo thun trơn basic", is_noise: 1, manual_override: 0, brand_type: "off_niche", noise_reason: "Ngành hàng giày dép, không phải áo thun",
    page_created: "2022-11-11", page_ads_active: 7, landing_page_url: "https://shoemood.vn/sneaker-de-don",
    media_urls: [], ad_copies: ["Sneaker đế độn êm chân — tăng chiều cao 5cm tự nhiên."],
    page_other_ads: [], landing_structure: [] },
  { id: "bp_14", session_id: "bm_demo_1", product_name: "Áo thun cotton form rộng tay dài mùa thu", page_name: "Cotton House VN", page_id: "1029384759",
    image_url: bmPic("bmp14"), price: 219000, currency: "VND", days_active: 3, reach: 6200, relevance_score: 0.74,
    matched_keyword: "áo thun cotton form rộng", is_noise: 0, manual_override: 0, brand_type: null, noise_reason: null,
    page_created: "2019-11-05", page_ads_active: 11, landing_page_url: "https://cottonhouse.vn/tay-dai-thu",
    media_urls: [], ad_copies: ["Bộ sưu tập thu mới — tay dài giữ ấm nhẹ, vẫn thoáng mát."],
    page_other_ads: [], landing_structure: [] },
];

/* --------------------------- EVALUATION CRITERIA TEMPLATE --------------------------- */
const BM_CRITERIA_TEMPLATE = [
  { key: "visual_trend", label: "Visual trend", label_vi: "Trend hình ảnh", weight: 15 },
  { key: "ad_traction", label: "Ad traction", label_vi: "Sức hút ads", weight: 20 },
  { key: "margin", label: "Margin", label_vi: "Biên lợi nhuận", weight: 15 },
  { key: "variants", label: "Variants", label_vi: "Biến thể", weight: 10 },
  { key: "creative", label: "Creative hook", label_vi: "Creative hook", weight: 15 },
  { key: "assets", label: "Media assets", label_vi: "Kho media", weight: 10 },
  { key: "localized_fit", label: "Localized fit", label_vi: "Bản địa hoá", weight: 10 },
  { key: "logistics", label: "Logistics", label_vi: "Kho vận", weight: 5 },
];

function bmCriteria(scores) {
  return BM_CRITERIA_TEMPLATE.map((c, i) => ({ ...c, score: scores[i], comment: BM_CRITERIA_COMMENTS[c.key][scores[i] >= 7 ? 0 : scores[i] >= 5 ? 1 : 2] }));
}
const BM_CRITERIA_COMMENTS = {
  visual_trend: ["Hình ảnh bắt trend hiện tại, phối màu nổi bật trên feed.", "Hình ảnh ổn nhưng chưa thực sự nổi bật so với đối thủ.", "Ảnh sản phẩm mờ nhạt, cần đầu tư lại bộ ảnh/creative."],
  ad_traction: ["Sản phẩm tương tự đang chạy ads dài ngày với reach cao trong benchmark.", "Có vài ads benchmark tương tự nhưng reach trung bình.", "Chưa thấy ads benchmark nào thực sự tương đồng đang chạy tốt."],
  margin: ["Biên lợi nhuận dự kiến tốt (trên 40%) so với giá vốn nhập.", "Biên lợi nhuận ở mức chấp nhận được (25-40%).", "Biên lợi nhuận mỏng, cần thương lượng lại giá vốn."],
  variants: ["Đa dạng biến thể màu/size giúp tăng AOV và giảm rủi ro tồn kho.", "Biến thể ở mức trung bình, có thể bổ sung thêm size.", "Thiếu biến thể, dễ gặp tình trạng hết size bán chạy."],
  creative: ["Có nhiều góc creative hook tiềm năng (unbox, before/after, POV).", "Có thể khai thác creative nhưng cần thử nghiệm thêm hook mới.", "Ý tưởng creative còn hạn chế, khó tạo hook viral."],
  assets: ["Kho media đầy đủ ảnh + video chất lượng để chạy ads ngay.", "Có ảnh cơ bản nhưng thiếu video, cần quay thêm.", "Kho media rất mỏng, cần đầu tư sản xuất từ đầu."],
  localized_fit: ["Rất phù hợp thị hiếu và văn hoá thị trường mục tiêu.", "Phù hợp một phần, có thể cần điều chỉnh nhỏ về thông điệp.", "Chưa phù hợp thị hiếu địa phương, rủi ro tỉ lệ chuyển đổi thấp."],
  logistics: ["Kích thước/khối lượng nhỏ gọn, dễ đóng gói và ship nhanh.", "Kho vận ở mức bình thường, không có trở ngại lớn.", "Sản phẩm cồng kềnh hoặc dễ vỡ, chi phí ship và hoàn hàng cao."],
};

/* --------------------------- EVALUATIONS (đã chấm điểm) --------------------------- */
const BM_EVALUATIONS_1 = [
  {
    id: "ev_1", session_id: "bm_demo_1", status: "done", product_name: "Áo thun local brand in 3D hiệu ứng chuyển động",
    image_url: bmPic("ev1"), source_url: "https://detail.1688.com/offer/778812345.html",
    price_expected: 199000, cogs: 62000, currency: "VND", win_score: 82, verdict: "scale", verdict_label: "Nên scale ngay",
    ai_source: "claude", created_at: bmIso(1, 10, 15),
    summary: "Sản phẩm bắt trend in 3D hiệu ứng chuyển động, biên lợi nhuận tốt và có nhiều ads benchmark tương tự đang chạy dài ngày.",
    criteria: bmCriteria([9, 8, 8, 7, 9, 8, 8, 7]),
    matches: [{ name: "Áo thun local brand DTF in hình rồng phong thuỷ", reason: "Cùng kỹ thuật in DTF/3D, cùng phân khúc giá và target trẻ", similarity: 0.87 },
      { name: "Áo thun local brand tay ngắn basic trơn 100% cotton", reason: "Cùng page benchmark chạy dài ngày, cùng chất liệu cotton", similarity: 0.61 }],
    strengths: ["Hiệu ứng in 3D độc đáo, dễ tạo hook video unbox.", "Giá vốn thấp giúp biên lợi nhuận trên 60%.", "Có 2 ads benchmark tương tự đã chạy hơn 40 ngày — dấu hiệu thị trường chấp nhận tốt."],
    gaps: ["Cần kiểm tra độ bền lớp in sau 20-30 lần giặt trước khi cam kết bảo hành.", "Chưa có biến thể màu tối, nên bổ sung để tăng lựa chọn."],
  },
  {
    id: "ev_2", session_id: "bm_demo_1", status: "done", product_name: "Áo thun cổ tròn basic phối nút vai",
    image_url: bmPic("ev2"), source_url: "https://item.taobao.com/item.htm?id=9981234",
    price_expected: 175000, cogs: 71000, currency: "VND", win_score: 58, verdict: "consider", verdict_label: "Cân nhắc thử nghiệm",
    ai_source: "claude", created_at: bmIso(2, 14, 30),
    summary: "Sản phẩm ổn nhưng khá tương đồng với nhiều mẫu basic đã bão hoà trên thị trường, cần điểm khác biệt rõ hơn.",
    criteria: bmCriteria([6, 5, 6, 5, 4, 6, 7, 8]),
    matches: [{ name: "Combo 3 áo thun cotton form rộng basic", reason: "Cùng phân khúc basic combo, cạnh tranh trực tiếp về giá", similarity: 0.54 }],
    strengths: ["Kho vận thuận lợi vì kích thước nhỏ gọn.", "Phù hợp thị hiếu người tiêu dùng phổ thông."],
    gaps: ["Thiết kế phối nút vai chưa đủ khác biệt để nổi bật trên newsfeed.", "Chưa có creative hook rõ ràng, dễ bị chìm giữa nhiều đối thủ basic.", "Biên lợi nhuận chỉ khoảng 24%, cần thương lượng giảm giá vốn."],
  },
  {
    id: "ev_3", session_id: "bm_demo_1", status: "done", product_name: "Áo thun tay ngắn in hình anime bootleg",
    image_url: bmPic("ev3"), source_url: null,
    price_expected: 159000, cogs: 78000, currency: "VND", win_score: 39, verdict: "risky", verdict_label: "Rủi ro cao",
    ai_source: "claude", created_at: bmIso(3, 9, 5),
    summary: "Rủi ro bản quyền hình ảnh anime cao, đồng thời chưa thấy ads benchmark tương tự nào bền vững trong ngành hàng này.",
    criteria: bmCriteria([5, 3, 4, 4, 6, 4, 5, 6]),
    matches: [],
    strengths: ["Ảnh in bắt mắt, có thể thu hút nhóm khách hàng fan anime.", "Chi phí sản xuất thấp nếu in theo yêu cầu (POD)."],
    gaps: ["Rủi ro vi phạm bản quyền hình ảnh, dễ bị khoá trang quảng cáo.", "Không tìm thấy ads benchmark tương tự đang chạy ổn định trong tập dữ liệu.", "Biên lợi nhuận mỏng chỉ khoảng 16% trên giá bán dự kiến."],
  },
  {
    id: "ev_4", session_id: "bm_demo_1", status: "done", product_name: "Áo thun local brand thêu chữ ký hoạ tiết hoa",
    image_url: bmPic("ev4"), source_url: "https://www.dhgate.com/product/embroidery-tee/456789.html",
    price_expected: 245000, cogs: 95000, currency: "VND", win_score: 69, verdict: "potential", verdict_label: "Tiềm năng",
    ai_source: "claude", created_at: bmIso(5, 16, 45),
    summary: "Có tiềm năng tốt nhờ chi tiết thêu tinh xảo và phù hợp nhóm khách hàng nữ trẻ, cần thử nghiệm thêm creative trước khi scale mạnh.",
    criteria: bmCriteria([8, 6, 6, 8, 7, 7, 7, 6]),
    matches: [{ name: "Áo thun local brand hot trend logo thêu", reason: "Cùng kỹ thuật thêu, cùng target khách hàng nữ 18-28 tuổi", similarity: 0.73 }],
    strengths: ["Chi tiết thêu tạo cảm giác cao cấp, tăng giá trị cảm nhận.", "Đa dạng biến thể hoạ tiết giúp giữ khách quay lại mua thêm."],
    gaps: ["Giá vốn cao hơn trung bình ngành, cần tối ưu lại nguồn hàng.", "Cần thêm video cận cảnh chi tiết thêu để tăng độ tin cậy."],
  },
  {
    id: "ev_5", session_id: "bm_demo_1", status: "error", product_name: "Áo thun không rõ nguồn (link lỗi)",
    image_url: null, source_url: "https://example-broken-link.cn/product/000",
    price_expected: null, cogs: null, currency: "VND", win_score: null, verdict: null, verdict_label: null,
    ai_source: "claude", created_at: bmIso(6, 11, 0),
    summary: null, criteria: [], matches: [], strengths: [], gaps: [],
    error: "Không tải được ảnh sản phẩm từ URL nguồn — vui lòng nhập ảnh thủ công và chấm lại.",
  },
];

/* --------------------------- SESSIONS --------------------------- */
function bmComputeStats(products) {
  const kept = products.filter((p) => !p.is_noise);
  const noise = products.filter((p) => p.is_noise);
  const longRunning = kept.filter((p) => p.days_active > 14).length;
  const avgPrice = kept.length ? Math.round(kept.reduce((a, p) => a + p.price, 0) / kept.length) : 0;
  const topReach = kept.length ? Math.max(...kept.map((p) => p.reach)) : 0;
  return { kept: kept.length, noise: noise.length, raw: products.length, long_running: longRunning, avg_price: avgPrice, top_reach: topReach };
}

const BENCHMARK_SESSIONS = [
  {
    id: "bm_demo_1",
    category: "Áo thun local brand",
    country_code: "VN", country_name: "Việt Nam", currency: "VND",
    seed_keywords: ["áo thun local brand", "áo thun oversize", "áo thun in hình"],
    status: "done", progress: 100, stage: "Đã lưu benchmark", message: null,
    source: "demo", ads_source: "demo",
    created_at: bmIso(9, 8, 0), last_scraped_at: bmIso(0, 8, 30),
    kept_count: bmComputeStats(BM_PRODUCTS_1).kept,
    noise_count: bmComputeStats(BM_PRODUCTS_1).noise,
    raw_count: bmComputeStats(BM_PRODUCTS_1).raw,
    _keywords: BM_KEYWORDS_1,
    _products: BM_PRODUCTS_1,
    _evaluations: BM_EVALUATIONS_1,
  },
];

/* ===================================================================
   ROUTES
   =================================================================== */

function bmFindSession(id) {
  const s = BENCHMARK_SESSIONS.find((x) => x.id === id);
  if (!s) throw new Error("Không tìm thấy phiên nghiên cứu");
  return s;
}
function bmSessionPublic(s) {
  // Trả object session "phẳng" — đúng field mà benchmark.jsx đọc trên `session.*`
  const { _keywords, _products, _evaluations, ...pub } = s;
  return { ...pub };
}
function bmFindProduct(id) {
  for (const s of BENCHMARK_SESSIONS) {
    const p = s._products.find((x) => x.id === id);
    if (p) return { session: s, product: p };
  }
  throw new Error("Không tìm thấy sản phẩm benchmark");
}
function bmFindEvaluation(id) {
  for (const s of BENCHMARK_SESSIONS) {
    const e = s._evaluations.find((x) => x.id === id);
    if (e) return { session: s, evaluation: e };
  }
  throw new Error("Không tìm thấy báo cáo chấm điểm");
}

/* GET /api/benchmark/meta */
addRoute("GET", "/api/benchmark/meta", () => BENCHMARK_META);

/* GET /api/benchmark/sessions — danh sách phiên (tóm tắt) */
addRoute("GET", "/api/benchmark/sessions", () => BENCHMARK_SESSIONS.map(bmSessionPublic));

/* POST /api/benchmark/sessions — tạo phiên mới (bắt đầu ở trạng thái idle) */
addRoute("POST", "/api/benchmark/sessions", ({ body }) => {
  const country = BENCHMARK_META.countries.find((c) => c.code === body.country_code) || BENCHMARK_META.countries[0];
  const id = uid("bm");
  const s = {
    id, category: body.category || "Ngành hàng chưa đặt tên",
    country_code: country.code, country_name: country.name, currency: country.currency,
    seed_keywords: body.seed_keywords || [],
    status: "idle", progress: 0, stage: "Chưa chạy", message: null,
    source: null, ads_source: BENCHMARK_META.ads_source,
    created_at: new Date().toISOString(), last_scraped_at: null,
    kept_count: 0, noise_count: 0, raw_count: 0,
    _keywords: (body.seed_keywords || []).map((kw) => ({
      id: uid("kw"), session_id: id, keyword: kw, kind: "seed", enabled: 1, volume: 500 + Math.floor(Math.random() * 20000), trend: Math.floor(Math.random() * 40) - 10, note: "Từ khoá gốc do người dùng nhập",
    })),
    _products: [], _evaluations: [],
  };
  BENCHMARK_SESSIONS.push(s);
  return bmSessionPublic(s);
});

/* GET /api/benchmark/sessions/:id */
addRoute("GET", "/api/benchmark/sessions/([^/]+)", ({ params }) => {
  const s = bmFindSession(params[0]);
  // Nếu đang chạy cào, tiến độ tăng dần theo thời gian mỗi lần poll
  if (s.status === "running") {
    s.progress = Math.min(100, (s.progress || 0) + 22 + Math.floor(Math.random() * 15));
    if (s.progress < 60) s.stage = "Fetching Ads Library";
    else if (s.progress < 90) s.stage = "AI Noise Filtering";
    else s.stage = "Storing Benchmark";
    if (s.progress >= 100) {
      s.progress = 100;
      s.status = "done";
      s.stage = "Đã lưu benchmark";
      s.source = "demo";
      s.last_scraped_at = new Date().toISOString();
      // Nếu phiên chưa có sản phẩm nào (phiên mới tạo), seed một tập nhỏ demo
      if (!s._products.length) {
        s._products = BM_PRODUCTS_1.slice(0, 8).map((p) => ({ ...p, id: uid("bp"), session_id: s.id }));
      }
      const st = bmComputeStats(s._products);
      s.kept_count = st.kept; s.noise_count = st.noise; s.raw_count = st.raw;
    }
  }
  return bmSessionPublic(s);
});

/* DELETE /api/benchmark/sessions/:id */
addRoute("DELETE", "/api/benchmark/sessions/([^/]+)", ({ params }) => {
  const idx = BENCHMARK_SESSIONS.findIndex((x) => x.id === params[0]);
  if (idx === -1) throw new Error("Không tìm thấy phiên nghiên cứu");
  BENCHMARK_SESSIONS.splice(idx, 1);
  return { ok: true };
});

/* GET /api/benchmark/sessions/:id/keywords */
addRoute("GET", "/api/benchmark/sessions/([^/]+)/keywords", ({ params }) => bmFindSession(params[0])._keywords);

/* POST /api/benchmark/sessions/:id/keywords/expand */
const BM_EXPAND_POOL = [
  { keyword: "áo thun trend 2026", kind: "cta" }, { keyword: "áo thun form baggy", kind: "feature" },
  { keyword: "áo thun cổ tròn unisex", kind: "synonym" }, { keyword: "đặt áo thun local brand online", kind: "cta" },
  { keyword: "áo thun thêu logo mini", kind: "feature" }, { keyword: "áo thun basic tee", kind: "synonym" },
];
addRoute("POST", "/api/benchmark/sessions/([^/]+)/keywords/expand", ({ params }) => {
  const s = bmFindSession(params[0]);
  const extra = BM_EXPAND_POOL.filter((x) => !s._keywords.some((k) => k.keyword === x.keyword)).slice(0, 4).map((x) => ({
    id: uid("kw"), session_id: s.id, keyword: x.keyword, kind: x.kind, enabled: 1,
    volume: 500 + Math.floor(Math.random() * 25000), trend: Math.floor(Math.random() * 50) - 15,
    note: "AI mở rộng — biến thể / dịch từ khoá gốc",
  }));
  s._keywords = [...s._keywords, ...extra];
  return { keywords: s._keywords, source: BENCHMARK_META.ai_enabled ? "claude" : "rule" };
});

/* PUT /api/benchmark/keywords/:id — bật/tắt keyword */
addRoute("PUT", "/api/benchmark/keywords/([^/]+)", ({ params, body }) => {
  for (const s of BENCHMARK_SESSIONS) {
    const k = s._keywords.find((x) => x.id === params[0]);
    if (k) { k.enabled = body.enabled ? 1 : 0; return k; }
  }
  throw new Error("Không tìm thấy keyword");
});

/* POST /api/benchmark/sessions/:id/scrape — khởi động (hoặc làm mới) tiến trình cào */
addRoute("POST", "/api/benchmark/sessions/([^/]+)/scrape", ({ params }) => {
  const s = bmFindSession(params[0]);
  s.status = "running"; s.progress = 5; s.stage = "Fetching Ads Library"; s.message = null;
  return { ok: true, status: "running" };
});

/* GET /api/benchmark/sessions/:id/products?min_days_active=&sort=&include_noise= */
addRoute("GET", "/api/benchmark/sessions/([^/]+)/products", ({ params, query }) => {
  const s = bmFindSession(params[0]);
  const minDays = Number(query.min_days_active || 0);
  const includeNoise = query.include_noise === "true" || query.include_noise === "1";
  let list = s._products.filter((p) => (includeNoise || !p.is_noise) && p.days_active >= minDays);
  const sort = query.sort || "reach";
  const cmp = {
    reach: (a, b) => b.reach - a.reach,
    days: (a, b) => b.days_active - a.days_active,
    price_asc: (a, b) => a.price - b.price,
    price_desc: (a, b) => b.price - a.price,
    relevance: (a, b) => b.relevance_score - a.relevance_score,
  }[sort] || ((a, b) => b.reach - a.reach);
  list = [...list].sort(cmp);
  const stats = bmComputeStats(s._products.filter((p) => includeNoise || true));
  return { stats, products: list };
});

/* PUT /api/benchmark/products/:id/noise — đánh dấu / bỏ đánh dấu nhiễu (thủ công) */
addRoute("PUT", "/api/benchmark/products/([^/]+)/noise", ({ params, body }) => {
  const { session, product } = bmFindProduct(params[0]);
  product.is_noise = body.is_noise ? 1 : 0;
  product.manual_override = 1;
  product.noise_reason = product.is_noise ? "Đánh dấu thủ công bởi người dùng" : null;
  const st = bmComputeStats(session._products);
  session.kept_count = st.kept; session.noise_count = st.noise; session.raw_count = st.raw;
  return product;
});
/* Một số bản UI có thể gọi POST thay vì PUT cho cùng thao tác — hỗ trợ cả hai */
addRoute("POST", "/api/benchmark/products/([^/]+)/noise", ({ params, body }) => {
  const { session, product } = bmFindProduct(params[0]);
  product.is_noise = body.is_noise ? 1 : 0;
  product.manual_override = 1;
  product.noise_reason = product.is_noise ? "Đánh dấu thủ công bởi người dùng" : null;
  const st = bmComputeStats(session._products);
  session.kept_count = st.kept; session.noise_count = st.noise; session.raw_count = st.raw;
  return product;
});

/* GET /api/benchmark/products/:id — chi tiết 1 ads/sản phẩm benchmark (kèm media/copies/page/lp) */
addRoute("GET", "/api/benchmark/products/([^/]+)", ({ params }) => bmFindProduct(params[0]).product);

/* GET /api/benchmark/sessions/:id/evaluations — danh sách sản phẩm đã chấm điểm */
addRoute("GET", "/api/benchmark/sessions/([^/]+)/evaluations", ({ params }) => bmFindSession(params[0])._evaluations);

/* POST /api/benchmark/parse-upload — bóc tách file CSV/Excel người dùng tải lên */
addRoute("POST", "/api/benchmark/parse-upload", ({ body }) => {
  // Mock: trả về 3 dòng mẫu để minh hoạ, giả lập kết quả đọc file `body.filename`
  const products = [
    { name: "Áo thun local brand cổ bẻ polo mini", source_url: "", cogs: 68000, price_expected: 189000, variants: "Đen, Trắng, Be / S-XL", material: "Cá sấu cotton", note: "Nhập từ file " + (body.filename || "upload"), image_url: bmPic("upl1"), media_count: 1 },
    { name: "Áo thun local brand tay dài raglan", source_url: "", cogs: 74000, price_expected: 209000, variants: "Đen phối trắng / S-XXL", material: "Cotton 4 chiều", note: "Nhập từ file " + (body.filename || "upload"), image_url: bmPic("upl2"), media_count: 1 },
    { name: "Áo thun local brand crop top nữ", source_url: "", cogs: 55000, price_expected: 159000, variants: "Hồng, Đen / S-L", material: "Cotton co giãn", note: "Nhập từ file " + (body.filename || "upload"), image_url: bmPic("upl3"), media_count: 1 },
  ];
  return { products };
});

/* POST /api/benchmark/enrich-url — bóc tách 1 URL sản phẩm nguồn */
const BM_BLOCKED_HOSTS = ["1688.com", "taobao.com"];
addRoute("POST", "/api/benchmark/enrich-url", ({ body }) => {
  let host = "";
  try { host = new URL(body.url).hostname.replace(/^www\./, ""); } catch (e) { host = ""; }
  if (BM_BLOCKED_HOSTS.some((h) => host.includes(h)) && Math.random() < 0.35) {
    return { ok: false, warning: "Trang yêu cầu đăng nhập hoặc chặn bot — vui lòng nhập tay thông tin sản phẩm này." };
  }
  const seedN = Math.abs(Array.from(host + body.url).reduce((a, c) => a + c.charCodeAt(0), 0)) % 1000;
  return {
    ok: true,
    name: "Áo thun local brand mẫu bóc tách từ " + (host || "nguồn"),
    source_url: body.url,
    image_url: bmPic("enrich" + seedN),
    media_count: 1 + (seedN % 3),
    variants: "Đen, Trắng / S-XXL",
    material: "Chất liệu: cotton 220gsm",
    note: "Tự động bóc tách — vui lòng kiểm tra lại giá và biến thể trước khi chấm điểm.",
    price: 120000 + (seedN % 15) * 10000,
    scrape_source: host.includes("1688") || host.includes("taobao") ? "playwright" : "http",
  };
});

/* POST /api/benchmark/sessions/:id/evaluate — chấm điểm 1..N sản phẩm mới bằng AI */
addRoute("POST", "/api/benchmark/sessions/([^/]+)/evaluate", ({ params, body }) => {
  const s = bmFindSession(params[0]);
  const results = (body.products || []).map((p) => {
    const scores = BM_CRITERIA_TEMPLATE.map(() => 4 + Math.floor(Math.random() * 6));
    const winScore = Math.round(scores.reduce((a, v, i) => a + v * BM_CRITERIA_TEMPLATE[i].weight, 0) / 10);
    const verdict = winScore >= 75 ? "scale" : winScore >= 60 ? "potential" : winScore >= 45 ? "consider" : "risky";
    const verdictLabel = { scale: "Nên scale ngay", potential: "Tiềm năng", consider: "Cân nhắc thử nghiệm", risky: "Rủi ro cao" }[verdict];
    const ev = {
      id: uid("ev"), session_id: s.id, status: "done",
      product_name: p.name || "Sản phẩm chưa đặt tên",
      image_url: p.image_url || bmPic(uid("ev")),
      source_url: p.source_url || null,
      price_expected: p.price_expected, cogs: p.cogs, currency: s.currency,
      win_score: winScore, verdict, verdict_label: verdictLabel,
      ai_source: BENCHMARK_META.ai_enabled ? "claude" : "rule",
      created_at: new Date().toISOString(),
      summary: `AI đã so sánh sản phẩm với ${s.kept_count} sản phẩm benchmark trong ngành “${s.category}” tại ${s.country_name}.`,
      criteria: bmCriteria(scores),
      matches: s._products.filter((x) => !x.is_noise).slice(0, 2).map((x) => ({
        name: x.product_name, reason: "Tương đồng về phân khúc giá và tệp khách hàng mục tiêu",
        similarity: Math.round((0.5 + Math.random() * 0.4) * 100) / 100,
      })),
      strengths: ["Phù hợp xu hướng hiện tại của ngành hàng.", "Có dư địa biên lợi nhuận nếu tối ưu giá vốn."],
      gaps: ["Cần bổ sung thêm ảnh/video để tăng độ tin cậy.", "Nên thử nghiệm nhỏ trước khi scale ngân sách lớn."],
    };
    s._evaluations.unshift(ev);
    return ev;
  });
  return { results };
});

/* GET /api/benchmark/evaluations/:id — chi tiết báo cáo chấm điểm */
addRoute("GET", "/api/benchmark/evaluations/([^/]+)", ({ params }) => bmFindEvaluation(params[0]).evaluation);

/* DELETE /api/benchmark/evaluations/:id */
addRoute("DELETE", "/api/benchmark/evaluations/([^/]+)", ({ params }) => {
  const { session, evaluation } = bmFindEvaluation(params[0]);
  session._evaluations = session._evaluations.filter((x) => x.id !== evaluation.id);
  return { ok: true };
});

const fmtVnd = (v) => {
  v = Number(v || 0);
  const en = window.I18N && window.I18N.lang === "en";
  if (v >= 1e9) return (v / 1e9).toFixed(2) + (en ? "B" : " tỷ");
  if (v >= 1e6) return (v / 1e6).toFixed(1) + (en ? "M" : " tr");
  if (v >= 1e3) return Math.round(v / 1e3) + "K";
  return String(Math.round(v));
};
const fmtVndFull = (v) => Number(v || 0).toLocaleString("vi-VN") + " ₫";
const fmtInt = (v) => Number(v || 0).toLocaleString("vi-VN");
const fmtPct = (v) => Number(v || 0).toFixed(1) + "%";

/* ============================ icons (Lucide paths) ============================ */
const ICONS = {
  dashboard: '<rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/>',
  search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
  package: '<path d="m7.5 4.27 9 5.15"/><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>',
  monitor: '<rect width="20" height="14" x="2" y="3" rx="2"/><line x1="8" x2="16" y1="21" y2="21"/><line x1="12" x2="12" y1="17" y2="21"/>',
  megaphone: '<path d="m3 11 18-5v12L3 14v-3z"/><path d="M11.6 16.8a3 3 0 1 1-5.8-1.6"/>',
  bell: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
  plus: '<path d="M5 12h14"/><path d="M12 5v14"/>',
  trash: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/>',
  pencil: '<path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/><path d="m15 5 4 4"/>',
  x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  check: '<path d="M20 6 9 17l-5-5"/>',
  checkCircle: '<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><path d="m9 11 3 3L22 4"/>',
  externalLink: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  trendingUp: '<polyline points="22 7 13.5 15.5 8.5 10.5 2 17"/><polyline points="16 7 22 7 22 13"/>',
  trendingDown: '<polyline points="22 17 13.5 8.5 8.5 13.5 2 7"/><polyline points="16 17 22 17 22 11"/>',
  dollar: '<line x1="12" x2="12" y1="2" y2="22"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>',
  cart: '<circle cx="8" cy="21" r="1"/><circle cx="19" cy="21" r="1"/><path d="M2.05 2.05h2l2.66 12.42a2 2 0 0 0 2 1.58h9.78a2 2 0 0 0 1.95-1.57l1.65-7.43H5.12"/>',
  users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
  eye: '<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/>',
  target: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/>',
  card: '<rect width="20" height="14" x="2" y="5" rx="2"/><line x1="2" x2="22" y1="10" y2="10"/>',
  calendar: '<path d="M8 2v4"/><path d="M16 2v4"/><rect width="18" height="18" x="3" y="4" rx="2"/><path d="M3 10h18"/>',
  building: '<rect width="16" height="20" x="4" y="2" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01"/><path d="M16 6h.01"/><path d="M12 6h.01"/><path d="M12 10h.01"/><path d="M8 10h.01"/><path d="M16 10h.01"/>',
  filter: '<polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/>',
  sort: '<path d="m21 16-4 4-4-4"/><path d="M17 20V4"/><path d="m3 8 4-4 4 4"/><path d="M7 4v16"/>',
  chevronLeft: '<path d="m15 18-6-6 6-6"/>',
  chevronRight: '<path d="m9 18 6-6-6-6"/>',
  chevronDown: '<path d="m6 9 6 6 6-6"/>',
  sparkles: '<path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .962 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.582a.5.5 0 0 1 0 .962L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.962 0z"/>',
  play: '<polygon points="6 3 20 12 6 21 6 3"/>',
  pause: '<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>',
  rocket: '<path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"/><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/>',
  store: '<path d="m2 7 4.41-4.41A2 2 0 0 1 7.83 2h8.34a2 2 0 0 1 1.42.59L22 7"/><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/><path d="M2 7h20"/>',
  layers: '<path d="m12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z"/><path d="m22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65"/><path d="m22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65"/>',
  star: '<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>',
  image: '<rect width="18" height="18" x="3" y="3" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/>',
  link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
  flame: '<path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z"/>',
  upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" x2="12" y1="3" y2="15"/>',
  radar: '<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/>',
};
function Icon({ name, className = "w-5 h-5", stroke = 2, fill = "none" }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill={fill} stroke={fill === "none" ? "currentColor" : "none"}
      strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round"
      dangerouslySetInnerHTML={{ __html: ICONS[name] || "" }} />
  );
}
function Rating({ value, className = "" }) {
  return (
    <span className={"inline-flex items-center gap-1 " + className}>
      <Icon name="star" className="w-4 h-4 text-amber-400" fill="currentColor" />
      <span className="text-ink font-medium tabular-nums">{value != null ? value : "—"}</span>
    </span>
  );
}

/* ============================ UI primitives ============================ */
function Button({ variant = "secondary", size = "md", icon, children, className = "", ...p }) {
  const base = "inline-flex items-center justify-center gap-2 font-medium rounded-[10px] transition-colors duration-150 disabled:opacity-50 disabled:cursor-not-allowed";
  const sizes = { sm: "text-[13px] px-3 h-9", md: "text-sm px-4 h-10" };
  const variants = {
    primary: "bg-primary text-white hover:bg-primaryhover",
    secondary: "bg-surface text-gray-700 border border-gray-300 hover:bg-hover hover:text-ink",
    danger: "bg-error text-white hover:brightness-95",
    ghost: "text-muted hover:bg-hover hover:text-ink",
  };
  return (
    <button className={`${base} ${sizes[size]} ${variants[variant]} ${className}`} {...p}>
      {icon && <Icon name={icon} className="w-4 h-4" />}
      {children}
    </button>
  );
}
function Card({ title, action, children, className = "", pad = true }) {
  return (
    <div className={`bg-surface border border-line rounded-xl shadow-soft ${className}`}>
      {(title || action) && (
        <div className="flex items-center justify-between px-6 pt-5 pb-3">
          {title && <h3 className="text-[18px] font-semibold text-ink">{title}</h3>}
          {action}
        </div>
      )}
      <div className={pad ? "px-6 pb-6" : ""}>{children}</div>
    </div>
  );
}
const TONES = {
  blue: "text-primary bg-softblue", green: "text-success bg-green-50",
  amber: "text-amber-700 bg-amber-50", red: "text-error bg-red-50",
  slate: "text-slate-600 bg-slate-100", gray: "text-muted bg-hover",
};
function Badge({ tone = "slate", children }) {
  return <span className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-semibold ${TONES[tone] || TONES.slate}`}>{children}</span>;
}
function KpiCard({ label, value, icon, delta, deltaGood }) {
  return (
    <div className="bg-surface border border-line rounded-xl shadow-soft p-5 hover:shadow-softmd transition-shadow duration-150">
      <div className="flex items-center justify-between">
        <span className="text-[13px] font-medium text-muted">{label}</span>
        {icon && <span className="w-9 h-9 rounded-[9px] bg-hover text-muted flex items-center justify-center"><Icon name={icon} className="w-[18px] h-[18px]" /></span>}
      </div>
      <div className="text-[28px] font-bold text-ink mt-3 leading-none tracking-tight">{value}</div>
      {delta != null && (
        <div className={`text-[13px] font-medium mt-2 flex items-center gap-1 ${deltaGood == null ? "text-muted" : deltaGood ? "text-success" : "text-error"}`}>
          {deltaGood != null && <Icon name={deltaGood ? "trendingUp" : "trendingDown"} className="w-3.5 h-3.5" />}
          {delta}
        </div>
      )}
    </div>
  );
}
function Field({ label, children, hint }) {
  return (
    <label className="block">
      {label && <span className="block text-[13px] font-medium text-gray-700 mb-1.5">{label}</span>}
      {children}
      {hint && <span className="block text-xs text-muted mt-1">{hint}</span>}
    </label>
  );
}
const inputCls = "w-full h-11 px-3.5 rounded-[10px] border border-gray-300 text-sm bg-surface outline-none focus:border-primary focus:ring-4 focus:ring-primary/15 transition";
function Input(p) { return <input className={inputCls} {...p} />; }
function Textarea(p) { return <textarea className={inputCls + " h-auto py-2.5 min-h-[96px]"} {...p} />; }
function Select({ children, ...p }) {
  return <div className="relative"><select className={inputCls + " appearance-none pr-9"} {...p}>{children}</select>
    <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted"><Icon name="chevronDown" className="w-4 h-4" /></span></div>;
}
function Modal({ title, onClose, children, wide }) {
  useEffect(() => {
    const h = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center p-4 pt-16 bg-black/30" onClick={onClose}>
      <div className={`bg-surface rounded-xl shadow-xl w-full ${wide ? "max-w-3xl" : "max-w-lg"} fade-in`} onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-line">
          <h3 className="text-lg font-semibold text-ink">{title}</h3>
          <button onClick={onClose} className="text-muted hover:text-ink"><Icon name="x" /></button>
        </div>
        <div className="px-6 py-5 max-h-[70vh] overflow-y-auto">{children}</div>
      </div>
    </div>
  );
}
function Drawer({ title, subtitle, onClose, children, footer, width = "max-w-2xl" }) {
  useEffect(() => {
    const h = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, []);
  return (
    <div className="fixed inset-0 z-50">
      <div className="absolute inset-0 bg-black/30 fade-in" onClick={onClose} />
      <div className={`absolute right-0 top-0 h-full w-full ${width} bg-surface shadow-2xl flex flex-col drawer-in`}>
        <header className="flex items-center justify-between px-6 h-16 border-b border-line shrink-0">
          <div className="min-w-0">
            <h3 className="text-lg font-semibold text-ink truncate">{title}</h3>
            {subtitle && <div className="text-[13px] text-muted truncate">{subtitle}</div>}
          </div>
          <button onClick={onClose} className="w-9 h-9 rounded-[10px] border border-gray-300 text-muted hover:bg-hover hover:text-ink flex items-center justify-center shrink-0"><Icon name="x" /></button>
        </header>
        <div className="flex-1 overflow-y-auto px-6 py-5">{children}</div>
        {footer && <div className="px-6 py-4 border-t border-line shrink-0 bg-surface">{footer}</div>}
      </div>
    </div>
  );
}
function Empty({ icon = "package", title, hint, action }) {
  return (
    <div className="flex flex-col items-center justify-center text-center py-16 px-6">
      <div className="w-14 h-14 rounded-full bg-hover text-muted flex items-center justify-center mb-4"><Icon name={icon} className="w-7 h-7" /></div>
      <p className="text-ink font-semibold">{title}</p>
      {hint && <p className="text-muted text-sm mt-1 max-w-md">{hint}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
function Spinner() {
  return <div className="flex items-center justify-center py-16"><div className="w-8 h-8 border-[3px] border-line border-t-primary rounded-full animate-spin" /></div>;
}

/* ============================ Chart (Chart.js wrapper) ============================ */
const CHART_COLORS = ["#2563EB", "#16A34A", "#F59E0B", "#8B5CF6", "#EC4899"];
function ChartView({ type, data, options, height = 260 }) {
  const ref = useRef(null);
  const inst = useRef(null);
  /* Chart.js vẽ chú thích lên canvas nên bộ i18n (vốn bọc React.createElement)
     không với tới được -> dịch nhãn dataset ngay tại đây. */
  const localized = useMemo(() => {
    const t = (v) => (typeof v === "string" && window.I18N ? window.I18N.t(v) : v);
    if (!data || !data.datasets) return data;
    return { ...data, datasets: data.datasets.map((d) => ({ ...d, label: t(d.label) })) };
  }, [JSON.stringify(data)]);
  useEffect(() => {
    if (!ref.current) return;
    inst.current = new ChartLib(ref.current, {
      type, data: localized,
      options: {
        responsive: true, maintainAspectRatio: false,
        plugins: { legend: { labels: { font: { family: "Inter", size: 12 }, color: "#6B7280", boxWidth: 12, boxHeight: 12, padding: 16 } } },
        scales: type === "doughnut" ? {} : {
          x: { grid: { display: false }, ticks: { font: { family: "Inter", size: 11 }, color: "#9CA3AF", maxRotation: 0, autoSkip: true, maxTicksLimit: 8 } },
          y: { grid: { color: "#F1F3F7" }, border: { display: false }, ticks: { font: { family: "Inter", size: 11 }, color: "#9CA3AF" } },
        },
        ...options,
      },
    });
    return () => inst.current && inst.current.destroy();
  }, [JSON.stringify(localized), type]);
  return <div style={{ height }}><canvas ref={ref} /></div>;
}

/* ============================ DataTable ============================ */
function DataTable({ columns, rows, searchKeys = [], filters = [], pageSize = 8, rightAction, onRowClick }) {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState({ key: null, dir: 1 });
  const [active, setActive] = useState({});
  const [page, setPage] = useState(1);

  const filtered = useMemo(() => {
    let r = rows;
    if (q) { const s = q.toLowerCase(); r = r.filter((row) => searchKeys.some((k) => String(row[k] ?? "").toLowerCase().includes(s))); }
    Object.entries(active).forEach(([k, v]) => { if (v != null && v !== "") r = r.filter((row) => String(row[k]) === String(v)); });
    if (sort.key) { r = [...r].sort((a, b) => { const x = a[sort.key], y = b[sort.key]; if (x == null) return 1; if (y == null) return -1; return (typeof x === "number" ? x - y : String(x).localeCompare(String(y))) * sort.dir; }); }
    return r;
  }, [rows, q, active, sort]);

  const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const cur = Math.min(page, pages);
  const view = filtered.slice((cur - 1) * pageSize, cur * pageSize);
  const toggleSort = (k) => setSort((s) => s.key === k ? { key: k, dir: -s.dir } : { key: k, dir: 1 });

  return (
    <div>
      {(searchKeys.length > 0 || filters.length > 0 || rightAction) && (
        <div className="flex flex-wrap items-center gap-2 mb-4">
          {searchKeys.length > 0 && (
            <div className="relative">
              <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted"><Icon name="search" className="w-4 h-4" /></span>
              <input value={q} onChange={(e) => { setQ(e.target.value); setPage(1); }} placeholder="Tìm kiếm…"
                className="h-9 pl-9 pr-3 w-64 rounded-[10px] border border-gray-300 text-sm outline-none focus:border-primary focus:ring-4 focus:ring-primary/15" />
            </div>
          )}
          {filters.map((f) => (
            <div key={f.key} className="flex items-center gap-1 flex-wrap">
              {[{ label: f.label + ": Tất cả", value: "" }, ...f.options.map((o) => ({ label: o, value: o }))].map((o) => (
                <button key={o.value} onClick={() => { setActive((a) => ({ ...a, [f.key]: o.value })); setPage(1); }}
                  className={`h-9 px-3 rounded-full text-[13px] font-medium border transition-colors ${String(active[f.key] || "") === String(o.value) ? "bg-softblue border-primary text-primary" : "bg-surface border-gray-300 text-gray-600 hover:bg-hover"}`}>
                  {o.label}
                </button>
              ))}
            </div>
          ))}
          <div className="ml-auto">{rightAction}</div>
        </div>
      )}
      <div className="overflow-x-auto border border-line rounded-xl">
        <table className="w-full text-sm">
          <thead className="bg-[#F9FAFB] sticky top-0">
            <tr>
              {columns.map((c) => (
                <th key={c.key} className={`px-4 py-3 text-[13px] font-semibold text-gray-600 whitespace-nowrap ${c.align === "right" ? "text-right" : "text-left"} ${c.headClass || c.cellClass || ""}`}>
                  <button className={`inline-flex items-center gap-1.5 ${c.sortable === false ? "cursor-default" : "hover:text-ink"}`}
                    onClick={() => c.sortable !== false && toggleSort(c.key)}>
                    {c.label}
                    {c.sortable !== false && <Icon name={sort.key === c.key ? "sort" : "sort"} className={`w-3.5 h-3.5 ${sort.key === c.key ? "text-primary" : "text-gray-300"}`} />}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {view.length === 0 && <tr><td colSpan={columns.length} className="px-4 py-10 text-center text-muted">Không có dữ liệu.</td></tr>}
            {view.map((row, i) => (
              <tr key={i} onClick={onRowClick ? () => onRowClick(row) : undefined}
                className={"transition-colors " + (onRowClick ? "row-click hover:bg-softblue/60" : "hover:bg-hover")}>
                {columns.map((c) => (
                  <td key={c.key} className={`px-4 py-3.5 text-ink ${c.align === "right" ? "text-right tabular-nums" : "text-left"} ${c.cellClass || ""}`}>
                    {c.render ? c.render(row) : row[c.key]}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between mt-4 text-[13px] text-muted">
        <span>{filtered.length} bản ghi</span>
        <div className="flex items-center gap-1">
          <Button size="sm" variant="ghost" disabled={cur <= 1} onClick={() => setPage(cur - 1)}><Icon name="chevronLeft" className="w-4 h-4" /></Button>
          <span className="px-2">Trang {cur}/{pages}</span>
          <Button size="sm" variant="ghost" disabled={cur >= pages} onClick={() => setPage(cur + 1)}><Icon name="chevronRight" className="w-4 h-4" /></Button>
        </div>
      </div>
    </div>
  );
}

/* ============================ Layout ============================ */
const NAV = [
  { id: "dashboard", label: "Tổng quan", icon: "dashboard" },
  { id: "market", label: "Nghiên cứu ngách", icon: "search" },
  { id: "benchmark", label: "Benchmark & SP tiềm năng", icon: "target" },
  { id: "products", label: "Sản phẩm", icon: "package" },
  { id: "landing", label: "Landing page", icon: "monitor" },
  { id: "ads", label: "Quảng cáo", icon: "megaphone" },
];
function Sidebar({ route, setRoute }) {
  return (
    <aside className="w-60 shrink-0 bg-surface border-r border-line flex flex-col h-screen sticky top-0">
      <div className="h-16 flex items-center gap-2.5 px-5 border-b border-line">
        <div className="w-8 h-8 rounded-lg bg-primary text-white flex items-center justify-center"><Icon name="layers" className="w-[18px] h-[18px]" /></div>
        <span className="font-bold text-ink text-[17px]">Winora</span>
      </div>
      <nav className="flex-1 py-3 px-2 space-y-0.5">
        {NAV.map((n) => (
          <button key={n.id} onClick={() => setRoute(n.id)}
            className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium text-left transition-colors ${route === n.id ? "bg-softblue text-primary" : "text-gray-700 hover:bg-hover"}`}>
            <Icon name={n.icon} className="w-[18px] h-[18px]" />{n.label}
          </button>
        ))}
      </nav>
      <div className="p-3 border-t border-line">
        <div className="flex items-center gap-3 px-2 py-1.5">
          <div className="w-9 h-9 rounded-full bg-softblue text-primary font-semibold flex items-center justify-center text-sm">RA</div>
          <div className="min-w-0"><div className="text-sm font-medium text-ink truncate">RnD AI</div><div className="text-xs text-muted truncate">rnd_ai1@relipa…</div></div>
        </div>
      </div>
    </aside>
  );
}
/* Chuyển ngôn ngữ VI / EN — lưu vào localStorage rồi reload để áp dụng toàn app */
function LangSwitch() {
  const cur = (window.I18N && window.I18N.lang) || "vi";
  return (
    <div className="flex items-center rounded-[10px] border border-gray-300 overflow-hidden h-9">
      {[["vi", "VI"], ["en", "EN"]].map(([code, label]) => (
        <button key={code} onClick={() => code !== cur && window.I18N.setLang(code)}
          className={`px-2.5 h-full text-xs font-semibold transition-colors ${
            cur === code ? "bg-primary text-white" : "text-muted hover:bg-hover"}`}>
          {label}
        </button>
      ))}
    </div>
  );
}

function TopBar({ title }) {
  return (
    <header className="h-16 shrink-0 bg-surface/80 backdrop-blur border-b border-line flex items-center gap-4 px-6 sticky top-0 z-30">
      <div className="flex items-center gap-2 text-[13px] text-muted">
        <Icon name="layers" className="w-3.5 h-3.5" /><span>Winora</span><span>/</span>
        <span className="text-gray-700 font-medium">{title}</span>
      </div>
      <div className="ml-auto flex items-center gap-3">
        <div className="relative hidden md:block">
          <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted"><Icon name="search" className="w-4 h-4" /></span>
          <input placeholder="Tìm kiếm…" className="h-9 pl-9 pr-3 w-56 rounded-[10px] border border-gray-300 text-sm outline-none focus:border-primary focus:ring-4 focus:ring-primary/15" />
        </div>
        <LangSwitch />
        <button className="relative w-9 h-9 rounded-[10px] border border-gray-300 text-gray-600 hover:bg-hover flex items-center justify-center">
          <Icon name="bell" className="w-[18px] h-[18px]" />
          <span className="absolute top-2 right-2.5 w-1.5 h-1.5 bg-error rounded-full" />
        </button>
        <div className="w-9 h-9 rounded-full bg-softblue text-primary font-semibold flex items-center justify-center text-sm">RA</div>
      </div>
    </header>
  );
}
function PageHead({ title, subtitle, action }) {
  return (
    <div className="flex items-start justify-between mb-6">
      <div><h1 className="text-[32px] font-bold text-ink tracking-tight leading-tight">{title}</h1>
        {subtitle && <p className="text-muted text-[15px] mt-1">{subtitle}</p>}</div>
      {action}
    </div>
  );
}

/* ============================ toast ============================ */
const ToastCtx = React.createContext(() => {});
function useToast() { return React.useContext(ToastCtx); }

/* ============================ Pages ============================ */
const PRESETS = [{ label: "7 ngày", days: 7 }, { label: "30 ngày", days: 30 }, { label: "90 ngày", days: 90 }];
function TimeFilter({ days, custom, range, onPreset, onCustom }) {
  const [open, setOpen] = useState(false);
  const [s, setS] = useState(range?.start || "");
  const [e, setE] = useState(range?.end || "");
  useEffect(() => { if (range) { setS(range.start); setE(range.end); } }, [range?.start, range?.end]);
  return (
    <div className="flex items-center gap-2">
      <span className="text-[13px] text-muted flex items-center gap-1.5"><Icon name="calendar" className="w-4 h-4" />Khoảng thời gian:</span>
      <div className="flex items-center gap-1 bg-surface border border-line rounded-[10px] p-1">
        {PRESETS.map((p) => (
          <button key={p.days} onClick={() => { onPreset(p.days); setOpen(false); }}
            className={`h-8 px-3 rounded-lg text-[13px] font-medium transition-colors ${!custom && days === p.days ? "bg-softblue text-primary" : "text-gray-600 hover:bg-hover"}`}>{p.label}</button>
        ))}
        <div className="relative">
          <button onClick={() => setOpen((o) => !o)}
            className={`h-8 px-3 rounded-lg text-[13px] font-medium flex items-center gap-1 transition-colors ${custom ? "bg-softblue text-primary" : "text-gray-600 hover:bg-hover"}`}>
            {custom ? `${range.start} → ${range.end}` : "Tuỳ chọn"}<Icon name="chevronDown" className="w-3.5 h-3.5" />
          </button>
          {open && (
            <div className="absolute right-0 mt-2 z-40 bg-surface border border-line rounded-xl shadow-lg p-4 w-72">
              <Field label="Từ ngày"><input type="date" className={inputCls} value={s} onChange={(x) => setS(x.target.value)} /></Field>
              <div className="h-3" />
              <Field label="Đến ngày"><input type="date" className={inputCls} value={e} onChange={(x) => setE(x.target.value)} /></Field>
              <div className="flex justify-end gap-2 mt-3">
                <Button size="sm" onClick={() => setOpen(false)}>Huỷ</Button>
                <Button size="sm" variant="primary" onClick={() => { if (s && e) { onCustom(s, e); setOpen(false); } }}>Áp dụng</Button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
function Funnel({ f }) {
  const stages = [
    { label: "Lead", value: f.leads, color: "#64748B" },
    { label: "Đã xác nhận", value: f.confirmed, color: "#2563EB", sub: [{ label: "Đã thanh toán", value: f.paid, color: "#16A34A" }, { label: "Chưa thanh toán", value: f.unpaid, color: "#F59E0B" }] },
    { label: "Đang vận chuyển", value: f.shipping, color: "#8B5CF6" },
    { label: "Đã vận chuyển", value: f.shipped, color: "#0EA5E9" },
    { label: "Hoàn hàng", value: f.returned, color: "#DC2626" },
  ];
  const base = f.leads || 1;
  return (
    <div className="space-y-4">
      {stages.map((st, i) => {
        const pct = st.value / base * 100;
        const step = i > 0 && stages[i - 1].value ? st.value / stages[i - 1].value * 100 : null;
        return (
          <div key={st.label}>
            <div className="flex justify-between items-baseline mb-1.5">
              <span className="text-sm font-medium text-ink">{st.label}</span>
              <span className="text-[13px] text-muted">
                <b className="text-ink">{fmtInt(st.value)}</b> · {pct.toFixed(0)}% tổng
                {step != null && <span className={`ml-2 ${st.label === "Hoàn hàng" ? "text-error" : "text-muted"}`}>({step.toFixed(0)}% từ bước trước)</span>}
              </span>
            </div>
            <div className="h-9 rounded-lg bg-hover overflow-hidden">
              <div className="h-full rounded-lg flex items-center px-3 text-white text-xs font-semibold transition-all duration-300"
                style={{ width: Math.max(pct, 5) + "%", background: st.color }}>{fmtInt(st.value)}</div>
            </div>
            {st.sub && (
              <div className="flex flex-wrap gap-4 mt-2 ml-1">
                {st.sub.map((x) => (
                  <span key={x.label} className="text-[13px] text-muted flex items-center gap-1.5">
                    <span className="w-2.5 h-2.5 rounded-full" style={{ background: x.color }} />{x.label}: <b className="text-ink">{fmtInt(x.value)}</b>
                  </span>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
/* Sinh chuỗi doanh thu/đơn theo ngày quanh 2 tổng cho trước, dao động nhẹ
   theo sin để biểu đồ trông tự nhiên — dữ liệu tĩnh, không phụ thuộc API. */
function buildMockDaily(startDate, numDays, totalRevenue, totalOrders) {
  const weights = Array.from({ length: numDays }, (_, i) => 1 + 0.4 * Math.sin(i / 2.3) + 0.15 * Math.sin(i * 1.7));
  const sum = weights.reduce((a, b) => a + b, 0);
  const start = new Date(startDate + "T00:00:00Z");
  return weights.map((w, i) => {
    const date = new Date(start);
    date.setUTCDate(date.getUTCDate() + i);
    return {
      date: date.toISOString().slice(0, 10),
      revenue: Math.round((totalRevenue * w) / sum / 1000) * 1000,
      orders: Math.round((totalOrders * w) / sum),
    };
  });
}
const MOCK_DASHBOARD = {
  range: { start: "2026-07-09", end: "2026-08-07" },
  kpis: {
    revenue: 361300000,
    aov: 380000,
    orders_confirmed: 1072,
    close_rate: 64.0,
    return_rate: 16.9,
    returned_amount: 50200000,
  },
  funnel: { leads: 1676, confirmed: 1072, paid: 950, unpaid: 122, shipping: 870, shipped: 703, returned: 119 },
  channels: [
    { channel: "TikTok Shop", revenue: 130000000 },
    { channel: "Shopee", revenue: 95000000 },
    { channel: "Website riêng", revenue: 75000000 },
    { channel: "Facebook Shop", revenue: 45000000 },
    { channel: "Lazada", revenue: 16300000 },
  ],
  daily: buildMockDaily("2026-07-09", 30, 361300000, 1072),
};
const MOCK_OVERVIEW = {
  ads: { kill: 2 },
  campaigns: [
    { name: "TikTok - Áo polo nam hè", status: "active", kpi: { roas: 3.42, spend: 18500000, revenue: 63270000 }, ai_verdict: "scale" },
    { name: "Facebook - Set đồ bộ nữ", status: "active", kpi: { roas: 1.15, spend: 12300000, revenue: 14145000 }, ai_verdict: "kill" },
    { name: "Shopee Ads - Quần jean nam", status: "paused", kpi: { roas: 2.05, spend: 8200000, revenue: 16810000 }, ai_verdict: "hold" },
    { name: "TikTok - Đồng phục nhóm", status: "active", kpi: { roas: 0.87, spend: 9600000, revenue: 8352000 }, ai_verdict: "kill" },
    { name: "Facebook - Polo đoàn thanh niên", status: "active", kpi: { roas: 2.78, spend: 14000000, revenue: 38920000 }, ai_verdict: "optimize" },
  ],
};
function Dashboard() {
  const [days, setDays] = useState(30);
  const [custom, setCustom] = useState(null);
  // Tạm thời hardcode dữ liệu (backend /api/dashboard, /api/overview chưa sẵn sàng).
  // Bỏ comment 2 useEffect bên dưới và xoá 2 dòng useState(MOCK_*) khi API đã có.
  const [d, setD] = useState(MOCK_DASHBOARD);
  const [ov, setOv] = useState(MOCK_OVERVIEW);
  // useEffect(() => {
  //   setD(null);
  //   const qs = custom ? `?start=${custom.start}&end=${custom.end}` : `?days=${days}`;
  //   api("/api/dashboard" + qs).then(setD);
  // }, [days, custom]);
  // useEffect(() => { api("/api/overview").then(setOv); }, []);

  const k = d?.kpis, f = d?.funnel;
  return (
    <div className="fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4 mb-6">
        <div><h1 className="text-[32px] font-bold text-ink tracking-tight leading-tight">Tổng quan</h1>
          <p className="text-muted text-[15px] mt-1">Luồng đơn hàng & hiệu quả kinh doanh {d ? `· ${d.range.start} → ${d.range.end}` : ""}</p></div>
        <TimeFilter days={days} custom={custom} range={d?.range}
          onPreset={(n) => { setCustom(null); setDays(n); }}
          onCustom={(s, e) => setCustom({ start: s, end: e })} />
      </div>

      {!d ? <Spinner /> : (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
            <KpiCard label="Doanh thu (đã thu)" value={fmtVnd(k.revenue) + " ₫"} icon="dollar" delta={"AOV " + fmtVnd(k.aov) + " ₫"} />
            <KpiCard label="Đơn đã xác nhận" value={fmtInt(k.orders_confirmed)} icon="cart" delta={fmtInt(f.leads) + " lead"} />
            <KpiCard label="Tỉ lệ chốt đơn" value={fmtPct(k.close_rate)} icon="target" delta="xác nhận / lead" />
            <KpiCard label="Tỉ lệ hoàn hàng" value={fmtPct(k.return_rate)} icon="package" delta={fmtVnd(k.returned_amount) + " ₫ hoàn"} deltaGood={k.return_rate <= 10} />
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
            <Card title="Luồng đơn theo trạng thái" className="lg:col-span-2"
              action={<Badge tone="slate">{fmtInt(f.leads)} lead → {fmtInt(f.shipped)} giao</Badge>}>
              <Funnel f={f} />
            </Card>
            <Card title="Doanh thu theo kênh">
              {d.channels.some((c) => c.revenue > 0)
                ? <ChartView type="doughnut" height={300} data={{ labels: d.channels.map((c) => c.channel), datasets: [{ data: d.channels.map((c) => c.revenue), backgroundColor: CHART_COLORS, borderWidth: 0 }] }} options={{ cutout: "62%", plugins: { legend: { position: "bottom" } } }} />
                : <Empty icon="dollar" title="Chưa có doanh thu trong kỳ" />}
            </Card>
          </div>

          <Card title="Doanh thu & đơn theo ngày" className="mb-6">
            <ChartView type="line" height={280} data={{
              labels: d.daily.map((r) => r.date.slice(5)),
              datasets: [
                { label: "Doanh thu", data: d.daily.map((r) => r.revenue), borderColor: "#2563EB", backgroundColor: "rgba(37,99,235,.08)", fill: true, tension: .35, pointRadius: 0, borderWidth: 2, yAxisID: "y" },
                { label: "Đơn xác nhận", data: d.daily.map((r) => r.orders), borderColor: "#16A34A", tension: .35, pointRadius: 0, borderWidth: 2, yAxisID: "y1" },
              ],
            }} options={{ scales: { y: { ticks: { callback: (v) => fmtVnd(v) } }, y1: { position: "right", grid: { display: false } } } }} />
          </Card>

          {ov && ov.ads.kill > 0 && (
            <div className="mb-6 flex items-center gap-3 bg-red-50 border border-red-200 text-error rounded-xl px-4 py-3 text-sm">
              <Icon name="megaphone" className="w-5 h-5" /><span><b>{ov.ads.kill} campaign</b> được AI gợi ý KILL — vào mục Quảng cáo để xử lý.</span>
            </div>
          )}
          {ov && (
            <Card title="Chiến dịch quảng cáo" action={<Badge tone="blue">{ov.campaigns.length} campaign</Badge>}>
              <DataTable
                columns={[
                  { key: "name", label: "Campaign", render: (r) => <span className="font-medium">{r.name}</span> },
                  { key: "status", label: "Trạng thái", render: (r) => <StatusBadge s={r.status} /> },
                  { key: "roas", label: "ROAS", align: "right", render: (r) => <b>{r.kpi.roas.toFixed(2)}</b>, sortable: false },
                  { key: "spend", label: "Chi tiêu", align: "right", render: (r) => fmtVnd(r.kpi.spend) + " ₫", sortable: false },
                  { key: "revenue", label: "Doanh thu", align: "right", render: (r) => fmtVnd(r.kpi.revenue) + " ₫", sortable: false },
                  { key: "ai_verdict", label: "AI", render: (r) => r.ai_verdict ? <VerdictBadge v={r.ai_verdict} /> : <span className="text-muted">—</span> },
                ]}
                rows={ov.campaigns} searchKeys={["name"]} pageSize={5} />
            </Card>
          )}
        </>
      )}
    </div>
  );
}

function StatusBadge({ s }) {
  const map = { active: ["green", "Đang chạy"], paused: ["amber", "Tạm dừng"], ended: ["slate", "Kết thúc"], draft: ["slate", "Nháp"], published: ["blue", "Đã xuất bản"] };
  const [tone, label] = map[s] || ["slate", s];
  return <Badge tone={tone}>{label}</Badge>;
}
function VerdictBadge({ v }) {
  const map = { scale: ["green", "SCALE"], hold: ["blue", "HOLD"], optimize: ["amber", "OPTIMIZE"], kill: ["red", "KILL"] };
  const [tone, label] = map[v] || ["slate", v];
  return <Badge tone={tone}>{label}</Badge>;
}

/* ---------------- Market ---------------- */
/* Màn hình “Nghiên cứu thị trường” theo ngành hàng rộng đã được thay bằng
   MODULE 1C — Nghiên cứu ngách hẹp (web/niche.jsx, đăng ký qua window.NichePage). */

function Products() {
  const [data, setData] = useState(null);
  const [edit, setEdit] = useState(null);
  const toast = useToast();
  const load = () => api("/api/products").then(setData);
  useEffect(() => { load(); }, []);
  if (!data) return <Spinner />;
  const st = data.stats;
  return (
    <div className="fade-in">
      <PageHead title="Quản lý sản phẩm" subtitle="Sản phẩm, biến thể, giá & tồn kho"
        action={<Button variant="primary" icon="plus" onClick={() => setEdit({})}>Thêm sản phẩm</Button>} />
      <div className="grid grid-cols-3 gap-4 mb-6">
        <KpiCard label="Số sản phẩm" value={st.products} icon="package" />
        <KpiCard label="Tổng tồn kho" value={fmtInt(st.total_stock)} icon="store" />
        <KpiCard label="Tổng đã bán" value={fmtInt(st.total_sold)} icon="checkCircle" />
      </div>
      <Card title="Danh sách sản phẩm">
        <DataTable
          searchKeys={["name", "category", "material"]}
          filters={[{ key: "category", label: "Danh mục", options: data.categories }, { key: "status", label: "Trạng thái", options: ["active", "draft", "archived"] }]}
          columns={[
            { key: "name", label: "Sản phẩm", render: (r) => <div><div className="font-medium text-ink">{r.name}</div><div className="text-xs text-muted">{r.material || "—"}</div></div> },
            { key: "category", label: "Danh mục", render: (r) => <Badge tone="slate">{r.category || "—"}</Badge> },
            { key: "sell_price", label: "Giá bán", align: "right", render: (r) => fmtVndFull(r.sell_price) },
            { key: "cost_price", label: "Giá nhập", align: "right", render: (r) => fmtVnd(r.cost_price) + " ₫" },
            { key: "total_stock", label: "Tồn", align: "right", render: (r) => fmtInt(r.total_stock) },
            { key: "total_sold", label: "Đã bán", align: "right", render: (r) => fmtInt(r.total_sold) },
            { key: "variant_count", label: "Biến thể", align: "right" },
            { key: "status", label: "TT", render: (r) => <StatusBadge s={r.status} /> },
            { key: "_", label: "", sortable: false, render: (r) => <Button size="sm" variant="ghost" icon="pencil" onClick={() => setEdit(r)}>Sửa</Button> },
          ]}
          rows={data.products} pageSize={8} />
      </Card>
      {edit && <ProductModal product={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); toast("Đã lưu sản phẩm"); }} onDeleted={() => { setEdit(null); load(); toast("Đã xoá"); }} />}
    </div>
  );
}
function ProductModal({ product, onClose, onSaved, onDeleted }) {
  const isNew = !product.id;
  const [f, setF] = useState({ name: "", category: "", material: "", description: "", cost_price: 0, sell_price: 0, status: "active", ...product });
  const [variants, setVariants] = useState([]);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const loadV = () => product.id && api(`/api/products/${product.id}/variants`).then(setVariants);
  useEffect(() => { loadV(); }, []);
  const save = async () => {
    const body = { name: f.name, category: f.category, material: f.material, description: f.description, cost_price: Number(f.cost_price), sell_price: Number(f.sell_price), status: f.status };
    if (isNew) await api("/api/products", { method: "POST", body });
    else await api(`/api/products/${product.id}`, { method: "PUT", body });
    onSaved();
  };
  return (
    <Modal title={isNew ? "Thêm sản phẩm" : "Sửa sản phẩm"} onClose={onClose} wide>
      <div className="grid grid-cols-2 gap-4">
        <Field label="Tên sản phẩm"><Input value={f.name} onChange={set("name")} /></Field>
        <Field label="Danh mục"><Input value={f.category} onChange={set("category")} /></Field>
        <Field label="Chất liệu"><Input value={f.material} onChange={set("material")} /></Field>
        <Field label="Trạng thái"><Select value={f.status} onChange={set("status")}><option value="active">active</option><option value="draft">draft</option><option value="archived">archived</option></Select></Field>
        <Field label="Giá nhập (₫)"><Input type="number" value={f.cost_price} onChange={set("cost_price")} /></Field>
        <Field label="Giá bán (₫)"><Input type="number" value={f.sell_price} onChange={set("sell_price")} /></Field>
        <div className="col-span-2"><Field label="Mô tả"><Textarea value={f.description || ""} onChange={set("description")} /></Field></div>
      </div>
      {!isNew && (
        <div className="mt-6">
          <div className="flex items-center justify-between mb-2"><span className="font-semibold text-ink">Biến thể ({variants.length})</span>
            <AddVariant productId={product.id} onAdded={loadV} /></div>
          <div className="border border-line rounded-xl overflow-hidden">
            <table className="w-full text-sm"><thead className="bg-[#F9FAFB]"><tr>
              {["Màu", "Size", "Tồn", "Đã bán", ""].map((h) => <th key={h} className="px-3 py-2 text-left text-[13px] font-semibold text-gray-600">{h}</th>)}
            </tr></thead><tbody className="divide-y divide-line">
              {variants.map((v) => <VariantRow key={v.id} v={v} onChanged={loadV} />)}
              {variants.length === 0 && <tr><td colSpan={5} className="px-3 py-6 text-center text-muted">Chưa có biến thể.</td></tr>}
            </tbody></table>
          </div>
        </div>
      )}
      <div className="flex justify-between gap-2 pt-6">
        {!isNew ? <Button variant="danger" icon="trash" onClick={async () => { if (confirm("Xoá sản phẩm?")) { await api(`/api/products/${product.id}`, { method: "DELETE" }); onDeleted(); } }}>Xoá</Button> : <span />}
        <div className="flex gap-2"><Button onClick={onClose}>Huỷ</Button><Button variant="primary" onClick={save}>Lưu</Button></div>
      </div>
    </Modal>
  );
}
function AddVariant({ productId, onAdded }) {
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ color: "", size: "", stock: 0, sold: 0 });
  if (!open) return <Button size="sm" icon="plus" onClick={() => setOpen(true)}>Thêm biến thể</Button>;
  return (
    <div className="flex items-center gap-2">
      <input className="h-9 w-24 px-2 rounded-lg border border-gray-300 text-sm" placeholder="Màu" value={f.color} onChange={(e) => setF({ ...f, color: e.target.value })} />
      <input className="h-9 w-16 px-2 rounded-lg border border-gray-300 text-sm" placeholder="Size" value={f.size} onChange={(e) => setF({ ...f, size: e.target.value })} />
      <input className="h-9 w-16 px-2 rounded-lg border border-gray-300 text-sm" type="number" placeholder="Tồn" value={f.stock} onChange={(e) => setF({ ...f, stock: e.target.value })} />
      <Button size="sm" variant="primary" onClick={async () => { await api(`/api/products/${productId}/variants`, { method: "POST", body: { ...f, stock: Number(f.stock), sold: Number(f.sold) } }); setF({ color: "", size: "", stock: 0, sold: 0 }); setOpen(false); onAdded(); }}>Lưu</Button>
    </div>
  );
}
function VariantRow({ v, onChanged }) {
  const [e, setE] = useState({ stock: v.stock, sold: v.sold });
  const dirty = e.stock != v.stock || e.sold != v.sold;
  return (
    <tr className="hover:bg-hover">
      <td className="px-3 py-2">{v.color}</td>
      <td className="px-3 py-2">{v.size}</td>
      <td className="px-3 py-2"><input className="h-8 w-20 px-2 rounded border border-gray-300 text-sm" type="number" value={e.stock} onChange={(x) => setE({ ...e, stock: x.target.value })} /></td>
      <td className="px-3 py-2"><input className="h-8 w-20 px-2 rounded border border-gray-300 text-sm" type="number" value={e.sold} onChange={(x) => setE({ ...e, sold: x.target.value })} /></td>
      <td className="px-3 py-2 text-right">
        {dirty && <Button size="sm" variant="primary" onClick={async () => { await api(`/api/variants/${v.id}`, { method: "PUT", body: { color: v.color, size: v.size, stock: Number(e.stock), sold: Number(e.sold) } }); onChanged(); }}>Lưu</Button>}
        <button className="ml-1 text-muted hover:text-error" onClick={async () => { await api(`/api/variants/${v.id}`, { method: "DELETE" }); onChanged(); }}><Icon name="trash" className="w-4 h-4" /></button>
      </td>
    </tr>
  );
}

/* ---------------- Landing ---------------- */
function Landing() {
  const [pages, setPages] = useState(null);
  const [edit, setEdit] = useState(null);
  const toast = useToast();
  const load = () => api("/api/landing").then(setPages);
  useEffect(() => { load(); }, []);
  if (!pages) return <Spinner />;
  const tv = pages.reduce((s, p) => s + p.views, 0), tc = pages.reduce((s, p) => s + p.conversions, 0), tr = pages.reduce((s, p) => s + p.revenue, 0);
  return (
    <div className="fade-in">
      <PageHead title="Landing page" subtitle="Tạo, theo dõi số liệu & xuất bản"
        action={<Button variant="primary" icon="plus" onClick={() => setEdit({})}>Tạo landing page</Button>} />
      <div className="grid grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
        <KpiCard label="Số landing page" value={pages.length} icon="monitor" />
        <KpiCard label="Lượt xem" value={fmtInt(tv)} icon="eye" />
        <KpiCard label="Chuyển đổi" value={fmtInt(tc)} icon="target" />
        <KpiCard label="Doanh thu" value={fmtVnd(tr) + " ₫"} icon="dollar" />
      </div>
      <Card title="Danh sách landing page">
        <DataTable searchKeys={["title", "product_name"]} filters={[{ key: "status", label: "Trạng thái", options: ["draft", "published"] }]}
          columns={[
            { key: "title", label: "Tiêu đề", render: (r) => <div><div className="font-medium text-ink">{r.title}</div><div className="text-xs text-muted">/{r.slug}</div></div> },
            { key: "product_name", label: "Sản phẩm", render: (r) => r.product_name || "—" },
            { key: "status", label: "TT", render: (r) => <StatusBadge s={r.status} /> },
            { key: "views", label: "Xem", align: "right", render: (r) => fmtInt(r.views) },
            { key: "conversions", label: "CĐ", align: "right", render: (r) => fmtInt(r.conversions) },
            { key: "revenue", label: "Doanh thu", align: "right", render: (r) => fmtVnd(r.revenue) + " ₫" },
            { key: "_", label: "", sortable: false, render: (r) => <Button size="sm" variant="ghost" icon="pencil" onClick={() => setEdit(r)}>Mở</Button> },
          ]} rows={pages} pageSize={8} />
      </Card>
      {edit && <LandingModal page={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); toast("Đã lưu"); }} onDeleted={() => { setEdit(null); load(); toast("Đã xoá"); }} />}
    </div>
  );
}
function LandingModal({ page, onClose, onSaved, onDeleted }) {
  const isNew = !page.id;
  const [f, setF] = useState({ title: "", headline: "", subheadline: "", body: "", cta_text: "Mua ngay", theme: "light", ...page });
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const save = async () => {
    const body = { title: f.title, headline: f.headline, subheadline: f.subheadline, body: f.body, cta_text: f.cta_text, theme: f.theme };
    if (isNew) await api("/api/landing", { method: "POST", body });
    else await api(`/api/landing/${page.id}`, { method: "PUT", body });
    onSaved();
  };
  return (
    <Modal title={isNew ? "Tạo landing page" : "Chỉnh sửa landing page"} onClose={onClose} wide>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="space-y-4">
          <Field label="Tiêu đề"><Input value={f.title} onChange={set("title")} /></Field>
          <Field label="Headline"><Input value={f.headline || ""} onChange={set("headline")} /></Field>
          <Field label="Sub-headline"><Input value={f.subheadline || ""} onChange={set("subheadline")} /></Field>
          <Field label="Nội dung"><Textarea value={f.body || ""} onChange={set("body")} /></Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Nút CTA"><Input value={f.cta_text || ""} onChange={set("cta_text")} /></Field>
            <Field label="Giao diện"><Select value={f.theme} onChange={set("theme")}><option value="light">light</option><option value="dark">dark</option></Select></Field>
          </div>
        </div>
        <div>
          <span className="block text-[13px] font-medium text-gray-700 mb-1.5">Xem trước</span>
          <div className="border border-line rounded-xl overflow-hidden h-[420px] bg-hover">
            {!isNew ? <iframe title="preview" src={`/api/landing/${page.id}/preview`} className="w-full h-full" /> : <div className="flex items-center justify-center h-full text-muted text-sm">Lưu để xem trước</div>}
          </div>
        </div>
      </div>
      <div className="flex justify-between gap-2 pt-6 mt-4 border-t border-line">
        {!isNew ? <Button variant="danger" icon="trash" onClick={async () => { if (confirm("Xoá landing page?")) { await api(`/api/landing/${page.id}`, { method: "DELETE" }); onDeleted(); } }}>Xoá</Button> : <span />}
        <div className="flex gap-2">
          <Button onClick={onClose}>Đóng</Button>
          {!isNew && <Button icon="rocket" onClick={async () => { await save(); await api(`/api/landing/${page.id}/publish`, { method: "POST" }); onSaved(); }}>Xuất bản</Button>}
          <Button variant="primary" onClick={save}>Lưu</Button>
        </div>
      </div>
    </Modal>
  );
}

/* ---------------- Ads ---------------- */
function Ads() {
  const [camps, setCamps] = useState(null);
  const [sel, setSel] = useState(null);
  const [creating, setCreating] = useState(false);
  const toast = useToast();
  const load = () => api("/api/ads/campaigns").then(setCamps);
  useEffect(() => { load(); }, []);
  if (!camps) return <Spinner />;
  const spend = camps.reduce((s, c) => s + c.kpi.spend, 0), rev = camps.reduce((s, c) => s + c.kpi.revenue, 0);
  const roas = spend ? rev / spend : 0, purch = camps.reduce((s, c) => s + c.kpi.purchases, 0);
  const evalAll = async () => { for (const c of camps) await api(`/api/ads/campaigns/${c.id}/evaluate`, { method: "POST", body: { use_ai: false } }); load(); toast("Đã đánh giá tất cả"); };
  return (
    <div className="fade-in">
      <PageHead title="Quảng cáo" subtitle="Campaign Meta · KPI · AI đánh giá Scale/Hold/Optimize/Kill"
        action={<div className="flex gap-2"><Button icon="sparkles" onClick={evalAll}>Đánh giá tất cả</Button><Button variant="primary" icon="plus" onClick={() => setCreating(true)}>Tạo campaign</Button></div>} />
      <div className="grid grid-cols-2 xl:grid-cols-4 gap-4 mb-6">
        <KpiCard label="Tổng chi tiêu" value={fmtVnd(spend) + " ₫"} icon="card" />
        <KpiCard label="Doanh thu QC" value={fmtVnd(rev) + " ₫"} icon="dollar" />
        <KpiCard label="ROAS chung" value={roas.toFixed(2)} icon="trendingUp" delta={roas >= 1.5 ? "tốt" : "cần cải thiện"} deltaGood={roas >= 1.5} />
        <KpiCard label="Chuyển đổi" value={fmtInt(purch)} icon="target" />
      </div>
      <Card title="Chiến dịch">
        <DataTable searchKeys={["name"]} filters={[{ key: "status", label: "Trạng thái", options: ["active", "paused", "ended"] }]}
          columns={[
            { key: "name", label: "Campaign", render: (r) => <span className="font-medium">{r.name}</span> },
            { key: "status", label: "TT", render: (r) => <StatusBadge s={r.status} /> },
            { key: "daily_budget", label: "NS/ngày", align: "right", render: (r) => fmtVnd(r.daily_budget) + " ₫" },
            { key: "spend", label: "Chi tiêu", align: "right", render: (r) => fmtVnd(r.kpi.spend) + " ₫", sortable: false },
            { key: "revenue", label: "Doanh thu", align: "right", render: (r) => fmtVnd(r.kpi.revenue) + " ₫", sortable: false },
            { key: "roas", label: "ROAS", align: "right", render: (r) => <b className={r.kpi.roas >= 1.5 ? "text-success" : "text-error"}>{r.kpi.roas.toFixed(2)}</b>, sortable: false },
            { key: "ai_verdict", label: "AI", render: (r) => r.ai_verdict ? <VerdictBadge v={r.ai_verdict} /> : <span className="text-muted">—</span> },
            { key: "_", label: "", sortable: false, render: (r) => <Button size="sm" variant="ghost" onClick={() => setSel(r.id)}>Chi tiết</Button> },
          ]} rows={camps} pageSize={8} />
      </Card>
      {sel && <CampaignDetail id={sel} onClose={() => setSel(null)} onChanged={load} />}
      {creating && <CampaignModal onClose={() => setCreating(false)} onSaved={() => { setCreating(false); load(); toast("Đã tạo & publish Meta"); }} />}
    </div>
  );
}
function CampaignModal({ onClose, onSaved }) {
  const [f, setF] = useState({ name: "", objective: "Conversions", daily_budget: 300000 });
  return (
    <Modal title="Tạo campaign" onClose={onClose}>
      <div className="space-y-4">
        <Field label="Tên campaign"><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} /></Field>
        <Field label="Mục tiêu"><Select value={f.objective} onChange={(e) => setF({ ...f, objective: e.target.value })}>{["Conversions", "Traffic", "Engagement", "Reach"].map((o) => <option key={o}>{o}</option>)}</Select></Field>
        <Field label="Ngân sách/ngày (₫)"><Input type="number" value={f.daily_budget} onChange={(e) => setF({ ...f, daily_budget: e.target.value })} /></Field>
        <div className="flex justify-end gap-2 pt-2"><Button onClick={onClose}>Huỷ</Button>
          <Button variant="primary" icon="rocket" onClick={async () => { await api("/api/ads/campaigns", { method: "POST", body: { ...f, daily_budget: Number(f.daily_budget) } }); onSaved(); }}>Tạo & publish Meta</Button></div>
      </div>
    </Modal>
  );
}
function CampaignDetail({ id, onClose, onChanged }) {
  const [m, setM] = useState(null);
  const [camp, setCamp] = useState(null);
  const [ev, setEv] = useState(null);
  const [loading, setLoading] = useState(false);
  const toast = useToast();
  const load = () => Promise.all([api(`/api/ads/campaigns/${id}/metrics`), api("/api/ads/campaigns")]).then(([mm, cs]) => { setM(mm); setCamp(cs.find((c) => c.id === id)); });
  useEffect(() => { load(); }, [id]);
  const evaluate = async () => { setLoading(true); const r = await api(`/api/ads/campaigns/${id}/evaluate`, { method: "POST", body: { use_ai: true } }); setEv(r); setLoading(false); load(); onChanged(); };
  const setStatus = async (s) => { await api(`/api/ads/campaigns/${id}`, { method: "PUT", body: { status: s } }); load(); onChanged(); toast("Đã cập nhật"); };
  if (!m || !camp) return <Modal title="Chi tiết campaign" onClose={onClose}><Spinner /></Modal>;
  const k = m.kpi;
  const verdict = ev || (camp.ai_verdict ? { verdict: camp.ai_verdict, reasoning: camp.ai_reasoning } : null);
  const vtone = { scale: "green", hold: "blue", optimize: "amber", kill: "red" };
  return (
    <Modal title={camp.name} onClose={onClose} wide>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-5">
        <KpiCard label="ROAS" value={k.roas.toFixed(2)} deltaGood={k.roas >= 1.5} delta={k.roas >= 1.5 ? "tốt" : "yếu"} />
        <KpiCard label="CTR" value={fmtPct(k.ctr)} />
        <KpiCard label="CPA" value={fmtVnd(k.cpa) + " ₫"} />
        <KpiCard label="Chi tiêu" value={fmtVnd(k.spend) + " ₫"} />
      </div>
      <Card title="Chi tiêu vs Doanh thu" className="mb-4">
        <ChartView type="line" height={220} data={{
          labels: m.daily.map((r) => r.date.slice(5)),
          datasets: [
            { label: "Chi tiêu", data: m.daily.map((r) => r.spend), borderColor: "#F59E0B", tension: .35, pointRadius: 0, borderWidth: 2 },
            { label: "Doanh thu", data: m.daily.map((r) => r.revenue), borderColor: "#2563EB", tension: .35, pointRadius: 0, borderWidth: 2 },
          ],
        }} options={{ scales: { y: { ticks: { callback: (v) => fmtVnd(v) } } } }} />
      </Card>
      <div className="bg-hover rounded-xl p-5">
        <div className="flex items-center justify-between mb-3">
          <span className="font-semibold text-ink flex items-center gap-2"><Icon name="sparkles" className="w-5 h-5 text-primary" />AI đánh giá</span>
          <Button variant="primary" size="sm" icon="sparkles" onClick={evaluate} disabled={loading}>{loading ? "Đang phân tích…" : "Phân tích (Claude)"}</Button>
        </div>
        {verdict ? (
          <div>
            <VerdictBadge v={verdict.verdict} />
            <p className="text-sm text-gray-700 mt-3 whitespace-pre-wrap leading-relaxed">{verdict.reasoning}</p>
          </div>
        ) : <p className="text-sm text-muted">Bấm "Phân tích" để AI đánh giá Scale/Hold/Optimize/Kill.</p>}
      </div>
      <div className="flex justify-end gap-2 pt-5">
        <Button icon="pause" onClick={() => setStatus("paused")}>Tạm dừng</Button>
        <Button icon="play" onClick={() => setStatus("active")}>Kích hoạt</Button>
        <Button variant="danger" onClick={() => setStatus("ended")}>Kết thúc</Button>
      </div>
    </Modal>
  );
}

/* ============ Primitives dùng chung cho các file JSX khác ============
   benchmark.jsx (nạp trước file này) đọc window.UI lúc render. */
window.UI = {
  api, Icon, ICONS, Rating, Button, Card, Badge, KpiCard, Field, Input, Textarea, Select,
  Modal, Drawer, Empty, Spinner, ChartView, DataTable, PageHead, useToast,
  CHART_COLORS, fmtVnd, fmtVndFull, fmtInt, fmtPct,
};

/* ============================ Root ============================ */
function App() {
  const [route, setRoute] = useState("dashboard");
  const [toasts, setToasts] = useState([]);
  const pushToast = useCallback((msg) => {
    const id = Math.random();
    setToasts((t) => [...t, { id, msg }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 2600);
  }, []);
  const titles = { dashboard: "Tổng quan", market: "Nghiên cứu ngách", benchmark: "Benchmark & SP tiềm năng", products: "Sản phẩm", landing: "Landing page", ads: "Quảng cáo" };
  const Pages = { dashboard: Dashboard, market: window.NichePage, benchmark: window.BenchmarkPage, products: Products, landing: Landing, ads: Ads };
  const Current = Pages[route];
  return (
    <ToastCtx.Provider value={pushToast}>
      <div className="flex">
        <Sidebar route={route} setRoute={setRoute} />
        <div className="flex-1 min-w-0">
          <TopBar title={titles[route]} />
          <main className="p-6 max-w-[1400px] mx-auto">
            <Current key={route} />
          </main>
        </div>
      </div>
      <div className="fixed bottom-5 right-5 z-[60] space-y-2">
        {toasts.map((t) => (
          <div key={t.id} className="flex items-center gap-2 bg-ink text-white text-sm px-4 py-2.5 rounded-[10px] shadow-lg fade-in">
            <Icon name="checkCircle" className="w-4 h-4 text-green-400" />{t.msg}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}
ReactDOM.createRoot(document.getElementById("root")).render(<App />);
