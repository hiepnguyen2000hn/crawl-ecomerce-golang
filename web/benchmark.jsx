/* MODULE 1B — Nghiên cứu thị trường & Benchmark sản phẩm (Meta Ads Library).

   Điều hướng:
     view "list"   — danh sách phiên nghiên cứu + nút tạo phiên (drawer bên phải)
     view "detail" — 1 phiên: tab Kho benchmark | tab Chấm điểm sản phẩm

   File này nạp TRƯỚC app.jsx nên chỉ đọc `window.UI` lúc render (app.jsx gán
   window.UI trước khi ReactDOM render). Trang đăng ký qua `window.BenchmarkPage`. */
(function () {
  const { useState, useEffect, useRef, useCallback } = React;

  /* ---------------------------- helpers ---------------------------- */
  const U = () => window.UI;
  const LANG = () => (window.I18N && window.I18N.lang) || "vi";
  const api = (p, o) => window.UI.api(p, o);
  const nf = (v) => Number(v || 0).toLocaleString("vi-VN");
  const money = (v, cur) => {
    if (v == null || v === "") return "—";
    const sym = { EUR: "€", GBP: "£", USD: "$", PLN: "zł", RON: "lei", HUF: "Ft",
      CZK: "Kč", AED: "AED", SAR: "SAR", KWD: "KWD", VND: "₫" }[cur] || (cur || "");
    if (cur === "VND" || cur === "HUF") return nf(Math.round(v)) + " " + sym;
    return sym + Number(v).toFixed(2);
  };
  const dt = (iso) => {
    if (!iso) return "chưa cập nhật";
    const d = new Date(iso);
    if (isNaN(d)) return "chưa cập nhật";
    const p = (n) => String(n).padStart(2, "0");
    return `${p(d.getDate())}/${p(d.getMonth() + 1)}/${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`;
  };
  /** Cờ quốc gia từ mã ISO-2 (regional indicator symbols) — "NL" -> 🇳🇱 */
  const flagOf = (code) => {
    const c = (code || "").trim().toUpperCase();
    if (!/^[A-Z]{2}$/.test(c)) return "🌐";
    return String.fromCodePoint(...[...c].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65));
  };
  function Flag({ code, className = "text-base leading-none" }) {
    return <span className={className} title={code} role="img" aria-label={code}>{flagOf(code)}</span>;
  }

  const BM_ICONS = {
    globe: '<circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    arrowLeft: '<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>',
    refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
    fileText: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>',
    keyboard: '<rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.01"/><path d="M10 8h.01"/><path d="M14 8h.01"/><path d="M18 8h.01"/><path d="M8 12h.01"/><path d="M12 12h.01"/><path d="M16 12h.01"/><path d="M7 16h10"/>',
  };
  function BIcon({ name, className = "w-5 h-5" }) {
    const html = BM_ICONS[name] || (window.UI.ICONS || {})[name] || "";
    return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
      dangerouslySetInnerHTML={{ __html: html }} />;
  }

  const KIND_LABEL = { seed: "Gốc", synonym: "Đồng nghĩa", feature: "Tính năng", cta: "Chốt đơn" };
  const VERDICT_TONE = { scale: "green", potential: "blue", consider: "amber", risky: "red" };
  const STEPS = [
    { key: "fetch", label: "Fetching Ads Library", until: 60 },
    { key: "filter", label: "AI Noise Filtering", until: 90 },
    { key: "store", label: "Storing Benchmark", until: 100 },
  ];

  function Thumb({ src, alt, className = "w-full h-44", rounded = "rounded-t-xl" }) {
    const [bad, setBad] = useState(false);
    useEffect(() => setBad(false), [src]);
    if (!src || bad) {
      return (
        <div className={`${className} ${rounded} bg-hover flex items-center justify-center text-disabled`}>
          <BIcon name="image" className="w-7 h-7" />
        </div>
      );
    }
    return <img src={src} alt={alt || ""} onError={() => setBad(true)}
      className={`${className} ${rounded} object-cover bg-hover`} />;
  }

  /* --------------------- Tiến trình cào theo step --------------------- */
  function StepProgress({ session }) {
    const { Badge } = U();
    const p = session.progress || 0;
    const err = session.status === "error";
    const state = (i) => {
      if (err) return p >= (STEPS[i - 1] ? STEPS[i - 1].until : 0) ? "error" : "todo";
      if (session.status === "done" && p >= 100) return "done";
      if (p >= STEPS[i].until) return "done";
      if (i === 0 || p >= STEPS[i - 1].until) return "active";
      return "todo";
    };
    return (
      <div className="p-4 rounded-xl border border-line bg-bg">
        <div className="flex items-center justify-between mb-3">
          <span className="text-sm font-medium text-ink">{session.stage || "Chưa chạy"}</span>
          <span className="text-[13px] text-muted tabular-nums">{p}%</span>
        </div>
        <div className="h-2 rounded-full bg-hover overflow-hidden mb-4">
          <div className={`h-full rounded-full transition-all duration-500 ${err ? "bg-error" : "bg-primary"}`}
            style={{ width: `${p}%` }} />
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {STEPS.map((s, i) => {
            const st = state(i);
            return (
              <div key={s.key} className={`flex items-center gap-2.5 px-3 py-2.5 rounded-[10px] border ${
                st === "done" ? "border-green-200 bg-green-50"
                  : st === "active" ? "border-primary bg-softblue"
                    : st === "error" ? "border-red-200 bg-red-50" : "border-line bg-surface"}`}>
                <span className={`w-6 h-6 shrink-0 rounded-full flex items-center justify-center text-xs font-semibold ${
                  st === "done" ? "bg-success text-white"
                    : st === "active" ? "bg-primary text-white"
                      : st === "error" ? "bg-error text-white" : "bg-hover text-muted"}`}>
                  {st === "done" ? "✓" : st === "error" ? "!" : i + 1}
                </span>
                <div className="min-w-0">
                  <div className="text-[13px] font-medium text-ink truncate">{s.label}</div>
                  <div className="text-[11px] text-muted">
                    {st === "done" ? "Xong" : st === "active" ? "Đang chạy…" : st === "error" ? "Lỗi" : "Chờ"}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
        {session.message && <p className="text-[13px] text-error mt-3">{session.message}</p>}
      </div>
    );
  }

  /* ===================================================================
     MÀN HÌNH A — Danh sách phiên nghiên cứu
     =================================================================== */
  function SessionList({ meta, sessions, onOpen, onCreate, onDeleted }) {
    const { Card, Button, Badge, Empty } = U();
    const del = async (e, s) => {
      e.stopPropagation();
      if (!confirm(`Xoá phiên “${s.category} — ${s.country_name}” cùng toàn bộ dữ liệu đã cào?`)) return;
      await api(`/api/benchmark/sessions/${s.id}`, { method: "DELETE" });
      onDeleted();
    };
    return (
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2 text-[13px]">
          <Badge tone={meta.ads_source === "meta_api" ? "green" : "amber"}>
            Nguồn ads: {meta.ads_source === "meta_api" ? "Meta Ads Library API" : "Demo (chưa có META_ACCESS_TOKEN)"}
          </Badge>
          <Badge tone={meta.ai_enabled ? "green" : "amber"}>
            AI: {meta.ai_enabled ? "Claude (vision + text)" : "Rule engine nội bộ"}
          </Badge>
          <Badge tone="slate">Ngưỡng lọc nhiễu: relevance ≥ {meta.relevance_threshold}</Badge>
        </div>

        <Card title={`Phiên nghiên cứu (${sessions.length})`}
          action={<Button variant="primary" icon="plus" onClick={onCreate}>Tạo phiên nghiên cứu</Button>}>
          {!sessions.length ? (
            <Empty icon="search" title="Chưa có phiên nghiên cứu nào"
              hint="Tạo phiên đầu tiên: chọn thị trường, ngành hàng và từ khoá gốc."
              action={<Button variant="primary" icon="plus" onClick={onCreate}>Tạo phiên nghiên cứu</Button>} />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              {sessions.map((s) => (
                <div key={s.id} onClick={() => onOpen(s.id)}
                  className="bm-session-card p-4 rounded-xl border border-line hover:border-primary hover:shadow-softmd
                             transition-all cursor-pointer bg-surface group">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="font-semibold text-ink truncate">{s.category}</div>
                      <div className="text-[13px] text-muted mt-0.5 flex items-center gap-1.5">
                        <Flag code={s.country_code} />
                        {s.country_name} ({s.country_code}) · {s.currency}
                      </div>
                    </div>
                    <Badge tone={s.status === "done" ? "green" : s.status === "running" ? "blue"
                      : s.status === "error" ? "red" : "slate"}>
                      {s.status === "running" ? "đang cào" : s.status}
                    </Badge>
                  </div>

                  <div className="grid grid-cols-3 gap-2 mt-3 text-center">
                    {[["Benchmark", s.kept_count, "text-success"], ["Nhiễu", s.noise_count, "text-warning"],
                      ["Ads thô", s.raw_count, "text-ink"]].map(([label, v, cls]) => (
                      <div key={label} className="py-1.5 rounded-lg bg-bg">
                        <div className={`text-lg font-bold tabular-nums ${cls}`}>{nf(v)}</div>
                        <div className="text-[11px] text-muted">{label}</div>
                      </div>
                    ))}
                  </div>

                  <div className="flex items-center justify-between mt-3 text-xs text-muted">
                    <span>Cập nhật: {dt(s.last_scraped_at)}</span>
                    <button onClick={(e) => del(e, s)}
                      className="opacity-0 group-hover:opacity-100 transition-opacity text-muted hover:text-error">
                      <BIcon name="trash" className="w-4 h-4" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
    );
  }

  /* ===================================================================
     DRAWER — Tạo phiên nghiên cứu mới
     =================================================================== */
  function NewSessionDrawer({ meta, onClose, onDone }) {
    const { Drawer, Button, Badge, Field, Input, Select, Spinner } = U();
    const toast = U().useToast();
    const [form, setForm] = useState({ country_code: "NL", category: "", seeds: "" });
    const [phase, setPhase] = useState("form");   // form | scraping
    const [busy, setBusy] = useState("");
    const [session, setSession] = useState(null);
    const [keywords, setKeywords] = useState([]);

    // Poll tiến trình cào
    useEffect(() => {
      if (phase !== "scraping" || !session) return;
      const t = setInterval(async () => {
        const s = await api(`/api/benchmark/sessions/${session.id}`);
        setSession(s);
        if (s.status !== "running") {
          clearInterval(t);
          if (s.status === "done") setTimeout(() => onDone(s.id), 900);
        }
      }, 1200);
      return () => clearInterval(t);
    }, [phase, session && session.id]);

    const ensureSession = async () => {
      if (session) return session;
      if (!form.category.trim()) { toast("Nhập ngành hàng trước đã"); return null; }
      const s = await api("/api/benchmark/sessions", {
        method: "POST",
        body: {
          country_code: form.country_code, category: form.category,
          seed_keywords: form.seeds.split(/[,\n]/).map((x) => x.trim()).filter(Boolean),
        },
      });
      setSession(s);
      setKeywords(await api(`/api/benchmark/sessions/${s.id}/keywords`));
      return s;
    };

    const expand = async () => {
      setBusy("expand");
      try {
        const s = await ensureSession();
        if (!s) return;
        const r = await api(`/api/benchmark/sessions/${s.id}/keywords/expand?lang=${LANG()}`, { method: "POST" });
        setKeywords(r.keywords);
        toast(r.source === "claude" ? "Claude đã mở rộng bộ keyword" : "Đã mở rộng keyword (rule engine)");
      } finally { setBusy(""); }
    };

    const toggle = async (k) => {
      setKeywords((ks) => ks.map((x) => (x.id === k.id ? { ...x, enabled: x.enabled ? 0 : 1 } : x)));
      await api(`/api/benchmark/keywords/${k.id}`, { method: "PUT", body: { enabled: !k.enabled } });
    };

    const start = async () => {
      setBusy("start");
      try {
        const s = await ensureSession();
        if (!s) return;
        const on = keywords.filter((k) => k.enabled);
        if (keywords.length && !on.length) { toast("Bật ít nhất 1 keyword"); return; }
        await api(`/api/benchmark/sessions/${s.id}/scrape`, { method: "POST", body: { limit_per_keyword: 12, lang: LANG() } });
        setSession(await api(`/api/benchmark/sessions/${s.id}`));
        setPhase("scraping");
      } finally { setBusy(""); }
    };

    const enabledCount = keywords.filter((k) => k.enabled).length;

    return (
      <Drawer title={phase === "form" ? "Tạo phiên nghiên cứu mới" : "Đang cào dữ liệu Meta Ads Library"}
        subtitle={phase === "form" ? "Chọn thị trường, ngành hàng và từ khoá gốc"
          : `${session.category} — ${session.country_name}`}
        onClose={onClose} width="max-w-xl">
        {phase === "scraping" ? (
          <div className="space-y-4">
            <StepProgress session={session} />
            {session.status === "done" && (
              <div className="p-4 rounded-xl bg-green-50 border border-green-100 text-sm text-ink">
                Xong! {session.kept_count} sản phẩm benchmark, loại {session.noise_count} nhiễu.
                Đang chuyển sang kho benchmark…
              </div>
            )}
            {session.status === "error" && (
              <Button variant="primary" className="w-full" onClick={() => setPhase("form")}>Quay lại chỉnh keyword</Button>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            <Field label="Quốc gia / Thị trường target">
              <Select value={form.country_code} disabled={!!session}
                onChange={(e) => setForm({ ...form, country_code: e.target.value })}>
                {meta.countries.map((c) => (
                  <option key={c.code} value={c.code}>{flagOf(c.code)} {c.name} ({c.code}) · {c.currency}</option>
                ))}
              </Select>
            </Field>

            <Field label="Ngành hàng">
              <Input value={form.category} disabled={!!session} placeholder="Thời trang nữ, Gia dụng thông minh…"
                onChange={(e) => setForm({ ...form, category: e.target.value })} />
            </Field>

            <Field label="Từ khoá gốc (ngôn ngữ bất kỳ)"
              hint="Cách nhau bằng dấu phẩy. Bấm “AI mở rộng” để dịch sang ngôn ngữ thị trường và sinh thêm biến thể.">
              <div className="flex gap-2">
                <Input value={form.seeds} disabled={!!session} placeholder="váy đầm, đầm nữ, women dress"
                  onChange={(e) => setForm({ ...form, seeds: e.target.value })} />
                <Button variant="primary" icon="sparkles" className="shrink-0" disabled={busy === "expand"}
                  onClick={expand}>
                  {busy === "expand" ? "Đang mở rộng…" : "AI mở rộng"}
                </Button>
              </div>
            </Field>

            {!!keywords.length && (
              <div>
                <div className="text-[13px] font-medium text-gray-700 mb-2">
                  Chọn keyword sẽ dùng để cào ({enabledCount}/{keywords.length})
                </div>
                <div className="flex flex-wrap gap-2 max-h-64 overflow-y-auto">
                  {keywords.map((k) => (
                    <button key={k.id} onClick={() => toggle(k)} title={k.note || ""}
                      className={`px-3 py-1.5 rounded-full text-[13px] border transition-colors ${
                        k.enabled ? "border-primary bg-softblue text-primary" : "border-gray-300 text-muted hover:bg-hover"}`}>
                      {k.keyword}
                      <span className="ml-1.5 text-[11px] opacity-70">{KIND_LABEL[k.kind] || k.kind}</span>
                    </button>
                  ))}
                </div>
              </div>
            )}

            <div className="flex gap-2 pt-2">
              <Button variant="primary" icon="play" className="flex-1" disabled={busy === "start"} onClick={start}>
                {busy === "start" ? "Đang khởi chạy…" : "Tạo phiên & bắt đầu cào"}
              </Button>
              <Button onClick={onClose}>Huỷ</Button>
            </div>
          </div>
        )}
      </Drawer>
    );
  }

  /* ===================================================================
     MÀN HÌNH B — Chi tiết 1 phiên (2 tab)
     =================================================================== */
  function SessionDetail({ session, onBack, onRefreshSession }) {
    const { Button, Badge } = U();
    const [tab, setTab] = useState("store");
    const TABS = [["store", "Sản phẩm benchmark"], ["scores", "Chấm điểm sản phẩm"]];
    return (
      <div>
        <button onClick={onBack} className="inline-flex items-center gap-1.5 text-[13px] text-muted hover:text-ink mb-3">
          <BIcon name="arrowLeft" className="w-4 h-4" /> Tất cả phiên nghiên cứu
        </button>
        <div className="flex items-start justify-between gap-4 mb-5">
          <div>
            <h1 className="text-[28px] font-bold text-ink tracking-tight leading-tight">{session.category}</h1>
            <div className="flex flex-wrap items-center gap-2 mt-1.5">
              <Badge tone="blue"><Flag code={session.country_code} className="mr-1" />{session.country_name} ({session.country_code})</Badge>
              <Badge tone="slate">{session.currency}</Badge>
              <span className="text-[13px] text-muted">
                Từ khoá gốc: {(session.seed_keywords || []).join(", ") || "—"}
              </span>
            </div>
          </div>
        </div>

        <div className="flex gap-1 border-b border-line mb-5">
          {TABS.map(([k, label]) => (
            <button key={k} onClick={() => setTab(k)}
              className={`px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors ${
                tab === k ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink"}`}>
              {label}
            </button>
          ))}
        </div>

        {tab === "store" ? <BenchmarkStore session={session} onRefreshSession={onRefreshSession} />
          : <ScoreTab session={session} />}
      </div>
    );
  }

  /* ---------------------- Tab 1: kho benchmark ---------------------- */
  function BenchmarkStore({ session, onRefreshSession }) {
    const { Card, Button, Badge, Select, Empty, Spinner, KpiCard } = U();
    const toast = U().useToast();
    const [data, setData] = useState(null);
    const [filters, setFilters] = useState({ min_days_active: 0, sort: "reach", include_noise: false });
    const [detailId, setDetailId] = useState(null);
    const running = session.status === "running";

    const load = useCallback(() => {
      const q = new URLSearchParams({
        min_days_active: filters.min_days_active, sort: filters.sort, include_noise: filters.include_noise,
      });
      api(`/api/benchmark/sessions/${session.id}/products?${q}`).then(setData);
    }, [session.id, filters]);
    useEffect(load, [load]);
    useEffect(() => { if (!running && data) load(); }, [running]);

    // sau khi user đánh dấu nhiễu: nạp lại lưới (item có thể rời khỏi bộ lọc) + số liệu phiên
    const patchProduct = useCallback(() => {
      load();
      onRefreshSession && onRefreshSession();
    }, [load, onRefreshSession]);

    const rescrape = async () => {
      await api(`/api/benchmark/sessions/${session.id}/scrape`, { method: "POST", body: { limit_per_keyword: 12, lang: LANG() } });
      onRefreshSession();
      toast("Đang cập nhật dữ liệu từ Meta Ads Library");
    };

    const cur = session.currency;
    const st = (data && data.stats) || {};

    return (
      <div className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted">
            Lần cập nhật cuối: <span className="text-ink font-medium">{dt(session.last_scraped_at)}</span>
            {session.source && <span> · nguồn {session.source === "meta_api" ? "Meta Ads Library API" : "demo"}</span>}
          </div>
          <Button variant="primary" icon="refresh" disabled={running} onClick={rescrape}>
            {running ? "Đang cập nhật…" : "Cập nhật dữ liệu từ Meta"}
          </Button>
        </div>

        {running && <StepProgress session={session} />}

        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <KpiCard label="Sản phẩm benchmark" value={nf(st.kept || 0)} icon="package" />
          <KpiCard label="Ads sống > 14 ngày" value={nf(st.long_running || 0)} icon="flame" />
          <KpiCard label="Giá bán trung bình" value={money(st.avg_price, cur)} icon="dollar" />
          <KpiCard label="Reach cao nhất" value={nf(st.top_reach || 0)} icon="trendingUp" />
        </div>

        <Card title={`Kho sản phẩm Benchmark${data ? ` — ${data.products.length} kết quả` : ""}`} action={
          <div className="flex flex-wrap gap-2 items-center">
            {[[0, "Tất cả"], [14, "> 14 ngày"], [30, "> 30 ngày"]].map(([v, label]) => (
              <button key={v} onClick={() => setFilters({ ...filters, min_days_active: v })}
                className={`px-3 h-9 rounded-[10px] text-[13px] border ${
                  filters.min_days_active === v ? "border-primary bg-softblue text-primary"
                    : "border-gray-300 text-muted hover:bg-hover"}`}>{label}</button>
            ))}
            <div className="w-44">
              <Select value={filters.sort} onChange={(e) => setFilters({ ...filters, sort: e.target.value })}>
                <option value="reach">Reach cao nhất</option>
                <option value="days">Chạy lâu nhất</option>
                <option value="price_asc">Giá tăng dần</option>
                <option value="price_desc">Giá giảm dần</option>
                <option value="relevance">Độ liên quan</option>
              </Select>
            </div>
            <button onClick={() => setFilters({ ...filters, include_noise: !filters.include_noise })}
              className={`px-3 h-9 rounded-[10px] text-[13px] border ${
                filters.include_noise ? "border-warning bg-amber-50 text-amber-700"
                  : "border-gray-300 text-muted hover:bg-hover"}`}>
              {filters.include_noise ? "Đang hiện cả nhiễu" : "Hiện cả ads nhiễu"}
            </button>
          </div>
        }>
          {!data ? <Spinner /> : !data.products.length ? (
            <Empty icon="package" title="Chưa có dữ liệu benchmark"
              hint="Bấm “Cập nhật dữ liệu từ Meta” để cào, hoặc nới bộ lọc." />
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
              {data.products.map((p) => (
                <div key={p.id} onClick={() => setDetailId(p.id)}
                  className={`bm-card border rounded-xl overflow-hidden hover:shadow-softmd transition-shadow cursor-pointer bg-surface flex flex-col ${
                    p.is_noise ? "border-amber-300" : "border-line"}`}>
                  <Thumb src={p.image_url} alt={p.product_name} />
                  <div className="p-3 space-y-1.5 flex-1 flex flex-col">
                    <div className="font-medium text-ink text-sm line-clamp-2 min-h-[2.5rem]">{p.product_name}</div>
                    <div className="text-xs text-muted truncate">{p.page_name}</div>
                    <div className="flex items-center justify-between">
                      <span className="font-semibold text-ink">{money(p.price, p.currency || cur)}</span>
                      <Badge tone={p.days_active > 14 ? "green" : "slate"}>{p.days_active} ngày</Badge>
                    </div>
                    <div className="flex items-center justify-between text-xs text-muted">
                      <span>Reach ~{nf(p.reach)}</span>
                      <span>rel {Number(p.relevance_score).toFixed(2)}</span>
                    </div>
                    <div className="flex items-center gap-1.5 flex-wrap">
                      {!!p.is_noise && <Badge tone="amber">Nhiễu: {p.brand_type}</Badge>}
                      {!!p.manual_override && <Badge tone="slate">Thủ công</Badge>}
                    </div>
                    <div className="pt-1 mt-auto">
                      <NoiseButton product={p} onChanged={patchProduct} />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>

        {detailId && <ProductDrawer id={detailId} currency={cur} onChanged={patchProduct}
          onClose={() => setDetailId(null)} />}
      </div>
    );
  }

  /* Nút đánh dấu nhiễu / đưa lại vào benchmark — dùng ở cả card và drawer */
  function NoiseButton({ product, onChanged, full = false }) {
    const toast = U().useToast();
    const [busy, setBusy] = useState(false);
    const noise = !!product.is_noise;
    const click = async (e) => {
      e.stopPropagation();
      setBusy(true);
      try {
        const updated = await api(`/api/benchmark/products/${product.id}/noise`, {
          method: "PUT", body: { is_noise: !noise },
        });
        toast(noise ? "Đã đưa lại vào bộ benchmark" : "Đã đánh dấu là nhiễu");
        onChanged && onChanged(updated);
      } finally { setBusy(false); }
    };
    return (
      <button onClick={click} disabled={busy}
        className={`${full ? "px-3 h-9" : "w-full px-2 h-8"} rounded-[10px] border text-[12px] font-medium
                    inline-flex items-center justify-center gap-1.5 transition-colors disabled:opacity-50 ${
          noise ? "border-green-300 text-success hover:bg-green-50"
            : "border-gray-300 text-muted hover:bg-hover hover:text-amber-700"}`}>
        <BIcon name={noise ? "checkCircle" : "filter"} className="w-3.5 h-3.5" />
        {noise ? "Đưa vào benchmark" : "Đánh dấu nhiễu"}
      </button>
    );
  }

  function ProductDrawer({ id, currency, onClose, onChanged }) {
    const { Drawer, Badge, Spinner } = U();
    const [p, setP] = useState(null);
    const [tab, setTab] = useState("media");
    useEffect(() => { api(`/api/benchmark/products/${id}`).then(setP); }, [id]);
    if (!p) return <Drawer title="Đang tải…" onClose={onClose} width="max-w-3xl"><Spinner /></Drawer>;

    const TABS = [["media", "Media Clips"], ["copies", "Ad Copies"], ["page", "Fanpage"], ["lp", "Landing Page"]];
    return (
      <Drawer title={p.product_name} subtitle={`${p.page_name} · ${p.days_active} ngày active · reach ~${nf(p.reach)}`}
        onClose={onClose} width="max-w-3xl">
        <div className="flex flex-wrap items-center gap-2 mb-4">
          <Badge tone="blue">{money(p.price, p.currency || currency)}</Badge>
          <Badge tone={p.is_noise ? "amber" : "green"}>
            {p.is_noise ? "Bị loại khỏi benchmark" : "Đạt chuẩn benchmark"} · rel {Number(p.relevance_score).toFixed(2)}
          </Badge>
          <Badge tone="slate">Keyword: {p.matched_keyword}</Badge>
          {!!p.manual_override && <Badge tone="slate">Đánh dấu thủ công</Badge>}
          <NoiseButton product={p} full onChanged={(u) => { setP({ ...p, ...u }); onChanged && onChanged(u); }} />
          {p.noise_reason && <span className="text-[13px] text-muted self-center">{p.noise_reason}</span>}
        </div>

        <div className="flex gap-1 border-b border-line mb-4">
          {TABS.map(([k, label]) => (
            <button key={k} onClick={() => setTab(k)}
              className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                tab === k ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink"}`}>
              {label}
            </button>
          ))}
        </div>

        {tab === "media" && (
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            {(p.media_urls || []).length ? p.media_urls.map((m, i) => (
              <div key={i} className="border border-line rounded-xl overflow-hidden">
                <Thumb src={m.thumbnail || m.url} className="w-full h-36" rounded="" />
                <div className="px-2 py-1.5 text-xs text-muted flex items-center justify-between">
                  <span>{m.type === "video" ? "Video" : "Ảnh"}</span>
                  <a className="text-primary hover:underline" href={m.url} target="_blank" rel="noreferrer">Mở</a>
                </div>
              </div>
            )) : <p className="text-sm text-muted col-span-full">
              Chưa có media. Ad Library API public không trả CDN ảnh/video — cần enrichment ở Phase 2.
            </p>}
          </div>
        )}

        {tab === "copies" && (
          <div className="space-y-3">
            {(p.ad_copies || []).length ? p.ad_copies.map((c, i) => (
              <div key={i} className="p-3 rounded-[10px] bg-bg border border-line text-sm text-ink whitespace-pre-wrap">{c}</div>
            )) : <p className="text-sm text-muted">Không có nội dung quảng cáo.</p>}
          </div>
        )}

        {tab === "page" && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              {[["Tên page", p.page_name], ["Page ID", p.page_id],
                ["Thời gian lập", p.page_created || "—"], ["Ads đang active", p.page_ads_active ?? "—"]].map(([k, v]) => (
                <div key={k} className="p-3 rounded-[10px] bg-bg border border-line">
                  <div className="text-xs text-muted">{k}</div>
                  <div className="text-sm font-medium text-ink truncate">{v}</div>
                </div>
              ))}
            </div>
            <div>
              <div className="text-[13px] font-medium text-gray-700 mb-2">Sản phẩm khác cùng fanpage</div>
              {(p.page_other_ads || []).length ? (
                <div className="space-y-2">
                  {p.page_other_ads.map((o, i) => (
                    <div key={i} className="flex items-center gap-3 p-2 rounded-[10px] border border-line">
                      <Thumb src={o.image_url} className="w-12 h-12 shrink-0" rounded="rounded-lg" />
                      <div className="min-w-0 flex-1">
                        <div className="text-sm text-ink truncate">{o.product_name}</div>
                        <div className="text-xs text-muted">{o.days_active} ngày · reach ~{nf(o.reach)}</div>
                      </div>
                      <span className="text-sm font-medium">{money(o.price, currency)}</span>
                    </div>
                  ))}
                </div>
              ) : <p className="text-sm text-muted">Chưa cào được ads khác của page này.</p>}
            </div>
          </div>
        )}

        {tab === "lp" && (
          <div className="space-y-3">
            {p.landing_page_url && (
              <a href={p.landing_page_url} target="_blank" rel="noreferrer"
                className="inline-flex items-center gap-2 text-primary hover:underline text-sm break-all">
                <BIcon name="externalLink" className="w-4 h-4 shrink-0" />{p.landing_page_url}
              </a>
            )}
            <div className="space-y-2">
              {(p.landing_structure || []).map((b, i) => (
                <div key={i} className="p-3 rounded-[10px] bg-bg border border-line">
                  <div className="text-[13px] font-semibold text-ink">{b.block}</div>
                  <div className="text-sm text-muted">{b.detail}</div>
                </div>
              ))}
            </div>
          </div>
        )}
      </Drawer>
    );
  }

  /* ------------------- Tab 2: sản phẩm đã chấm điểm ------------------- */
  function ScoreTab({ session }) {
    const { Card, Button, Badge, Empty, Spinner } = U();
    const [items, setItems] = useState(null);
    const [openId, setOpenId] = useState(null);
    const [adding, setAdding] = useState(false);

    const load = useCallback(() => {
      api(`/api/benchmark/sessions/${session.id}/evaluations`).then(setItems);
    }, [session.id]);
    useEffect(load, [load]);

    return (
      <div className="space-y-5">
        <Card title={`Sản phẩm đã chấm điểm${items ? ` (${items.length})` : ""}`}
          action={<Button variant="primary" icon="plus" onClick={() => setAdding(true)}>Nhập sản phẩm mới</Button>}>
          {!items ? <Spinner /> : !items.length ? (
            <Empty icon="target" title="Chưa chấm điểm sản phẩm nào"
              hint={`Nhập sản phẩm mới để AI so với ${session.kept_count} sản phẩm benchmark tại ${session.country_name}.`}
              action={<Button variant="primary" icon="plus" onClick={() => setAdding(true)}>Nhập sản phẩm mới</Button>} />
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
              {items.map((e) => (
                <div key={e.id} onClick={() => setOpenId(e.id)}
                  className="bm-eval-card rounded-xl border border-line hover:shadow-softmd cursor-pointer bg-surface overflow-hidden flex">
                  <Thumb src={e.image_url} className="w-24 shrink-0 self-stretch" rounded="" />
                  <div className="p-3 min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <div className="font-medium text-ink text-sm line-clamp-2">{e.product_name}</div>
                      <div className={`text-xl font-bold tabular-nums shrink-0 ${
                        e.win_score >= 75 ? "text-success" : e.win_score >= 60 ? "text-primary"
                          : e.win_score >= 45 ? "text-warning" : "text-error"}`}>
                        {e.win_score != null ? `${e.win_score}%` : "—"}
                      </div>
                    </div>
                    <div className="text-xs text-muted mt-1">
                      {money(e.price_expected, e.currency)} · vốn {money(e.cogs, e.currency)}
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      <Badge tone={VERDICT_TONE[e.verdict] || "slate"}>{e.verdict_label}</Badge>
                      <Badge tone="slate">{e.ai_source === "claude" ? "Claude" : "Rule"}</Badge>
                    </div>
                    <div className="text-[11px] text-muted mt-1.5">{dt(e.created_at)}</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>

        {adding && <AddProductDrawer session={session} onClose={() => setAdding(false)}
          onDone={(ids) => { setAdding(false); load(); if (ids && ids.length) setOpenId(ids[0]); }} />}
        {openId && <ScorecardDrawer id={openId} onClose={() => setOpenId(null)}
          onDeleted={() => { setOpenId(null); load(); }} />}
      </div>
    );
  }

  /* ============ DRAWER — Nhập sản phẩm mới (3 cách nhập) ============ */
  const EMPTY_ROW = { name: "", source_url: "", cogs: "", price_expected: "", variants: "",
    material: "", note: "", image_url: "", media_count: 0 };

  function AddProductDrawer({ session, onClose, onDone }) {
    const { Drawer, Button, Badge, Field, Input, Spinner } = U();
    const toast = U().useToast();
    const [mode, setMode] = useState("manual");     // manual | file | url
    const [rows, setRows] = useState([{ ...EMPTY_ROW }]);
    const [urls, setUrls] = useState("");
    const [busy, setBusy] = useState("");
    const [notes, setNotes] = useState([]);         // cảnh báo khi bóc tách URL
    const imgRef = useRef(null);
    const fileRef = useRef(null);

    const setRow = (i, patch) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));
    const addRow = () => setRows((rs) => [...rs, { ...EMPTY_ROW }]);
    const delRow = (i) => setRows((rs) => (rs.length === 1 ? [{ ...EMPTY_ROW }] : rs.filter((_, j) => j !== i)));

    const readImage = (files, i) => {
      const f = Array.from(files).find((x) => x.type.startsWith("image/"));
      if (!f) return;
      const reader = new FileReader();
      reader.onload = () => setRow(i, {
        image_url: reader.result, media_count: files.length,
        name: rows[i].name || f.name.replace(/\.[^.]+$/, ""),
      });
      reader.readAsDataURL(f);
    };

    const importFile = (file) => {
      const reader = new FileReader();
      reader.onload = async () => {
        setBusy("file");
        try {
          const r = await api("/api/benchmark/parse-upload", {
            method: "POST", body: { filename: file.name, content_b64: String(reader.result).split(",").pop() },
          });
          if (!r.products.length) return toast("Không đọc được dòng nào — kiểm tra tiêu đề cột");
          setRows(r.products.map((p) => ({ ...EMPTY_ROW, ...p, cogs: p.cogs ?? "", price_expected: p.price_expected ?? "" })));
          setMode("manual");
          toast(`Đã nạp ${r.products.length} sản phẩm từ ${file.name}`);
        } catch (e) { toast("Lỗi đọc file: " + e.message); } finally { setBusy(""); }
      };
      reader.readAsDataURL(file);
    };

    const fetchUrls = async () => {
      const list = urls.split(/[\n,]/).map((x) => x.trim()).filter(Boolean);
      if (!list.length) return toast("Dán ít nhất 1 URL sản phẩm");
      setBusy("url"); setNotes([]);
      const out = [], warns = [];
      for (const url of list) {
        try {
          const r = await api("/api/benchmark/enrich-url", { method: "POST", body: { url, session_id: session.id, lang: LANG() } });
          if (r.ok) {
            out.push({ ...EMPTY_ROW, name: r.name || "", source_url: r.source_url, image_url: r.image_url || "",
              media_count: r.media_count || 0, variants: r.variants || "", material: r.material || "",
              note: r.note || "", price_expected: r.price ?? "" });
            warns.push({ url, ok: true, text: `Đã lấy: ${r.name}` + (r.scrape_source === "playwright" ? " (qua trình duyệt headless)" : "") });
          } else {
            out.push({ ...EMPTY_ROW, source_url: url });
            warns.push({ url, ok: false, text: r.warning || "Không bóc tách được" });
          }
        } catch (e) {
          out.push({ ...EMPTY_ROW, source_url: url });
          warns.push({ url, ok: false, text: e.message });
        }
      }
      setRows(out); setNotes(warns); setMode("manual"); setBusy("");
    };

    const submit = async () => {
      const products = rows.filter((r) => (r.name || "").trim() || (r.source_url || "").trim() || r.image_url);
      if (!products.length) return toast("Nhập ít nhất 1 sản phẩm (tên hoặc ảnh)");
      setBusy("score");
      try {
        const r = await api(`/api/benchmark/sessions/${session.id}/evaluate`, {
          method: "POST",
          body: {
            lang: LANG(),
            products: products.map((p) => ({
              name: p.name || null, source_url: p.source_url || null, image_url: p.image_url || null,
              note: [p.material && `Chất liệu: ${p.material}`, p.note,
                p.variants && `Biến thể: ${p.variants}`].filter(Boolean).join(" · ") || null,
              variants: p.variants || null,
              cogs: p.cogs === "" ? null : Number(p.cogs),
              price_expected: p.price_expected === "" ? null : Number(p.price_expected),
              media_count: p.media_count || (p.image_url ? 1 : 0),
            })),
          },
        });
        toast(`Đã chấm ${r.results.length} sản phẩm`);
        onDone(r.results.map((x) => x.id));
      } catch (e) { toast("Lỗi khi chấm điểm: " + e.message); } finally { setBusy(""); }
    };

    const MODES = [["manual", "Nhập thủ công", "keyboard"], ["file", "File CSV / Excel", "fileText"],
      ["url", "Từ URL sản phẩm", "link"]];

    return (
      <Drawer title="Nhập sản phẩm mới" subtitle={`Chấm điểm với bộ benchmark ${session.category} — ${session.country_name}`}
        onClose={onClose} width="max-w-2xl">
        <div className="grid grid-cols-3 gap-2 mb-5">
          {MODES.map(([k, label, icon]) => (
            <button key={k} onClick={() => setMode(k)}
              className={`px-3 py-2.5 rounded-[10px] border text-[13px] font-medium flex items-center gap-2 justify-center transition-colors ${
                mode === k ? "border-primary bg-softblue text-primary" : "border-gray-300 text-muted hover:bg-hover"}`}>
              <BIcon name={icon} className="w-4 h-4" />{label}
            </button>
          ))}
        </div>

        {mode === "file" && (
          <div className="space-y-3">
            <input ref={fileRef} type="file" accept=".csv,.tsv,.txt,.xlsx,.xlsm" className="hidden"
              onChange={(e) => e.target.files[0] && importFile(e.target.files[0])} />
            <div onClick={() => fileRef.current.click()}
              className="rounded-xl border-2 border-dashed border-gray-300 hover:bg-hover p-8 text-center cursor-pointer">
              <BIcon name="upload" className="w-6 h-6 mx-auto text-muted mb-2" />
              <div className="text-sm font-medium text-ink">Chọn file .csv hoặc .xlsx</div>
              <div className="text-xs text-muted mt-1">
                Cột nhận diện được: <code>Tên SP</code>, <code>URL SP</code>, <code>Giá vốn</code>,
                <code> Giá bán dự kiến</code>, <code>Ghi chú</code>, <code>Biến thể</code>
              </div>
            </div>
            {busy === "file" && <Spinner />}
          </div>
        )}

        {mode === "url" && (
          <div className="space-y-3">
            <Field label="Dán link sản phẩm (mỗi dòng 1 link)"
              hint="Hỗ trợ 1688 / Taobao / Tmall / Douyin / Shopify / supplier. Hệ thống tự lấy tên, ảnh, biến thể, chất liệu.">
              <textarea value={urls} onChange={(e) => setUrls(e.target.value)} rows={5}
                placeholder={"https://detail.1688.com/offer/123.html\nhttps://item.taobao.com/item.htm?id=456"}
                className="w-full px-3.5 py-2.5 rounded-[10px] border border-gray-300 text-sm bg-surface outline-none
                           focus:border-primary focus:ring-4 focus:ring-primary/15 transition" />
            </Field>
            <Button variant="primary" icon="sparkles" className="w-full" disabled={busy === "url"} onClick={fetchUrls}>
              {busy === "url" ? "Đang bóc tách trang nguồn…" : "Lấy thông tin từ URL"}
            </Button>
            <p className="text-xs text-muted">
              Một số sàn (1688/Taobao) chặn bot hoặc yêu cầu đăng nhập — khi đó hệ thống sẽ báo và bạn nhập tay phần còn thiếu.
            </p>
          </div>
        )}

        {mode === "manual" && (
          <div className="space-y-4">
            {!!notes.length && (
              <div className="space-y-1.5">
                {notes.map((n, i) => (
                  <div key={i} className={`text-[13px] px-3 py-2 rounded-[10px] border ${
                    n.ok ? "bg-green-50 border-green-100 text-ink" : "bg-amber-50 border-amber-100 text-amber-800"}`}>
                    <span className="font-medium break-all">{n.url}</span> — {n.text}
                  </div>
                ))}
              </div>
            )}

            <div className="space-y-4">
              {rows.map((r, i) => (
                <div key={i} className="p-3 rounded-xl border border-line space-y-3">
                  <div className="flex items-start gap-3">
                    <div className="shrink-0 cursor-pointer" onClick={() => { imgRef.current.dataset.row = i; imgRef.current.click(); }}>
                      <Thumb src={r.image_url} className="w-20 h-20" rounded="rounded-lg" />
                      <div className="text-[11px] text-primary text-center mt-1">Chọn ảnh</div>
                    </div>
                    <div className="flex-1 grid grid-cols-1 sm:grid-cols-2 gap-3">
                      <Field label="Tên sản phẩm">
                        <Input value={r.name} placeholder="Đầm maxi ren…" onChange={(e) => setRow(i, { name: e.target.value })} />
                      </Field>
                      <Field label="Chất liệu">
                        <Input value={r.material} placeholder="Chất liệu: cotton, linen, thép không gỉ…"
                          onChange={(e) => setRow(i, { material: e.target.value })} />
                      </Field>
                      <Field label={`Giá vốn (${session.currency})`}>
                        <Input type="number" value={r.cogs} onChange={(e) => setRow(i, { cogs: e.target.value })} />
                      </Field>
                      <Field label={`Giá bán dự kiến (${session.currency})`}>
                        <Input type="number" value={r.price_expected}
                          onChange={(e) => setRow(i, { price_expected: e.target.value })} />
                      </Field>
                    </div>
                    <button onClick={() => delRow(i)} className="text-muted hover:text-error shrink-0">
                      <BIcon name="trash" className="w-4 h-4" />
                    </button>
                  </div>
                  <Input value={r.variants} placeholder="Biến thể: màu Đen/Kem, size S-XXL"
                    onChange={(e) => setRow(i, { variants: e.target.value })} />
                  <Input value={r.note} placeholder="Ghi chú: mùa vụ, kích thước đóng gói…"
                    onChange={(e) => setRow(i, { note: e.target.value })} />
                  {/* Link chỉ hiện khi sản phẩm đến từ URL / file — nhập tay không cần */}
                  {!!r.source_url && (
                    <div className="text-xs text-muted truncate">
                      Nguồn: <a href={r.source_url} target="_blank" rel="noreferrer"
                        className="text-primary hover:underline">{r.source_url}</a>
                    </div>
                  )}
                </div>
              ))}
            </div>

            <input ref={imgRef} type="file" accept="image/*" multiple className="hidden"
              onChange={(e) => readImage(e.target.files, Number(imgRef.current.dataset.row || 0))} />

            <Button icon="plus" onClick={addRow}>Thêm sản phẩm</Button>

            <div className="flex gap-2 pt-2 border-t border-line">
              <Button variant="primary" icon="sparkles" className="flex-1" disabled={busy === "score"} onClick={submit}>
                {busy === "score" ? "AI đang chấm điểm…" : `Chấm điểm ${rows.length} sản phẩm`}
              </Button>
              <Button onClick={onClose}>Huỷ</Button>
            </div>
          </div>
        )}
      </Drawer>
    );
  }

  /* ------------------------- Scorecard ------------------------- */
  function Gauge({ score }) {
    const { ChartView } = U();
    const color = score >= 75 ? "#16A34A" : score >= 60 ? "#2563EB" : score >= 45 ? "#F59E0B" : "#DC2626";
    return (
      <div className="relative">
        <ChartView type="doughnut" height={180}
          data={{
            labels: ["Win", "Còn lại"],
            datasets: [{ data: [score, Math.max(0, 100 - score)], backgroundColor: [color, "#F1F3F7"],
              borderWidth: 0, cutout: "76%" }],
          }}
          options={{ rotation: -90, circumference: 180, plugins: { legend: { display: false }, tooltip: { enabled: false } } }} />
        <div className="absolute inset-0 flex flex-col items-center justify-center pt-8 pointer-events-none">
          <span className="text-4xl font-bold tabular-nums" style={{ color }}>{score}%</span>
          <span className="text-xs text-muted">Win Potential</span>
        </div>
      </div>
    );
  }

  function ScorecardDrawer({ id, onClose, onDeleted }) {
    const { Drawer, Badge, Button, Spinner, ChartView } = U();
    const [e, setE] = useState(null);
    useEffect(() => { api(`/api/benchmark/evaluations/${id}`).then(setE); }, [id]);
    if (!e) return <Drawer title="Đang tải…" onClose={onClose} width="max-w-3xl"><Spinner /></Drawer>;

    const SHORT = {
      visual_trend: "Trend hình ảnh", ad_traction: "Sức hút ads", margin: "Biên lợi nhuận",
      variants: "Biến thể", creative: "Creative hook", assets: "Kho media",
      localized_fit: "Bản địa hoá", logistics: "Kho vận",
    };
    const en = LANG() === "en";
    const labels = (e.criteria || []).map((c) => SHORT[c.key] || c.label_vi || c.key);
    const values = (e.criteria || []).map((c) => Number(c.score || 0));

    return (
      <Drawer title={e.product_name} subtitle={e.summary} onClose={onClose} width="max-w-3xl">
        {e.status === "error" ? (
          <div className="p-4 rounded-[10px] bg-red-50 text-error text-sm">Lỗi khi chấm điểm: {e.error}</div>
        ) : (
          <div className="space-y-6">
            {e.image_url && (
              <div className="flex items-center gap-4">
                <Thumb src={e.image_url} className="w-24 h-24" rounded="rounded-xl" />
                <div className="text-[13px] text-muted">
                  Chấm lúc {dt(e.created_at)} · nguồn dữ liệu {e.ai_source === "claude" ? "Claude AI" : "rule engine"}
                </div>
              </div>
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 items-center">
              <div>
                <Gauge score={Number(e.win_score || 0)} />
                <div className="flex justify-center gap-2 mt-1">
                  <Badge tone={VERDICT_TONE[e.verdict] || "slate"}>{e.verdict_label}</Badge>
                  <Badge tone="slate">{e.ai_source === "claude" ? "Claude AI" : "Rule engine"}</Badge>
                </div>
              </div>
              <ChartView type="radar" height={240}
                data={{
                  labels,
                  datasets: [{
                    label: "Điểm (0–10)", data: values,
                    backgroundColor: "rgba(37,99,235,.15)", borderColor: "#2563EB",
                    pointBackgroundColor: "#2563EB", borderWidth: 2,
                  }],
                }}
                options={{
                  layout: { padding: 4 },
                  scales: { r: { min: 0, max: 10, ticks: { stepSize: 2, font: { size: 9 }, backdropColor: "transparent" },
                    pointLabels: { font: { size: 10 }, color: "#6B7280" }, grid: { color: "#F1F3F7" },
                    angleLines: { color: "#F1F3F7" } } },
                  plugins: { legend: { display: false } },
                }} />
            </div>

            <div>
              <h4 className="text-[15px] font-semibold text-ink mb-2">Chi tiết 8 tiêu chí trọng số</h4>
              <div className="border border-line rounded-xl overflow-hidden">
                {(e.criteria || []).map((c, i) => (
                  <div key={c.key} className={`p-3 ${i ? "border-t border-line" : ""}`}>
                    <div className="flex items-center justify-between gap-3">
                      <span className="text-sm font-medium text-ink">
                        {en ? (c.label || c.label_vi) : (c.label_vi || c.label)} <span className="text-muted font-normal">({c.weight}%)</span>
                      </span>
                      <span className="text-sm font-semibold tabular-nums">{Number(c.score).toFixed(1)}/10</span>
                    </div>
                    <div className="h-1.5 rounded-full bg-hover mt-1.5 overflow-hidden">
                      <div className="h-full rounded-full bg-primary" style={{ width: `${Number(c.score) * 10}%` }} />
                    </div>
                    <p className="text-[13px] text-muted mt-1.5">{c.comment}</p>
                  </div>
                ))}
              </div>
            </div>

            {!!(e.matches || []).length && (
              <div>
                <h4 className="text-[15px] font-semibold text-ink mb-2">Top sản phẩm benchmark tương đồng</h4>
                <div className="space-y-2">
                  {e.matches.map((m, i) => (
                    <div key={i} className="p-3 rounded-[10px] border border-line flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <div className="text-sm font-medium text-ink truncate">{m.name}</div>
                        <div className="text-[13px] text-muted">{m.reason}</div>
                      </div>
                      <Badge tone="blue">{Math.round((m.similarity || 0) * 100)}% giống</Badge>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="p-4 rounded-xl bg-green-50 border border-green-100">
                <h4 className="text-[15px] font-semibold text-success mb-2">Điểm mạnh & cơ hội</h4>
                <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                  {(e.strengths || []).map((s, i) => <li key={i}>{s}</li>)}
                </ul>
              </div>
              <div className="p-4 rounded-xl bg-amber-50 border border-amber-100">
                <h4 className="text-[15px] font-semibold text-amber-700 mb-2">Cần cải thiện & hành động</h4>
                <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                  {(e.gaps || []).map((g, i) => <li key={i}>{g}</li>)}
                </ul>
              </div>
            </div>

            <div className="flex justify-between items-center pt-2">
              {e.source_url ? (
                <a href={e.source_url} target="_blank" rel="noreferrer" className="text-primary hover:underline text-sm break-all">
                  {e.source_url}
                </a>
              ) : <span />}
              <Button variant="danger" icon="trash" onClick={async () => {
                await api(`/api/benchmark/evaluations/${e.id}`, { method: "DELETE" });
                onDeleted();
              }}>Xoá báo cáo</Button>
            </div>
          </div>
        )}
      </Drawer>
    );
  }

  /* ===================================================================
     Root
     =================================================================== */
  function Benchmark() {
    const { PageHead, Spinner } = U();
    const [meta, setMeta] = useState(null);
    const [sessions, setSessions] = useState([]);
    const [session, setSession] = useState(null);   // null => màn hình danh sách
    const [creating, setCreating] = useState(false);

    const loadSessions = useCallback(async () => {
      const list = await api("/api/benchmark/sessions");
      setSessions(list);
      return list;
    }, []);

    useEffect(() => { api("/api/benchmark/meta").then(setMeta); loadSessions(); }, []);

    const open = async (id) => setSession(await api(`/api/benchmark/sessions/${id}`));
    const refreshSession = useCallback(async () => {
      if (!session) return;
      const s = await api(`/api/benchmark/sessions/${session.id}`);
      setSession(s);
      setSessions((ls) => ls.map((x) => (x.id === s.id ? s : x)));
    }, [session && session.id]);

    // Poll khi phiên đang mở đang cào dở
    useEffect(() => {
      if (!session || session.status !== "running") return;
      const t = setInterval(refreshSession, 1300);
      return () => clearInterval(t);
    }, [session && session.id, session && session.status, refreshSession]);

    if (!meta) return <Spinner />;

    return (
      <div>
        {!session && (
          <PageHead title="Benchmark & Sản phẩm tiềm năng"
            subtitle="Cào Meta Ads Library, AI lọc nhiễu ngành hàng và chấm % Win Potential cho sản phẩm mới" />
        )}

        {session ? (
          <SessionDetail session={session} onBack={() => { setSession(null); loadSessions(); }}
            onRefreshSession={refreshSession} />
        ) : (
          <SessionList meta={meta} sessions={sessions} onOpen={open}
            onCreate={() => setCreating(true)} onDeleted={loadSessions} />
        )}

        {creating && (
          <NewSessionDrawer meta={meta} onClose={() => { setCreating(false); loadSessions(); }}
            onDone={async (id) => { setCreating(false); await loadSessions(); open(id); }} />
        )}
      </div>
    );
  }

  window.BenchmarkPage = Benchmark;
})();
