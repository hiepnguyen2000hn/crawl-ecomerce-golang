/* MODULE 1C — NGHIÊN CỨU NGÁCH HẸP (Niche Research).

   Màn hình chia 4 vùng từ trên xuống:
     1. Quy mô & biến động thị trường   (đã làm — Bước 1.1 → 1.3)
     2. Sản phẩm tiềm năng               (chờ bổ sung)
     3. Đối thủ cạnh tranh               (chờ bổ sung)
     4. Nguồn hàng                       (chờ bổ sung)

   Vùng 1 xếp theo thứ tự: bộ từ khoá bản địa → biểu đồ đường 12 tháng (chọn/bỏ
   chọn từng từ khoá) → đánh giá mùa vụ → tệp đối tượng → nhận định.

   Nạp trước app.jsx; đọc window.UI lúc render, đăng ký qua window.NichePage. */
(function () {
  const { useState, useEffect, useMemo, useCallback, useRef } = React;

  const U = () => window.UI;
  const api = (p, o) => window.UI.api(p, o);
  const LANG = () => (window.I18N && window.I18N.lang) || "vi";
  const nf = (v) => Number(v || 0).toLocaleString("vi-VN");
  const pct = (v) => (v == null ? "—" : `${Math.round(v * 100)}%`);
  const compact = (v) => {
    v = Number(v || 0);
    if (v >= 1e6) return (v / 1e6).toFixed(1) + "M";
    if (v >= 1e3) return Math.round(v / 1e3) + "K";
    return String(v);
  };
  const flagOf = (code) => {
    const c = (code || "").trim().toUpperCase();
    if (!/^[A-Z]{2}$/.test(c)) return "🌐";
    return String.fromCodePoint(...[...c].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65));
  };
  const dt = (iso) => {
    if (!iso) return "chưa chạy";
    const d = new Date(iso); if (isNaN(d)) return "chưa chạy";
    const p = (n) => String(n).padStart(2, "0");
    return `${p(d.getDate())}/${p(d.getMonth() + 1)}/${d.getFullYear()} ${p(d.getHours())}:${p(d.getMinutes())}`;
  };
  /* Danh sách thị trường của phiên — tách từng tên để lớp i18n dịch được */
  function Markets({ session, max = 3 }) {
    const list = session.countries && session.countries.length
      ? session.countries : [{ code: session.country_code, name: session.country_name }];
    const shown = list.slice(0, max);
    return (
      <span className="inline-flex flex-wrap items-center gap-x-1">
        {shown.map((c, i) => (
          <span key={c.code}>{i ? ", " : ""}{flagOf(c.code)} <span>{c.name}</span></span>
        ))}
        {list.length > max && <span>+{list.length - max}</span>}
      </span>
    );
  }

  const MONTH_SHORT = ["T1", "T2", "T3", "T4", "T5", "T6", "T7", "T8", "T9", "T10", "T11", "T12"];
  const LINE_COLORS = ["#8B5CF6", "#EC4899", "#F59E0B", "#0EA5E9", "#84CC16",
                       "#F97316", "#14B8A6", "#A855F7", "#EF4444", "#6366F1"];

  const DEMAND_TONE = { evergreen: "green", seasonal: "amber", spike: "red" };
  const VERDICT_TONE = { too_narrow: "red", narrow: "amber", ideal: "green", wide: "blue", broad: "slate" };
  const KIND_LABEL = { core: "Cốt lõi", variant: "Biến thể", long_tail: "Đuôi dài" };
  const SEG_KIND_LABEL = { interest: "Sở thích", behavior: "Hành vi", demographic: "Nhân khẩu học" };
  const COMP_TONE = { LOW: "green", MEDIUM: "amber", HIGH: "red" };

  function NIcon({ name, className = "w-5 h-5" }) {
    const extra = {
      arrowLeft: '<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>',
      refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
      users: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
      lock: '<rect width="18" height="11" x="3" y="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
    };
    const html = extra[name] || (window.UI.ICONS || {})[name] || "";
    return <svg className={className} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
      dangerouslySetInnerHTML={{ __html: html }} />;
  }

  function Thumb({ src, alt, className = "w-full h-40", rounded = "rounded-lg" }) {
    const [bad, setBad] = useState(false);
    useEffect(() => setBad(false), [src]);
    if (!src || bad) {
      return (
        <div className={`${className} ${rounded} bg-hover flex items-center justify-center text-disabled`}>
          <NIcon name="image" className="w-5 h-5" />
        </div>
      );
    }
    return <img src={src} alt={alt || ""} onError={() => setBad(true)}
      className={`${className} ${rounded} object-cover bg-hover`} />;
  }

  /* Tiêu đề mỗi vùng của màn hình */
  function SectionHead({ index, title, subtitle, right }) {
    return (
      <div className="flex items-end justify-between gap-4 mb-4 pb-3 border-b-2 border-line">
        <div className="flex items-center gap-3">
          <span className="w-9 h-9 shrink-0 rounded-[10px] bg-primary text-white flex items-center
                           justify-center text-sm font-bold">{index}</span>
          <div>
            <h2 className="text-[20px] font-bold text-ink leading-tight">{title}</h2>
            {subtitle && <p className="text-[13px] text-muted mt-0.5">{subtitle}</p>}
          </div>
        </div>
        {right}
      </div>
    );
  }

  function ComingSoon({ note }) {
    return (
      <div className="rounded-xl border border-dashed border-gray-300 bg-bg px-6 py-10 text-center">
        <NIcon name="lock" className="w-6 h-6 mx-auto text-disabled mb-2" />
        <p className="text-sm font-medium text-muted">Phần này sẽ bổ sung ở bước tiếp theo</p>
        {note && <p className="text-[13px] text-disabled mt-1">{note}</p>}
      </div>
    );
  }

  /* ------------------------- tiến trình 3 bước ------------------------- */
  function StepProgress({ session, steps, prefix = "1" }) {
    const p = session.progress || 0;
    const err = session.status === "error";
    const state = (i) => {
      if (err) return p >= (steps[i - 1] ? steps[i - 1].until : 0) ? "error" : "todo";
      if (session.status === "done" && p >= 100) return "done";
      if (p >= steps[i].until) return "done";
      if (i === 0 || p >= steps[i - 1].until) return "active";
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
        <div className={`grid grid-cols-1 gap-3 ${
          steps.length >= 4 ? "sm:grid-cols-2 lg:grid-cols-4" : "sm:grid-cols-3"}`}>
          {steps.map((s, i) => {
            const st = state(i);
            return (
              <div key={s.key} className={`flex items-center gap-2.5 px-3 py-2.5 rounded-[10px] border ${
                st === "done" ? "border-green-200 bg-green-50" : st === "active" ? "border-primary bg-softblue"
                  : st === "error" ? "border-red-200 bg-red-50" : "border-line bg-surface"}`}>
                <span className={`w-7 h-6 shrink-0 rounded-full flex items-center justify-center text-[11px] font-semibold ${
                  st === "done" ? "bg-success text-white" : st === "active" ? "bg-primary text-white"
                    : st === "error" ? "bg-error text-white" : "bg-hover text-muted"}`}>
                  {st === "done" ? "✓" : st === "error" ? "!" : `${prefix}.${i + 1}`}
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

  /* =================== MÀN HÌNH A — danh sách phiên =================== */
  function SessionList({ meta, sessions, onOpen, onCreate, onDeleted }) {
    const { Card, Button, Badge, Empty } = U();
    const del = async (e, s) => {
      e.stopPropagation();
      if (!confirm(`Xoá phiên nghiên cứu ngách “${s.raw_keyword}”?`)) return;
      await api(`/api/niche/sessions/${s.id}`, { method: "DELETE" });
      onDeleted();
    };
    return (
      <div className="space-y-5">
        <div className="flex flex-wrap items-center gap-2 text-[13px]">
          <Badge tone={meta.volume_source === "google_ads" ? "green" : "amber"}>
            Dung lượng tìm kiếm: {meta.volume_source === "google_ads" ? "Google Ads API" : "Demo (chưa cấu hình Google Ads API)"}
          </Badge>
          <Badge tone={meta.audience_source === "api" ? "green" : "amber"}>
            Tệp đối tượng: {meta.audience_source === "api" ? "Meta Marketing API" : "Demo (chưa có token Meta)"}
          </Badge>
          <Badge tone={meta.ai_enabled ? "green" : "amber"}>
            AI: {meta.ai_enabled ? "Claude" : "Rule engine nội bộ"}
          </Badge>
        </div>

        <Card title={`Phiên nghiên cứu ngách (${sessions.length})`}
          action={<Button variant="primary" icon="plus" onClick={onCreate}>Nghiên cứu ngách mới</Button>}>
          {!sessions.length ? (
            <Empty icon="search" title="Chưa có ngách nào được nghiên cứu"
              hint="Nhập quốc gia target và keyword ngách (tiếng Việt hoặc tiếng Anh) để bắt đầu."
              action={<Button variant="primary" icon="plus" onClick={onCreate}>Nghiên cứu ngách mới</Button>} />
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
              {sessions.map((s) => (
                <div key={s.id} onClick={() => onOpen(s.id)}
                  className="niche-card p-4 rounded-xl border border-line hover:border-primary hover:shadow-softmd
                             transition-all cursor-pointer bg-surface group">
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="font-semibold text-ink truncate">{s.raw_keyword}</div>
                      <div className="text-[13px] text-muted mt-0.5 flex items-center gap-1.5">
                        <Markets session={s} max={2} />
                      </div>
                    </div>
                    {s.step1_score != null ? (
                      <div className="text-right shrink-0">
                        <div className={`text-2xl font-bold tabular-nums ${
                          s.step1_score >= 8 ? "text-success" : s.step1_score >= 5 ? "text-warning" : "text-error"}`}>
                          {s.step1_score}
                        </div>
                        <div className="text-[11px] text-muted">/10 điểm</div>
                      </div>
                    ) : (
                      <Badge tone={s.status === "running" ? "blue" : s.status === "error" ? "red" : "slate"}>
                        {s.status === "running" ? "đang chạy" : s.status}
                      </Badge>
                    )}
                  </div>
                  <div className="grid grid-cols-2 gap-2 mt-3">
                    <div className="py-1.5 px-2 rounded-lg bg-bg">
                      <div className="text-base font-bold tabular-nums text-ink">{nf(s.avg_monthly_searches)}</div>
                      <div className="text-[11px] text-muted">Lượt tìm/tháng</div>
                    </div>
                    <div className="py-1.5 px-2 rounded-lg bg-bg">
                      <div className="text-[13px] font-semibold text-ink truncate">
                        {s.demand_label ? s.demand_label.split("—")[0].trim() : "—"}
                      </div>
                      <div className="text-[11px] text-muted">Loại nhu cầu</div>
                    </div>
                  </div>
                  <div className="flex items-center justify-between mt-3 text-xs text-muted">
                    <span>Chạy lúc: {dt(s.last_run_at)}</span>
                    <button onClick={(e) => del(e, s)}
                      className="opacity-0 group-hover:opacity-100 transition-opacity text-muted hover:text-error">
                      <NIcon name="trash" className="w-4 h-4" />
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

  /* =================== DRAWER — tạo phiên mới =================== */
  /* Bộ chọn thị trường: tick cả châu lục, cả khu vực, hoặc từng nước */
  function MarketPicker({ tree, value, onChange }) {
    const [open, setOpen] = useState({});
    const [q, setQ] = useState("");
    const sel = new Set(value);

    const setMany = (codes, on) => {
      const n = new Set(sel);
      codes.forEach((c) => (on ? n.add(c) : n.delete(c)));
      onChange([...n]);
    };
    const codesOfRegion = (r) => r.countries.map((c) => c.code);
    const codesOfCont = (c) => c.regions.flatMap(codesOfRegion);
    const state = (codes) => {
      const on = codes.filter((c) => sel.has(c)).length;
      return on === 0 ? "off" : on === codes.length ? "all" : "some";
    };

    const Tick = ({ st, onClick }) => (
      <span onClick={(e) => { e.stopPropagation(); onClick(); }}
        className={`w-4 h-4 shrink-0 rounded border flex items-center justify-center text-[10px] cursor-pointer ${
          st === "all" ? "bg-primary border-primary text-white"
            : st === "some" ? "bg-white border-primary text-primary" : "border-gray-300 text-transparent"}`}>
        {st === "some" ? "–" : "✓"}
      </span>
    );

    const kw = q.trim().toLowerCase();
    const match = (c) => !kw || c.name.toLowerCase().includes(kw) || c.code.toLowerCase().includes(kw);

    return (
      <div className="border border-line rounded-[10px] overflow-hidden">
        <div className="px-3 py-2 border-b border-line bg-bg flex items-center gap-2">
          <input value={q} onChange={(e) => setQ(e.target.value)}
            placeholder="Tìm nhanh quốc gia…"
            className="flex-1 bg-transparent text-sm outline-none" />
          <span className="text-[12px] text-muted tabular-nums shrink-0">{sel.size} nước</span>
          {!!sel.size && (
            <button onClick={() => onChange([])}
              className="text-[12px] text-muted hover:text-error">Bỏ hết</button>
          )}
        </div>
        <div className="max-h-72 overflow-y-auto divide-y divide-line">
          {tree.map((cont) => {
            const cCodes = codesOfCont(cont);
            const visible = cont.regions
              .map((r) => ({ ...r, countries: r.countries.filter(match) }))
              .filter((r) => r.countries.length);
            if (!visible.length) return null;
            const isOpen = kw ? true : open[cont.key];
            return (
              <div key={cont.key}>
                <div className="px-3 py-2 flex items-center gap-2.5 bg-hover/60 cursor-pointer"
                  onClick={() => setOpen((o) => ({ ...o, [cont.key]: !o[cont.key] }))}>
                  <Tick st={state(cCodes)} onClick={() => setMany(cCodes, state(cCodes) !== "all")} />
                  <span className="text-sm font-semibold text-ink flex-1">{cont.label}</span>
                  <span className="text-[12px] text-muted tabular-nums">
                    {cCodes.filter((c) => sel.has(c)).length}/{cCodes.length}
                  </span>
                  <NIcon name={isOpen ? "chevronDown" : "chevronRight"} className="w-4 h-4 text-muted" />
                </div>
                {isOpen && visible.map((r) => {
                  const rCodes = codesOfRegion(r);
                  return (
                    <div key={r.key} className="pl-6 pr-3 py-1.5 border-t border-line">
                      <div className="flex items-center gap-2.5 py-1 cursor-pointer"
                        onClick={() => setMany(rCodes, state(rCodes) !== "all")}>
                        <Tick st={state(rCodes)} onClick={() => setMany(rCodes, state(rCodes) !== "all")} />
                        <span className="text-[13px] font-medium text-gray-700 flex-1">{r.label}</span>
                        <span className="text-[12px] text-muted tabular-nums">
                          {rCodes.filter((c) => sel.has(c)).length}/{rCodes.length}
                        </span>
                      </div>
                      <div className="flex flex-wrap gap-1.5 pl-6 pb-1">
                        {r.countries.map((c) => {
                          const on = sel.has(c.code);
                          return (
                            <button key={c.code} onClick={() => setMany([c.code], !on)}
                              className={`px-2 h-7 rounded-lg border text-[12px] flex items-center gap-1.5 ${
                                on ? "border-primary bg-softblue text-ink" : "border-line text-muted hover:bg-hover"}`}>
                              <span>{flagOf(c.code)}</span>{c.name}
                              <span className="text-disabled">{c.currency}</span>
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  function NewSessionDrawer({ meta, onClose, onDone }) {
    const { Drawer, Button, Field, Input, Select } = U();
    const toast = U().useToast();
    const [form, setForm] = useState({ country_codes: ["NL"], raw_keyword: "" });
    const [phase, setPhase] = useState("form");
    const [session, setSession] = useState(null);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
      if (phase !== "running" || !session) return;
      const t = setInterval(async () => {
        const s = await api(`/api/niche/sessions/${session.id}`);
        setSession(s);
        if (s.status !== "running") {
          clearInterval(t);
          if (s.status === "done") setTimeout(() => onDone(s.id), 900);
        }
      }, 1200);
      return () => clearInterval(t);
    }, [phase, session && session.id]);

    const start = async () => {
      if (!form.country_codes.length) return toast("Chọn ít nhất 1 thị trường");
      if (!form.raw_keyword.trim()) return toast("Nhập keyword ngách trước đã");
      setBusy(true);
      try {
        const s = await api("/api/niche/sessions", { method: "POST", body: form });
        await api(`/api/niche/sessions/${s.id}/run`, { method: "POST", body: { lang: LANG() } });
        setSession(await api(`/api/niche/sessions/${s.id}`));
        setPhase("running");
      } finally { setBusy(false); }
    };

    return (
      <Drawer title={phase === "form" ? "Nghiên cứu ngách mới" : "Đang phân tích quy mô thị trường"}
        subtitle={phase === "form" ? "Chọn thị trường target và nhập keyword ngách"
          : `${session.raw_keyword}`}
        onClose={onClose} width="max-w-xl">
        {phase === "running" ? (
          <div className="space-y-4">
            <StepProgress session={session} steps={meta.steps} />
            {session.status === "done" && (
              <div className="p-4 rounded-xl bg-green-50 border border-green-100 text-sm text-ink">
                Xong Bước 1 — điểm {session.step1_score}/10. Đang mở hồ sơ ngách…
              </div>
            )}
            {session.status === "error" && (
              <Button variant="primary" className="w-full" onClick={() => setPhase("form")}>Quay lại</Button>
            )}
          </div>
        ) : (
          <div className="space-y-4">
            <Field label="Thị trường target"
              hint="Tick cả châu lục, cả khu vực (Tây Âu, Trung Âu, Bắc Mỹ…) hoặc từng nước. Nước đầu tiên là thị trường chính — quyết định tiền tệ, ngôn ngữ bản địa và benchmark quảng cáo.">
              <MarketPicker tree={meta.region_tree || []} value={form.country_codes}
                onChange={(v) => setForm({ ...form, country_codes: v })} />
            </Field>
            {!!form.country_codes.length && (
              <div className="flex flex-wrap gap-1.5">
                {form.country_codes.map((c, i) => (
                  <span key={c} className={`px-2 h-7 rounded-lg text-[12px] flex items-center gap-1.5 border ${
                    i === 0 ? "border-primary bg-softblue text-ink font-medium" : "border-line text-muted"}`}>
                    {flagOf(c)} {c}{i === 0 ? " · thị trường chính" : ""}
                  </span>
                ))}
              </div>
            )}
            <Field label="Keyword ngách / sản phẩm"
              hint="Nhập bằng tiếng Việt hoặc tiếng Anh — AI sẽ tự chuyển sang cách người bản địa gõ tìm kiếm.">
              <Input value={form.raw_keyword} placeholder="váy oversized cho nữ / dụng cụ thái gọt hoa quả"
                onChange={(e) => setForm({ ...form, raw_keyword: e.target.value })}
                onKeyDown={(e) => e.key === "Enter" && start()} />
            </Field>
            <div className="p-3 rounded-[10px] bg-bg border border-line text-[13px] text-muted space-y-1">
              <div className="font-medium text-gray-700">Vùng 1 — Quy mô & biến động thị trường sẽ chạy:</div>
              {meta.steps.map((s, i) => <div key={s.key}>1.{i + 1} — {s.label}</div>)}
            </div>
            <div className="flex gap-2 pt-2">
              <Button variant="primary" icon="play" className="flex-1" disabled={busy} onClick={start}>
                {busy ? "Đang khởi chạy…" : "Bắt đầu nghiên cứu"}
              </Button>
              <Button onClick={onClose}>Huỷ</Button>
            </div>
          </div>
        )}
      </Drawer>
    );
  }

  /* =================== VÙNG 1 — các block =================== */

  /* 1b. Biểu đồ đường 12 tháng — chọn/bỏ chọn từng từ khoá */
  function TrendBlock({ kws, source }) {
    const { Card, ChartView, Empty, Badge } = U();
    const [off, setOff] = useState({});          // keyword id -> bị bỏ chọn
    const selected = kws.filter((k) => !off[k.id]);

    const { labels, total, series } = useMemo(() => {
      const base = (kws.find((k) => (k.monthly_volumes || []).length) || {}).monthly_volumes || [];
      const labels = base.map((v) => `${MONTH_SHORT[v.month - 1]}/${String(v.year).slice(2)}`);
      const sum = base.map((_, i) =>
        selected.reduce((acc, k) => acc + (((k.monthly_volumes || [])[i] || {}).searches || 0), 0));
      const series = selected.map((k) => ({
        id: k.id, keyword: k.keyword,
        color: LINE_COLORS[kws.indexOf(k) % LINE_COLORS.length],
        data: (k.monthly_volumes || []).map((v) => v.searches),
      }));
      return { labels, total: sum, series };
    }, [kws, JSON.stringify(off)]);

    if (!labels.length) {
      return <Card title="Lượt tìm kiếm 12 tháng"><Empty icon="search" title="Chưa có dữ liệu lượt tìm kiếm" /></Card>;
    }
    const peak = Math.max(...total);
    const peakIdx = total.indexOf(peak);

    return (
      <Card title="Lượt tìm kiếm thực tế 12 tháng"
        action={<Badge tone="slate">{source === "claude" ? "Claude AI" : "Rule engine"}</Badge>}>
        <ChartView type="line" height={300}
          data={{
            labels,
            datasets: [
              { label: "Tổng ngách", data: total, borderColor: "#2563EB", backgroundColor: "rgba(37,99,235,.10)",
                borderWidth: 3, fill: true, tension: 0.35, pointRadius: 3, pointBackgroundColor: "#2563EB", order: 0 },
              ...series.map((s) => ({
                label: s.keyword, data: s.data, borderColor: s.color, backgroundColor: s.color,
                borderWidth: 1.5, fill: false, tension: 0.35, pointRadius: 0, borderDash: [5, 4], order: 1,
              })),
            ],
          }}
          options={{
            interaction: { mode: "index", intersect: false },
            plugins: { legend: { display: true, position: "bottom",
              labels: { boxWidth: 10, boxHeight: 10, font: { size: 11 } } } },
          }} />

        {/* Danh sách từ khoá bản địa — bấm để bật/tắt khỏi đồ thị */}
        <div className="mt-4 divide-y divide-line border-t border-line">
          {kws.map((k, i) => {
            const on = !off[k.id];
            const color = LINE_COLORS[i % LINE_COLORS.length];
            return (
              <button key={k.id} onClick={() => setOff((o) => ({ ...o, [k.id]: on }))}
                className={`w-full flex items-center gap-3 py-2.5 text-left transition-colors ${
                  on ? "" : "opacity-45"}`}>
                <span className="w-3 h-3 rounded-full shrink-0"
                  style={{ background: on ? color : "#D1D5DB" }} />
                <span className="min-w-0 flex-1">
                  <span className={`font-medium text-ink ${on ? "" : "line-through"}`}>{k.keyword}</span>
                  {/* nghĩa tiếng Việt của từ khoá bản địa */}
                  {(k.translation || k.note) && (
                    <span className="text-muted"> ({k.translation || k.note})</span>
                  )}
                </span>
                <span className="text-[13px] text-muted tabular-nums shrink-0">
                  {nf(k.avg_monthly_searches)} lượt/tháng
                </span>
              </button>
            );
          })}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 mt-3 text-[13px] text-muted">
          <span>
            Đang tính {selected.length}/{kws.length} từ khoá · đỉnh {nf(peak)} lượt vào {labels[peakIdx] || "—"}
          </span>
          <span>Bỏ chọn một từ khoá sẽ trừ lượt tìm của từ đó khỏi đường tổng.</span>
        </div>
      </Card>
    );
  }

  /* 1c. Đánh giá mùa vụ */
  function ScoreGauge({ score, label = "điểm Bước 1" }) {
    const { ChartView } = U();
    const v = Number(score || 0);
    const color = v >= 8 ? "#16A34A" : v >= 5 ? "#F59E0B" : "#DC2626";
    return (
      <div className="relative">
        <ChartView type="doughnut" height={170}
          data={{ labels: ["Điểm", "Còn lại"],
            datasets: [{ data: [v, Math.max(0, 10 - v)], backgroundColor: [color, "#F1F3F7"],
              borderWidth: 0, cutout: "76%" }] }}
          options={{ rotation: -90, circumference: 180,
            plugins: { legend: { display: false }, tooltip: { enabled: false } } }} />
        <div className="absolute inset-0 flex flex-col items-center justify-center pt-7 pointer-events-none">
          <span className="text-4xl font-bold tabular-nums" style={{ color }}>{v}</span>
          <span className="text-xs text-muted">/10 {label}</span>
        </div>
      </div>
    );
  }

  const TREND_LABEL = { rising: ["green", "Đang vào mùa"], stable: ["slate", "Đi ngang"],
                        falling: ["amber", "Đã qua đỉnh"] };

  /* 1b. Chấm điểm & nhận định RIÊNG cho mục "lượt tìm kiếm → mùa vụ" — nằm ngay dưới đồ thị */
  function SeasonalityBlock({ s }) {
    const { Card, Badge } = U();
    const peaks = s.peak_months || [];
    const lows = s.low_months || [];
    const tr = TREND_LABEL[s.trend_direction] || null;
    const rows = [
      ["Nhãn nhu cầu", <span key="d" className="font-medium text-ink">{s.demand_label || "—"}</span>],
      ["Mức độ biến động", <Badge key="v" tone={s.volatility === "low" ? "green" : s.volatility === "medium" ? "amber" : "red"}>
        {s.volatility_label || "—"}</Badge>],
      ["Fluctuation Ratio", <span key="f" className="font-medium text-ink tabular-nums">{pct(s.fluctuation_ratio)}</span>],
      ["Hệ số biến thiên (CV)", <span key="c" className="font-medium text-ink tabular-nums">{s.cv == null ? "—" : s.cv}</span>],
      ["Tháng vàng", <span key="p" className="font-medium text-ink">
        {peaks.length ? peaks.map((m) => MONTH_SHORT[m - 1]).join(", ") : "—"}</span>],
      ["Tháng thấp điểm", <span key="l" className="font-medium text-ink">
        {lows.length ? lows.map((m) => MONTH_SHORT[m - 1]).join(", ") : "—"}</span>],
    ];
    return (
      <Card title="Đánh giá mùa vụ (từ lượt tìm kiếm 12 tháng)" action={
        <div className="flex items-center gap-2">
          {tr && <Badge tone={tr[0]}>{tr[1]}</Badge>}
          <Badge tone="slate">{s.ai_source === "claude" ? "Claude AI" : "Rule engine"}</Badge>
        </div>}>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-5">
          <div>
            <ScoreGauge score={s.step1_score} label="điểm mùa vụ" />
          </div>
          <div className="space-y-2 lg:border-l lg:border-line lg:pl-5">
            {rows.map(([label, node]) => (
              <div key={label} className="flex items-center justify-between gap-2 text-[13px]">
                <span className="text-muted shrink-0">{label}</span>{node}
              </div>
            ))}
          </div>
          <div className="space-y-3">
            {s.seasonality_note && (
              <div className="p-3 rounded-xl bg-softblue border border-primary/25">
                <h5 className="text-[13px] font-semibold text-ink mb-1">Nhận định mùa vụ</h5>
                <p className="text-[13px] text-ink">{s.seasonality_note}</p>
              </div>
            )}
            {s.ai_summary && <p className="text-[13px] text-muted">{s.ai_summary}</p>}
          </div>
        </div>
      </Card>
    );
  }

  /* 1c. Chấm điểm & nhận định RIÊNG cho mục "tệp đối tượng" — nằm ngay dưới bộ dựng tệp */
  function AudienceScoreBlock({ quality, session }) {
    const { Card, Badge } = U();
    if (!quality) return null;
    const tone = (v) => (v >= 8 ? "#16A34A" : v >= 5.5 ? "#F59E0B" : "#DC2626");
    return (
      <Card title="Đánh giá tệp đối tượng" action={
        <Badge tone={quality.score >= 8 ? "green" : quality.score >= 5.5 ? "amber" : "red"}>
          {quality.label}
        </Badge>}>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-5">
          <div>
            <ScoreGauge score={quality.score} label="điểm tệp đối tượng" />
          </div>
          <div className="space-y-3 lg:border-l lg:border-line lg:pl-5">
            {quality.criteria.map((c) => (
              <div key={c.key}>
                <div className="flex items-center justify-between gap-2 text-[13px]">
                  <span className="text-gray-700">
                    {c.label} <span className="text-disabled">×{pct(c.weight)}</span>
                  </span>
                  <span className="tabular-nums font-semibold" style={{ color: tone(c.score) }}>{c.score}</span>
                </div>
                <div className="h-1.5 rounded-full bg-hover overflow-hidden mt-1">
                  <div className="h-full rounded-full"
                    style={{ width: `${c.score * 10}%`, background: tone(c.score) }} />
                </div>
                <p className="text-[12px] text-muted mt-1 leading-snug">{c.note}</p>
              </div>
            ))}
          </div>
          <div className="p-3 rounded-xl bg-green-50 border border-green-100">
            <h5 className="text-[13px] font-semibold text-success mb-1.5">Cần làm gì để tệp tốt hơn</h5>
            <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
              {(quality.actions || []).map((a, i) => <li key={i}>{a}</li>)}
            </ul>
          </div>
        </div>
      </Card>
    );
  }

  /* 1d. Bộ dựng tệp đối tượng — tích/bỏ tích từng lát cắt, quy mô cập nhật theo */
  function AudienceBuilder({ sessionId, audience, segments, bands, onSegmentsChanged, onQuality }) {
    const { Card, Badge, Button, Empty, Spinner, Input } = U();
    const toast = U().useToast();
    const [q, setQ] = useState("");
    const [found, setFound] = useState(null);
    const [searching, setSearching] = useState(false);
    const groups = [
      { key: "country", title: "Quốc gia", kinds: ["country"], locked: true },
      { key: "age", title: "Độ tuổi", kinds: ["age"] },
      { key: "gender", title: "Giới tính", kinds: ["gender"] },
      { key: "target", title: "Sở thích · Hành vi · Nhân khẩu học", kinds: ["interest", "behavior", "demographic"] },
    ];
    const all = useMemo(() => {
      const out = {};
      groups.forEach((g) => { out[g.key] = g.kinds.flatMap((k) => (segments && segments[k]) || []); });
      return out;
    }, [segments]);
    const defaults = useMemo(
      () => new Set(Object.values(all).flat().filter((x) => x.selected).map((x) => x.id)),
      [all]);

    const [sel, setSel] = useState(defaults);
    const [res, setRes] = useState(null);
    const [busy, setBusy] = useState(false);
    useEffect(() => setSel(defaults), [defaults]);

    // đổi lựa chọn -> gọi lại API ước lượng (debounce)
    useEffect(() => {
      if (!Object.values(all).flat().length) return;
      setBusy(true);
      const t = setTimeout(async () => {
        try {
          setRes(await api(`/api/niche/sessions/${sessionId}/audience/estimate`, {
            method: "POST", body: { selected_ids: [...sel] },
          }));
        } finally { setBusy(false); }
      }, 350);
      return () => clearTimeout(t);
    }, [JSON.stringify([...sel].sort()), sessionId]);

    // tra cứu mục Detailed Targeting trên Meta rồi bổ sung vào bộ lọc
    useEffect(() => {
      if (!q.trim()) { setFound(null); return; }
      setSearching(true);
      const t = setTimeout(async () => {
        try {
          const r = await api(`/api/niche/sessions/${sessionId}/targeting/search?q=${encodeURIComponent(q)}`);
          setFound(r.results);
        } finally { setSearching(false); }
      }, 400);
      return () => clearTimeout(t);
    }, [q, sessionId]);

    const addTarget = async (item) => {
      await api(`/api/niche/sessions/${sessionId}/targeting`, { method: "POST", body: item });
      setQ(""); setFound(null);
      toast(`Đã thêm “${item.label}” vào bộ lọc tệp`);
      onSegmentsChanged && onSegmentsChanged();
    };
    const removeTarget = async (e, id) => {
      e.stopPropagation();
      await api(`/api/niche/sessions/${sessionId}/targeting/${id}`, { method: "DELETE" });
      onSegmentsChanged && onSegmentsChanged();
    };

    const toggle = (id) => setSel((s0) => {
      const n = new Set(s0);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });
    const metaAud = (audience || []).find((a) => a.platform === "meta");
    const shown = res || metaAud;
    const quality = res && res.quality;
    useEffect(() => { if (quality && onQuality) onQuality(quality); }, [quality]);
    if (!Object.values(all).flat().length) {
      return <Card title="Tệp đối tượng (Meta)"><Empty icon="users" title="Chưa có số liệu tệp đối tượng" /></Card>;
    }

    return (
      <Card title="Tệp đối tượng (Meta)" action={
        <div className="flex items-center gap-2">
          <Badge tone="slate">{(shown && shown.source) === "api" ? "Meta API" : "demo"}</Badge>
          <Button size="sm" icon="refresh" onClick={() => setSel(new Set(defaults))}>Về gợi ý AI</Button>
        </div>}>
        <div className="grid grid-cols-1 lg:grid-cols-5 gap-5">
          {/* kết quả quy mô tệp */}
          <div className="lg:col-span-2">
            <div className="p-4 rounded-xl border border-primary bg-softblue sticky top-20">
              <div className="flex items-center justify-between gap-2">
                <span className="text-[13px] text-muted">Quy mô tệp theo lựa chọn</span>
                {shown && <Badge tone={VERDICT_TONE[shown.verdict] || "slate"}>{shown.verdict}</Badge>}
              </div>
              <div className="text-[26px] font-bold text-ink tabular-nums mt-1 leading-tight">
                {busy ? "…" : shown ? `${nf(shown.lower_bound)} – ${nf(shown.upper_bound)}` : "—"}
              </div>
              {shown && <p className="text-[13px] text-muted mt-1">{shown.note}</p>}

              <div className="mt-3 pt-3 border-t border-white/60 space-y-1 text-[12px] text-muted">
                {groups.map((g) => (
                  <div key={g.key} className="flex items-center justify-between">
                    <span>{g.title}</span>
                    <span className="tabular-nums">
                      {all[g.key].filter((x) => sel.has(x.id)).length}/{all[g.key].length}
                    </span>
                  </div>
                ))}
              </div>
              <p className="text-[12px] text-disabled mt-3">
                Bỏ chọn bớt tiêu chí → tệp mở rộng. Không chọn mục targeting nào = không lọc
                Detailed Targeting (tệp rộng nhất).
              </p>
            </div>
          </div>

          {/* các lát cắt để tích chọn */}
          <div className="lg:col-span-3 space-y-4">
            {groups.map((g) => (
              <div key={g.key}>
                <div className="flex items-center justify-between mb-2">
                  <span className="text-[13px] font-semibold text-gray-700">
                    {g.title}
                    {/* không tích gì = không lọc theo tiêu chí này -> tệp rộng nhất */}
                    {!g.locked && !all[g.key].some((x) => sel.has(x.id)) && (
                      <span className="ml-2 font-normal text-muted">— không lọc, tính toàn bộ</span>
                    )}
                  </span>
                  {g.locked ? <span className="text-[12px] text-muted">cố định theo phiên nghiên cứu</span>
                    : <button onClick={() => setSel((s0) => {
                    const n = new Set(s0);
                    const ids = all[g.key].map((x) => x.id);
                    const allOn = ids.every((i) => n.has(i));
                    ids.forEach((i) => (allOn ? n.delete(i) : n.add(i)));
                    return n;
                  })} className="text-[12px] text-primary hover:underline">
                    {all[g.key].every((x) => sel.has(x.id)) ? "Bỏ chọn tất cả" : "Chọn tất cả"}
                  </button>}
                </div>
                <div className="space-y-1.5">
                  {all[g.key].map((x) => {
                    const on = sel.has(x.id);
                    return (
                      <button key={x.id} onClick={() => !g.locked && toggle(x.id)}
                        className={`w-full text-left px-3 py-2 rounded-[10px] border flex items-center gap-3 transition-colors ${
                          g.locked ? "border-primary bg-softblue cursor-default"
                            : on ? "border-primary bg-softblue" : "border-line hover:bg-hover"}`}>
                        <span className={`w-4 h-4 shrink-0 rounded border flex items-center justify-center text-[10px] ${
                          on ? "bg-primary border-primary text-white" : "border-gray-300 text-transparent"}`}>✓</span>
                        <span className="min-w-0 flex-1">
                          <span className="text-sm font-medium text-ink block truncate">
                            {g.locked && x.external_id ? `${flagOf(x.external_id)} ` : ""}{x.label}</span>
                          {(x.label_vi || x.note) && (
                            <span className="text-[12px] text-muted block truncate">{x.label_vi || x.note}</span>
                          )}
                        </span>
                        {x.kind !== "age" && x.kind !== "gender" && (
                          <Badge tone={x.kind === "behavior" ? "amber" : x.kind === "demographic" ? "blue" : "slate"}>
                            {SEG_KIND_LABEL[x.kind] || x.kind}
                          </Badge>
                        )}
                        <span className="text-[12px] text-muted tabular-nums shrink-0 w-24 text-right">
                          {x.share != null ? `${pct(x.share)} · ` : ""}{compact(x.size_upper)}
                        </span>
                        {g.key === "target" && (
                          <span onClick={(e) => removeTarget(e, x.id)}
                            className="text-muted hover:text-error shrink-0" title="Bỏ khỏi danh sách">
                            <NIcon name="x" className="w-3.5 h-3.5" />
                          </span>
                        )}
                      </button>
                    );
                  })}
                </div>
                {g.key === "target" && (
                  <div className="mt-2">
                    <Input value={q} onChange={(e) => setQ(e.target.value)}
                      placeholder="Tìm & thêm sở thích / hành vi / nhân khẩu học của Meta…" />
                    {searching && <div className="text-[12px] text-muted mt-1">Đang tra cứu…</div>}
                    {found && !!found.length && (
                      <div className="mt-1.5 border border-line rounded-[10px] divide-y divide-line
                                      max-h-56 overflow-y-auto">
                        {found.map((f, i) => (
                          <button key={i} onClick={() => addTarget(f)}
                            className="w-full text-left px-3 py-2 hover:bg-hover flex items-center gap-2">
                            <span className="min-w-0 flex-1">
                              <span className="text-sm text-ink block truncate">{f.label}</span>
                              {f.label_vi && <span className="text-[12px] text-muted">{f.label_vi}</span>}
                            </span>
                            <Badge tone={f.kind === "behavior" ? "amber"
                              : f.kind === "demographic" ? "blue" : "slate"}>
                              {SEG_KIND_LABEL[f.kind] || f.kind}
                            </Badge>
                            <span className="text-[12px] text-muted tabular-nums">
                              {compact(f.size_upper)}
                            </span>
                          </button>
                        ))}
                      </div>
                    )}
                    {found && !found.length && (
                      <div className="text-[12px] text-muted mt-1">Không tìm thấy mục phù hợp.</div>
                    )}
                  </div>
                )}
              </div>
            ))}

            <div className="pt-2 border-t border-line space-y-1">
              {(bands || []).map((b) => (
                <div key={b.key} className="flex items-center gap-2 text-[12px] text-muted pt-1">
                  <span className={`w-2 h-2 rounded-full shrink-0 ${
                    b.key === "ideal" ? "bg-success" : b.key === "too_narrow" ? "bg-error"
                      : b.key === "narrow" ? "bg-warning" : "bg-gray-300"}`} />
                  <span className="tabular-nums">{compact(b.min)}{b.max ? ` – ${compact(b.max)}` : "+"}</span>
                  <span className="truncate">{b.note}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </Card>
    );
  }

  /* =================== VÙNG 2 — Bảng xếp hạng sản phẩm tiềm năng =================== */
  const AD_STATUS = { valid: ["green", "Đạt"], irrelevant: ["slate", "Sai ngách"],
                      policy_risk: ["red", "Rủi ro chính sách"] };
  const AUD_MATCH = { match: ["green", "Khớp tệp"], partial: ["amber", "Lệch một phần"],
                      mismatch: ["red", "Lệch hẳn"], unknown: ["slate", "Không rõ"] };
  const GENDER_VI = { female: "Nữ", male: "Nam", unknown: "Không rõ" };
  const AUD_BG = "bg-[#F5F3FF]";                       /* tệp ads — tím nhạt */
  const AUD_HEAD = "!text-violet-900 bg-[#DDD6FE] border-l-2 border-violet-400";

  const MATCH_STATUS = {
    matched_both: ["green", "Khớp cả 2 nguồn"],
    ads_only: ["amber", "Chỉ có Ads"],
    ecom_only: ["blue", "Chỉ có Ecom"],
  };
  /* Hai vùng dữ liệu tô màu tách bạch: xanh dương = Spy Ads · xanh lá = sàn TMĐT */
  const ADS_BG = "bg-[#EFF6FF]";                       /* xanh lạnh */
  const ADS_HEAD = "!text-blue-900 bg-[#BFDBFE]";
  const ECOM_BG = "bg-[#FFF7ED]";                      /* cam ấm — tương phản nóng/lạnh, dễ tách bằng mắt */
  const ECOM_HEAD = "!text-orange-900 bg-[#FED7AA]";
  const GROUP_EDGE_ADS = "border-l-2 border-blue-400";
  const GROUP_EDGE_ECOM = "border-l-2 border-orange-400";
  /* Cột đầu giữ cố định khi kéo ngang */
  const STICK_RANK = "sticky left-0 z-10 bg-surface w-[68px] min-w-[68px]";
  const STICK_RANK_HEAD = "sticky left-0 z-20 bg-[#F9FAFB] w-[68px] min-w-[68px]";
  const STICK_NAME = "sticky left-[68px] z-10 bg-surface shadow-[6px_0_6px_-4px_rgba(16,24,40,.10)]";
  const STICK_NAME_HEAD = "sticky left-[68px] z-20 bg-[#F9FAFB] shadow-[6px_0_6px_-4px_rgba(16,24,40,.10)]";
  const NO_ECOM = "⚠️ Không có dữ liệu trên sàn E-commerce";
  const NO_ADS = "⚠️ Chưa phát hiện Ads chạy trả phí";

  /* Nguồn 1 thô — toàn bộ ads đã lọc (kể cả bị loại) */
  /* Logo + thương hiệu sàn TMĐT — nhận diện bằng mắt nhanh hơn đọc chữ.
     Vẽ bằng SVG/CSS inline nên không phụ thuộc CDN ảnh của bên thứ ba. */
  const SHOPS = {
    amazon:     { label: "Amazon",     bg: "#FF9900", fg: "#131A22", mark: "a" },
    aliexpress: { label: "AliExpress", bg: "#E62E04", fg: "#FFFFFF", mark: "Ali" },
    bol:        { label: "bol.com",    bg: "#0000A4", fg: "#FFFFFF", mark: "bol" },
    ebay:       { label: "eBay",       bg: "#FFFFFF", fg: "#E53238", mark: "e", multi: true },
    etsy:       { label: "Etsy",       bg: "#F1641E", fg: "#FFFFFF", mark: "E" },
    otto:       { label: "OTTO",       bg: "#D4021D", fg: "#FFFFFF", mark: "O" },
    walmart:    { label: "Walmart",    bg: "#0071CE", fg: "#FFC220", mark: "W" },
    shopee:     { label: "Shopee",     bg: "#EE4D2D", fg: "#FFFFFF", mark: "S" },
    lazada:     { label: "Lazada",     bg: "#0F146D", fg: "#FFFFFF", mark: "L" },
    cdiscount:  { label: "Cdiscount",  bg: "#EE1D23", fg: "#FFFFFF", mark: "C" },
    allegro:    { label: "Allegro",    bg: "#FF5A00", fg: "#FFFFFF", mark: "A" },
    /* nguồn sỉ — Bước 4 */
    "1688":     { label: "1688.com",   bg: "#FF6A00", fg: "#FFFFFF", mark: "16" },
    taobao:     { label: "Taobao",     bg: "#FF4400", fg: "#FFFFFF", mark: "淘" },
    alibaba:    { label: "Alibaba",    bg: "#FF6A00", fg: "#FFFFFF", mark: "Ali" },
    eu_stock:   { label: "Kho EU",     bg: "#1D4ED8", fg: "#FFFFFF", mark: "EU" },
  };
  const EBAY_COLORS = ["#E53238", "#0064D2", "#F5AF02", "#86B817"];

  function ShopLogo({ platform, size = "sm" }) {
    if (!platform) return <span className="text-muted">—</span>;
    const key = String(platform).toLowerCase().replace(/\.com$|\s+/g, "");
    const s = SHOPS[key] || { label: platform, bg: "#E4E7EC", fg: "#475467",
                              mark: String(platform).slice(0, 1).toUpperCase() };
    const box = size === "lg" ? "w-7 h-7 text-[11px]" : "w-6 h-6 text-[10px]";
    return (
      <span className="inline-flex items-center gap-2 whitespace-nowrap">
        <span className={`${box} rounded-md flex items-center justify-center font-black shrink-0
                          border border-black/10 leading-none`}
          style={{ background: s.bg, color: s.fg }} aria-hidden="true">{s.mark}</span>
        <span className="text-[13px] font-semibold" style={{ color: s.multi ? "#111827" : s.fg === "#FFFFFF" ? s.bg : s.fg }}>
          {s.multi
            ? s.label.split("").map((ch, i) => (
                <span key={i} style={{ color: EBAY_COLORS[i % 4] }}>{ch}</span>))
            : s.label}
        </span>
      </span>
    );
  }

  function SourceAdsTable({ rows }) {
    const { Card, Badge, Spinner, Empty, DataTable } = U();
    if (!rows) return <Spinner />;
    if (!rows.length) return <Empty icon="search" title="Chưa có dữ liệu Nguồn 1" />;
    return (
      <Card title={`Nguồn 1 — Quảng cáo thu thập được (${rows.length})`}>
        <DataTable rows={rows} pageSize={10} searchKeys={["product_guess", "page_name", "ad_copy"]}
          filters={[{ key: "ai_status", label: "AI lọc", options: ["valid", "irrelevant", "policy_risk"] },
                    { key: "platform", label: "Nền tảng", options: [...new Set(rows.map((r) => r.platform))] }]}
          columns={[
            { key: "product_guess", label: "Sản phẩm (AI suy ra)",
              render: (r) => (
                <div className="min-w-[200px]">
                  <div className="font-medium text-ink truncate max-w-[220px]">{r.product_guess}</div>
                  <div className="text-[12px] text-muted truncate max-w-[220px]">{r.page_name}</div>
                </div>) },
            { key: "ai_status", label: "AI lọc",
              render: (r) => {
                const st = AD_STATUS[r.ai_status] || ["slate", r.ai_status];
                return <Badge tone={st[0]}>{st[1]}</Badge>;
              } },
            { key: "active_days", label: "Ngày active", align: "right",
              render: (r) => <span className="tabular-nums">{r.active_days ?? "—"}</span> },
            { key: "reach", label: "Reach", align: "right",
              render: (r) => <span className="tabular-nums">{compact(r.reach)}</span> },
            { key: "platform", label: "Nền tảng" },
            { key: "matched_keyword", label: "Từ khoá" },
            { key: "cluster_id", label: "Cụm SKU", align: "right",
              render: (r) => <span className="tabular-nums text-muted">{r.cluster_id || "—"}</span> },
            { key: "ad_link", label: "Link", sortable: false,
              render: (r) => (r.ad_link
                ? <a href={r.ad_link} target="_blank" rel="noreferrer"
                    className="text-primary hover:underline text-[13px]">Mở</a>
                : <span className="text-disabled">—</span>) },
          ]} />
      </Card>
    );
  }

  /* Nguồn 2 thô — sản phẩm trên sàn TMĐT */
  function SourceEcomTable({ rows }) {
    const { Card, Badge, Spinner, Empty, DataTable } = U();
    if (!rows) return <Spinner />;
    if (!rows.length) return <Empty icon="package" title="Chưa có dữ liệu Nguồn 2" />;
    return (
      <Card title={`Nguồn 2 — Sản phẩm trên sàn TMĐT (${rows.length})`}>
        <DataTable rows={rows} pageSize={10} searchKeys={["name", "matched_keyword"]}
          filters={[{ key: "platform", label: "Sàn", options: [...new Set(rows.map((r) => r.platform))] }]}
          columns={[
            { key: "name", label: "Sản phẩm",
              render: (r) => (
                <div className="flex items-center gap-2.5 min-w-[220px]">
                  <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                  <div className="min-w-0">
                    <div className="font-medium text-ink truncate max-w-[200px]">{r.name}</div>
                    <div className="text-[12px] text-muted">{r.matched_keyword}</div>
                  </div>
                </div>) },
            { key: "kept", label: "Đạt chuẩn",
              render: (r) => <Badge tone={r.kept ? "green" : "slate"}>
                {r.kept ? "Rating ≥ 4.0" : "Dưới ngưỡng"}</Badge> },
            { key: "rating", label: "Rating", align: "right",
              render: (r) => <span className="tabular-nums">{r.rating}★</span> },
            { key: "reviews_count", label: "Đánh giá", align: "right",
              render: (r) => <span className="tabular-nums">{nf(r.reviews_count)}</span> },
            { key: "sales_volume", label: "Lượt mua", align: "right",
              render: (r) => <span className="tabular-nums">{compact(r.sales_volume)}</span> },
            { key: "price", label: "Giá", align: "right",
              render: (r) => <span className="tabular-nums whitespace-nowrap">
                {r.price} {r.currency}</span> },
            { key: "platform", label: "Sàn",
              render: (r) => <ShopLogo platform={r.platform} /> },
            { key: "cluster_id", label: "Cụm SKU", align: "right",
              render: (r) => <span className="tabular-nums text-muted">{r.cluster_id || "—"}</span> },
            { key: "url", label: "Link", sortable: false,
              render: (r) => (r.url
                ? <a href={r.url} target="_blank" rel="noreferrer"
                    className="text-primary hover:underline text-[13px]">Mở</a>
                : <span className="text-disabled">—</span>) },
          ]} />
      </Card>
    );
  }

  /* Chi tiết 1 SKU: tách riêng danh sách Ads và danh sách sản phẩm trên sàn */
  function ProductDrawer({ product: p, onClose }) {
    const { Drawer, Badge, Spinner } = U();
    const [members, setMembers] = useState(null);
    useEffect(() => { api(`/api/niche/products/${p.id}/members`).then(setMembers); }, [p.id]);
    const m = MATCH_STATUS[p.match_status] || ["slate", p.match_status];

    return (
      <Drawer title={p.name}
        subtitle={`Hạng #${p.rank} · S2 ${p.s2}/10 (ads ${p.s21 ?? "N/A"} · sàn ${p.s22 ?? "N/A"})`}
        onClose={onClose} width="max-w-3xl">
        <div className="space-y-6">
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={m[0]}>{m[1]}</Badge>
            {p.matched_keyword && <Badge tone="slate">{p.matched_keyword}</Badge>}
            {!!p.match_bonus && <Badge tone="green">+{p.match_bonus} bonus khớp 2 nguồn</Badge>}
          </div>

          {!members ? <Spinner /> : (
            <>
              {/* ---- Nguồn 1: danh sách quảng cáo ---- */}
              <div>
                <div className={`flex items-center justify-between px-3 py-2 rounded-t-[10px] ${ADS_HEAD}`}>
                  <h5 className="text-[13px] font-semibold text-ink">
                    Nguồn 1 — Quảng cáo ({members.ads.length})
                  </h5>
                  <span className="text-[12px] text-muted">
                    max {p.ads_active_days ?? "N/A"} ngày · tổng reach {compact(p.ad_reach)}
                  </span>
                </div>
                {!members.ads.length ? (
                  <p className="text-[13px] text-warning border border-line border-t-0 rounded-b-[10px] px-3 py-3">
                    {NO_ADS}
                  </p>
                ) : (
                  <div className="border border-line border-t-0 rounded-b-[10px] divide-y divide-line">
                    {members.ads.map((a) => (
                      <div key={a.id} className="p-3">
                        <div className="flex items-start justify-between gap-3">
                          <div className="min-w-0">
                            <div className="text-sm font-medium text-ink truncate">{a.product_guess}</div>
                            <div className="text-[12px] text-muted truncate">
                              {a.page_name} · {a.platform} · {a.matched_keyword}
                            </div>
                          </div>
                          <div className="text-right shrink-0 text-[13px]">
                            <div className="tabular-nums text-ink">{a.active_days} ngày</div>
                            <div className="tabular-nums text-muted">reach {compact(a.reach)}</div>
                          </div>
                        </div>
                        {a.ad_copy && <p className="text-[12px] text-muted mt-1.5 line-clamp-2">{a.ad_copy}</p>}
                        {a.ad_link && (
                          <a href={a.ad_link} target="_blank" rel="noreferrer"
                            className="text-primary hover:underline text-[12px]">Xem ads →</a>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* ---- Tệp đối tượng thực tế mà quảng cáo đang tiếp cận ---- */}
              {!!(p.aud_breakdown && (p.aud_breakdown.ages || []).length) && (
                <div>
                  <div className={`flex items-center justify-between px-3 py-2 rounded-t-[10px] ${AUD_HEAD}`}>
                    <h5 className="text-[13px] font-semibold text-ink">
                      Tệp đối tượng thực tế của quảng cáo
                    </h5>
                    <Badge tone={(AUD_MATCH[p.aud_match || "unknown"])[0]}>
                      {(AUD_MATCH[p.aud_match || "unknown"])[1]}
                    </Badge>
                  </div>
                  <div className="border border-line border-t-0 rounded-b-[10px] p-3 space-y-2">
                    {(p.aud_breakdown.ages || []).map((a) => (
                      <div key={a.label} className="flex items-center gap-3">
                        <span className="w-14 text-[12px] text-muted shrink-0">{a.label}</span>
                        <div className="flex-1 h-2.5 rounded-full bg-hover overflow-hidden">
                          <div className="h-full rounded-full bg-violet-500"
                            style={{ width: `${Math.min(100, a.share * 200)}%` }} />
                        </div>
                        <span className="w-12 text-right text-[12px] tabular-nums text-ink">
                          {pct(a.share)}
                        </span>
                      </div>
                    ))}
                    <div className="flex flex-wrap gap-3 pt-1 text-[12px] text-muted">
                      {(p.aud_breakdown.genders || []).map((g) => (
                        <span key={g.label}>{GENDER_VI[g.label] || g.label}: {pct(g.share)}</span>
                      ))}
                      {!!(p.ad_countries || []).length && (
                        <span>Thị trường: {p.ad_countries.map((c) => flagOf(c) + " " + c).join(" · ")}</span>
                      )}
                    </div>
                    {p.aud_match_note && (
                      <p className="text-[12px] text-ink pt-1">{p.aud_match_note}</p>
                    )}
                  </div>
                </div>
              )}

              {/* ---- Nguồn 2: danh sách sản phẩm trên sàn ---- */}
              <div>
                <div className={`flex items-center justify-between px-3 py-2 rounded-t-[10px] ${ECOM_HEAD}`}>
                  <h5 className="text-[13px] font-semibold text-ink">
                    Nguồn 2 — Sản phẩm trên sàn ({members.ecom.length})
                  </h5>
                  <span className="text-[12px] text-muted">
                    {p.rating ? `${p.rating}★ · ${nf(p.sales_volume)} lượt mua` : "N/A"}
                  </span>
                </div>
                {!members.ecom.length ? (
                  <p className="text-[13px] text-warning border border-line border-t-0 rounded-b-[10px] px-3 py-3">
                    {NO_ECOM}
                  </p>
                ) : (
                  <div className="border border-line border-t-0 rounded-b-[10px] divide-y divide-line">
                    {members.ecom.map((e) => (
                      <div key={e.id} className="p-3 flex items-start gap-3">
                        <Thumb src={e.image_url} className="w-12 h-12 shrink-0" rounded="rounded-lg" />
                        <div className="min-w-0 flex-1">
                          <div className="text-sm font-medium text-ink truncate">{e.name}</div>
                          <div className="text-[12px] text-muted flex items-center gap-2 flex-wrap">
                            <ShopLogo platform={e.platform} />
                            <span>{e.rating}★ · {nf(e.reviews_count)} đánh giá ·
                              {" "}{compact(e.sales_volume)} lượt mua · {e.price} {e.currency}</span>
                          </div>
                          {e.url && (
                            <a href={e.url} target="_blank" rel="noreferrer"
                              className="text-primary hover:underline text-[12px]">Xem sản phẩm →</a>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}

          {/* ---- Tổng hợp review ---- */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-3 rounded-[10px] bg-green-50 border border-green-100">
              <h5 className="text-[13px] font-semibold text-success mb-1.5">Điểm mạnh (khách khen)</h5>
              {(p.pros || []).length ? (
                <ul className="space-y-1 text-[13px] text-ink list-disc pl-4">
                  {p.pros.map((x, i) => <li key={i}>{x}</li>)}
                </ul>
              ) : <p className="text-[13px] text-muted">Chưa có review để phân tích.</p>}
            </div>
            <div className="p-3 rounded-[10px] bg-amber-50 border border-amber-100">
              <h5 className="text-[13px] font-semibold text-amber-700 mb-1.5">Điểm yếu (khách chê)</h5>
              {(p.cons || []).length ? (
                <ul className="space-y-1 text-[13px] text-ink list-disc pl-4">
                  {p.cons.map((x, i) => <li key={i}>{x}</li>)}
                </ul>
              ) : <p className="text-[13px] text-muted">Chưa có review để phân tích.</p>}
            </div>
          </div>
          {p.pain_points && (
            <p className="text-[13px] text-ink">
              <span className="font-medium">Nỗi đau lớn nhất: </span>{p.pain_points}
            </p>
          )}
          {p.ad_reason && <p className="text-[12px] text-muted">AI ghép nối: {p.ad_reason}</p>}
        </div>
      </Drawer>
    );
  }

  function ProductMatrix({ session, meta, onChanged }) {
    const { Card, Button, Badge, Empty, Spinner, DataTable } = U();
    const toast = U().useToast();
    const [data, setData] = useState(null);
    const [showFiltered, setShowFiltered] = useState(false);
    const [detail, setDetail] = useState(null);
    const [tab, setTab] = useState("matrix");
    const [sources, setSources] = useState(null);
    const running = session.step2_status === "running";

    const load = useCallback(() => {
      api(`/api/niche/sessions/${session.id}/products?include_filtered=${showFiltered}`)
        .then((r) => setData(r.products));
    }, [session.id, showFiltered]);
    useEffect(load, [load]);
    useEffect(() => { if (!running) load(); }, [running]);
    useEffect(() => {
      if (tab === "matrix" || sources) return;
      api(`/api/niche/sessions/${session.id}/sources`).then(setSources);
    }, [tab, session.id]);
    useEffect(() => { if (!running) setSources(null); }, [running]);

    const scan = async () => {
      try {
        await api(`/api/niche/sessions/${session.id}/step2/run`, {
          method: "POST", body: { lang: LANG() } });
        onChanged();
        toast("Đang scan quảng cáo & sàn TMĐT");
      } catch (e) { toast("Lỗi: " + e.message); }
    };

    const stages = meta.step2_stages || [];
    const stepSession = { progress: session.step2_progress, stage: session.step2_stage,
                          status: session.step2_status, message: session.step2_message };

    if (!session.step2_status || (session.step2_status === "draft" && !running)) {
      return (
        <div className="rounded-xl border border-dashed border-gray-300 bg-bg px-6 py-10 text-center">
          <p className="text-sm font-medium text-ink">Chưa quét sản phẩm tiềm năng cho ngách này</p>
          <p className="text-[13px] text-muted mt-1 mb-4">
            Dùng bộ từ khoá bản địa của Bước 1 để spy quảng cáo và quét sàn TMĐT tại{" "}
            <Markets session={session} max={4} />.
          </p>
          <div className="inline-flex flex-col items-start gap-1 text-left text-[12px] text-muted mb-4">
            {stages.map((st) => <span key={st.key}>{st.label}</span>)}
          </div>
          <div><Button variant="primary" icon="search" onClick={scan}>Scan &amp; Spy</Button></div>
        </div>
      );
    }

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted">
            Chạy lúc: <span className="text-ink font-medium">{dt(session.last_step2_at)}</span>
            {session.ads_source && <span> · ads {session.ads_source}</span>}
            {session.ecom_source && <span> · sàn {session.ecom_source}</span>}
          </div>
          <div className="flex gap-2">
            <button onClick={() => setShowFiltered(!showFiltered)}
              className={`px-3 h-9 rounded-[10px] text-[13px] border ${
                showFiltered ? "border-warning bg-amber-50 text-amber-700"
                  : "border-gray-300 text-muted hover:bg-hover"}`}>
              {showFiltered ? "Đang hiện cả ads bị lọc" : "Hiện cả ads bị lọc"}
            </button>
            <Button variant="primary" icon="refresh" disabled={running} onClick={scan}>
              {running ? "Đang scan…" : "Scan lại"}
            </Button>
          </div>
        </div>

        {running && <StepProgress session={stepSession} steps={stages} />}

        <div className="flex gap-1 border-b border-line">
          {[["matrix", `Bảng hợp nhất${data ? ` (${data.length})` : ""}`],
            ["ads", "Nguồn 1: Ads Spy"], ["ecom", "Nguồn 2: Sàn TMĐT"]].map(([k, label]) => (
            <button key={k} onClick={() => setTab(k)}
              className={`px-4 py-2 text-[13px] font-medium border-b-2 -mb-px transition-colors ${
                tab === k ? "border-primary text-primary" : "border-transparent text-muted hover:text-ink"}`}>
              {label}
            </button>
          ))}
        </div>

        {tab === "ads" && <SourceAdsTable rows={(sources && sources.ads) || null} />}
        {tab === "ecom" && <SourceEcomTable rows={(sources && sources.ecom) || null} />}

        {tab === "matrix" && (!data ? <Spinner /> : !data.length ? (
          <Empty icon="package" title="Chưa có ứng viên nào"
            hint="Thử scan lại hoặc nới bộ từ khoá ở Bước 1." />
        ) : (
          <Card title={`Bảng xếp hạng sản phẩm tiềm năng (${data.length})`}>
            {/* Mỗi chỉ số 1 cột riêng — bấm tiêu đề cột bất kỳ để sort */}
            <DataTable
              rows={data}
              pageSize={10}
              searchKeys={["name", "matched_keyword"]}
              filters={[
                ...(showFiltered ? [{ key: "ad_status", label: "Trạng thái",
                                      options: ["valid", "irrelevant", "policy_risk"] }] : []),
                { key: "ecom_platform", label: "Sàn",
                  options: [...new Set(data.map((x) => x.ecom_platform).filter(Boolean))] },
              ]}
              onRowClick={(r) => setDetail(r)}
              columns={[
                { key: "rank", label: "#", align: "right",
                  cellClass: STICK_RANK, headClass: STICK_RANK_HEAD,
                  render: (r) => (
                    <span className={`inline-flex w-7 h-7 rounded-lg items-center justify-center text-[12px] font-bold ${
                      r.rank <= 3 ? "bg-primary text-white" : "bg-hover text-muted"}`}>{r.rank}</span>
                  ) },
                { key: "name", label: "SKU / Cụm sản phẩm",
                  cellClass: STICK_NAME, headClass: STICK_NAME_HEAD,
                  render: (r) => (
                    <div className="flex items-center gap-2.5 min-w-[220px]">
                      <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                      <div className="min-w-0">
                        <div className="font-medium text-ink truncate max-w-[210px]">{r.name}</div>
                        {r.ad_status !== "valid" && (
                          <Badge tone={(AD_STATUS[r.ad_status] || ["slate"])[0]}>
                            {(AD_STATUS[r.ad_status] || [0, r.ad_status])[1]}
                          </Badge>
                        )}
                      </div>
                    </div>
                  ) },
                { key: "s2", label: "Điểm S2", align: "right",
                  render: (r) => (
                    <span className={`text-lg font-bold tabular-nums ${
                      r.s2 >= 8 ? "text-success" : r.s2 >= 5 ? "text-warning" : "text-error"}`}>{r.s2}</span>
                  ) },
                { key: "match_status", label: "Ghép nối",
                  render: (r) => {
                    const m = MATCH_STATUS[r.match_status] || ["slate", r.match_status];
                    return <Badge tone={m[0]}>{m[1]}</Badge>;
                  } },
                { key: "matched_keyword", label: "Từ khoá",
                  render: (r) => <span className="text-[13px] text-muted whitespace-nowrap">{r.matched_keyword || "—"}</span> },

                /* ---- Nguồn 1: Spy Ads (nền xanh dương) ---- */
                { key: "ads_active_days", label: "Ngày active (max)", align: "right",
                  cellClass: `${ADS_BG} ${GROUP_EDGE_ADS}`, headClass: `${ADS_HEAD} ${GROUP_EDGE_ADS}`,
                  render: (r) => (r.ads_active_days != null
                    ? <span className="tabular-nums">{r.ads_active_days}</span>
                    : <span className="text-[12px] text-gray-400 whitespace-nowrap" title={NO_ADS}>N/A</span>) },
                { key: "ad_reach", label: "Reach (tổng)", align: "right",
                  cellClass: ADS_BG, headClass: ADS_HEAD,
                  render: (r) => (r.ad_reach
                    ? <span className="tabular-nums">{compact(r.ad_reach)}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "ads_count", label: "Số ads", align: "right",
                  cellClass: ADS_BG, headClass: ADS_HEAD,
                  render: (r) => <span className="tabular-nums">{r.ads_count ?? "—"}</span> },

                /* ---- Nguồn 2: Sàn TMĐT (nền xanh lá) ---- */
                { key: "ecom_platform", label: "Sàn",
                  cellClass: `${ECOM_BG} ${GROUP_EDGE_ECOM}`, headClass: `${ECOM_HEAD} ${GROUP_EDGE_ECOM}`,
                  render: (r) => <ShopLogo platform={r.ecom_platform} /> },
                { key: "revenue", cellClass: ECOM_BG, headClass: ECOM_HEAD, label: "Doanh thu", align: "right",
                  render: (r) => (r.revenue
                    ? <span className="tabular-nums whitespace-nowrap">{compact(r.revenue)} {r.currency || ""}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "sales_volume", label: "Lượt mua (tổng)", align: "right",
                  cellClass: ECOM_BG, headClass: ECOM_HEAD,
                  render: (r) => (r.sales_volume
                    ? <span className="tabular-nums">{compact(r.sales_volume)}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "shops_count", cellClass: ECOM_BG, headClass: ECOM_HEAD, label: "Số shop", align: "right",
                  render: (r) => <span className="tabular-nums">{r.shops_count ?? "—"}</span> },
                { key: "rating", label: "Rating TB", align: "right",
                  cellClass: ECOM_BG, headClass: ECOM_HEAD,
                  render: (r) => (r.rating
                    ? <span className={`tabular-nums font-medium ${r.rating >= 4.5 ? "text-success"
                        : r.rating >= 4 ? "text-ink" : "text-error"}`}>{r.rating}★</span>
                    : <span className="text-[12px] text-gray-400" title={NO_ECOM}>N/A</span>) },
                { key: "reviews_count", label: "Lượt đánh giá", align: "right",
                  cellClass: ECOM_BG, headClass: ECOM_HEAD,
                  render: (r) => (r.reviews_count
                    ? <span className="tabular-nums">{nf(r.reviews_count)}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "price", label: "Giá TB", align: "right",
                  cellClass: ECOM_BG, headClass: ECOM_HEAD,
                  render: (r) => (r.price
                    ? <span className="tabular-nums whitespace-nowrap">{r.price} {r.currency || ""}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },

                /* ---- Tệp đối tượng THỰC TẾ mà ads đang tiếp cận ---- */
                { key: "aud_age_top", label: "Nhóm tuổi ads", align: "right",
                  cellClass: `${AUD_BG} border-l-2 border-violet-400`, headClass: AUD_HEAD,
                  render: (r) => (r.aud_age_top
                    ? <span className="whitespace-nowrap">
                        <b className="text-ink">{r.aud_age_top}</b>
                        <span className="text-[12px] text-muted"> · {pct(r.aud_age_share)}</span>
                      </span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "aud_gender_top", label: "Giới tính ads", align: "right",
                  cellClass: AUD_BG, headClass: "!text-violet-900 bg-[#DDD6FE]",
                  render: (r) => (r.aud_gender_top
                    ? <span className="whitespace-nowrap">
                        {GENDER_VI[r.aud_gender_top] || r.aud_gender_top}
                        <span className="text-[12px] text-muted"> · {pct(r.aud_gender_share)}</span>
                      </span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "aud_match", label: "Khớp tệp Bước 1",
                  cellClass: AUD_BG, headClass: "!text-violet-900 bg-[#DDD6FE]",
                  render: (r) => {
                    const m = AUD_MATCH[r.aud_match || "unknown"];
                    return <span title={r.aud_match_note || ""}><Badge tone={m[0]}>{m[1]}</Badge></span>;
                  } },

                { key: "s21", label: "S2.1 ads", align: "right",
                  cellClass: `${ADS_BG} ${GROUP_EDGE_ADS}`, headClass: `${ADS_HEAD} ${GROUP_EDGE_ADS}`,
                  render: (r) => <span className="tabular-nums text-muted">{r.s21 ?? "N/A"}</span> },
                { key: "s22", label: "S2.2 sàn", align: "right",
                  cellClass: `${ECOM_BG} ${GROUP_EDGE_ECOM}`, headClass: `${ECOM_HEAD} ${GROUP_EDGE_ECOM}`,
                  render: (r) => <span className="tabular-nums text-muted">{r.s22 ?? "N/A"}</span> },
              ]} />
            <div className="flex flex-wrap items-center gap-4 mt-3 text-[13px] text-muted">
              <span className="inline-flex items-center gap-1.5">
                <span className={`w-3.5 h-3.5 rounded ${ADS_HEAD} border border-blue-400`} />Số liệu từ Spy Ads
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className={`w-3.5 h-3.5 rounded ${ECOM_HEAD} border border-orange-400`} />Số liệu từ sàn TMĐT
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="w-3.5 h-3.5 rounded bg-[#DDD6FE] border border-violet-400" />
                Tệp đối tượng thực tế của ads
              </span>
              <span>Bấm tiêu đề cột bất kỳ để sắp xếp · bấm vào dòng để xem chi tiết SKU.</span>
            </div>
          </Card>
        ))}

        {detail && <ProductDrawer product={detail} onClose={() => setDetail(null)} />}
      </div>
    );
  }

  /* =================== VÙNG 3 — Đối thủ cạnh tranh & định giá =================== */
  const usd = (v) => (v == null ? "—" : `$${Number(v).toFixed(2)}`);
  const PRICE_SRC = { ads_landing: ["blue", "Landing page ads"], ecom: ["green", "Trang sàn TMĐT"] };

  /* Trục giá trực quan: P_min ─ P_avg ─ P_target ─ P_max */
  function PriceAxis({ p }) {
    if (!p.p_min || !p.p_max) return null;
    const span = Math.max(0.01, p.p_max - p.p_min);
    const at = (v) => `${Math.min(100, Math.max(0, ((v - p.p_min) / span) * 100))}%`;
    // chừa lề 2 bên để nhãn ở đầu/cuối dải không bị cắt
    return (
      <div className="pt-11 pb-3 mx-12">
        <div className="relative">
          <div className="h-2 rounded-full bg-gradient-to-r from-sky-200 via-emerald-200 to-amber-200" />
          {/* vùng an toàn P_avg → P_max: nơi giá bán mục tiêu được phép nằm */}
          <div className="absolute h-2 rounded-full bg-emerald-400/50 top-0"
            style={{ left: at(p.p_avg), right: `calc(100% - ${at(p.p_max)})` }} />
          {[["p_min", "Thấp nhất", "#0EA5E9"], ["p_avg", "Trung bình", "#6B7280"],
            ["p_target", "Giá mục tiêu", "#16A34A"], ["p_max", "Cao nhất", "#F59E0B"]].map(
            ([k, label, color]) => (p[k] == null ? null : (
              <div key={k} className="absolute bottom-1 -translate-x-1/2 text-center"
                style={{ left: at(p[k]) }}>
                <div className="text-[11px] text-muted whitespace-nowrap">{label}</div>
                <div className="text-[12px] font-bold tabular-nums whitespace-nowrap"
                  style={{ color }}>{usd(p[k])}</div>
                <div className="w-0.5 h-3 mx-auto" style={{ background: color }} />
              </div>
            )))}
        </div>
      </div>
    );
  }

  /* Chọn SKU vào shortlist + cấu hình ngưỡng biên lợi nhuận rồi chạy Bước 3 */
  function ShortlistDrawer({ session, meta, onClose, onDone }) {
    const { Drawer, Button, Badge, Spinner, Empty, Field, Input } = U();
    const toast = U().useToast();
    const [rows, setRows] = useState(null);
    const [sel, setSel] = useState(new Set());
    const cfgP = meta.pricing || {};
    const cogsPct = Math.round((session.cogs_share ?? cfgP.cogs_share ?? 0.4) * 100);
    const adsPct = Math.round((session.ads_share ?? cfgP.ads_share ?? 0.3) * 100);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
      api(`/api/niche/sessions/${session.id}/products`).then((r) => {
        setRows(r.products);
        setSel(new Set(r.products.filter((x) => x.shortlisted).map((x) => x.id)));
      });
    }, [session.id]);

    const toggle = (id) => setSel((s0) => {
      const n = new Set(s0); n.has(id) ? n.delete(id) : n.add(id); return n;
    });

    const run = async () => {
      if (!sel.size) return toast("Hãy chọn ít nhất 1 SKU");
      setBusy(true);
      try {
        await api(`/api/niche/sessions/${session.id}/shortlist`,
          { method: "POST", body: { product_ids: [...sel] } });
        await api(`/api/niche/sessions/${session.id}/step3/run`,
          { method: "POST", body: { lang: LANG() } });
        toast(`Đang phân tích giá ${sel.size} SKU`);
        onDone();
      } catch (e) { toast("Lỗi: " + e.message); } finally { setBusy(false); }
    };

    return (
      <Drawer title="Chọn sản phẩm phân tích giá"
        subtitle="Pick SKU tiềm năng từ bảng xếp hạng Bước 2 rồi đặt ngưỡng biên lợi nhuận"
        onClose={onClose} width="max-w-2xl"
        footer={
          <div className="flex items-center justify-between gap-3">
            <span className="text-[13px] text-muted">Đã chọn {sel.size} SKU</span>
            <div className="flex gap-2">
              <Button onClick={onClose}>Đóng</Button>
              <Button variant="primary" icon="search" disabled={busy} onClick={run}>
                {busy ? "Đang gửi…" : "Phân tích giá & benchmark"}
              </Button>
            </div>
          </div>}>
        <div className="space-y-5">
          <div className="p-3 rounded-[10px] bg-bg border border-line text-[13px] text-muted">
            Đang dùng cấu trúc tài chính của phiên: giá vốn <b className="text-ink">{cogsPct}%</b> ·
            quảng cáo <b className="text-ink">{adsPct}%</b> · biên còn lại{" "}
            <b className="text-ink">{100 - cogsPct - adsPct}%</b> doanh thu.
            Sửa tỷ lệ ở khối “Cấu trúc tài chính trên mỗi đơn hàng” phía trên.
          </div>

          {!rows ? <Spinner /> : !rows.length ? (
            <Empty icon="package" title="Chưa có SKU nào ở Bước 2"
              hint="Chạy Scan & Spy ở vùng 2 trước." />
          ) : (
            <div className="space-y-1.5">
              {rows.map((r) => {
                const on = sel.has(r.id);
                const links = (r.ads_count || 0) + (r.shops_count || 0);
                return (
                  <button key={r.id} onClick={() => toggle(r.id)}
                    className={`w-full text-left px-3 py-2.5 rounded-[10px] border flex items-center gap-3 ${
                      on ? "border-primary bg-softblue" : "border-line hover:bg-hover"}`}>
                    <span className={`w-4 h-4 shrink-0 rounded border flex items-center justify-center text-[10px] ${
                      on ? "bg-primary border-primary text-white" : "border-gray-300 text-transparent"}`}>✓</span>
                    <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                    <span className="min-w-0 flex-1">
                      <span className="text-sm font-medium text-ink block truncate">{r.name}</span>
                      <span className="text-[12px] text-muted">
                        S2 {r.s2}/10 · {links} link đối thủ
                        {r.price ? ` · ${r.price} ${r.currency || ""}` : ""}
                      </span>
                    </span>
                    <Badge tone={(MATCH_STATUS[r.match_status] || ["slate"])[0]}>
                      {(MATCH_STATUS[r.match_status] || [0, "—"])[1]}
                    </Badge>
                  </button>
                );
              })}
            </div>
          )}
        </div>
      </Drawer>
    );
  }

  /* Chi tiết 1 SKU: dải giá, trần tài chính, benchmark ads và từng điểm giá thu được */
  function PricingDrawer({ product: p, session, meta, onClose }) {
    const { Drawer, Badge } = U();
    const cfg = meta.pricing || {};
    const cogsPct = Math.round((session.cogs_share ?? cfg.cogs_share ?? 0.4) * 100);
    const adsPct = Math.round((session.ads_share ?? cfg.ads_share ?? 0.3) * 100);
    const feasible = p.ads_feasible;

    const cards = [
      ["Giá bán mục tiêu (P_target)", usd(p.p_target), "#16A34A",
       `Nằm trong khoảng an toàn ${usd(p.p_avg)} – ${usd(p.p_max)}`],
      ["Trần giá vốn cho phép (COGS_max)", usd(p.cogs_max), "#2563EB",
       `= P_target × ${cogsPct}% doanh thu · gồm giá nhập + ship về kho`],
      ["CPA trần cho phép", usd(p.cpa_target_max), "#7C3AED",
       `= P_target × ${adsPct}% doanh thu dành cho ads`],
    ];

    return (
      <Drawer title={p.name}
        subtitle={`S3 ${p.s3 ?? "—"}/10 · ${p.price_points || 0} điểm giá đối thủ · giá vốn ${cogsPct}% · ads ${adsPct}%`}
        onClose={onClose} width="max-w-3xl">
        <div className="space-y-6">
          <PriceAxis p={p} />

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {cards.map(([label, value, color, note]) => (
              <div key={label} className="p-3 rounded-xl border border-line bg-bg">
                <div className="text-[12px] text-muted leading-snug">{label}</div>
                <div className="text-[22px] font-bold tabular-nums mt-0.5" style={{ color }}>{value}</div>
                <div className="text-[11px] text-muted mt-1 leading-snug">{note}</div>
              </div>
            ))}
          </div>

          {p.pricing_note && (
            <div className="p-4 rounded-xl bg-softblue border border-primary/30">
              <h5 className="text-[13px] font-semibold text-ink mb-1">
                Lập luận định giá
                <Badge tone="slate">{p.pricing_source === "claude" ? "Claude AI" : "Rule engine"}</Badge>
              </h5>
              <p className="text-[13px] text-ink">{p.pricing_note}</p>
            </div>
          )}

          {/* Benchmark quảng cáo & khả thi tài chính */}
          <div>
            <h5 className="text-[13px] font-semibold text-ink mb-2">
              Benchmark quảng cáo Meta — <Markets session={session} max={3} />
            </h5>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              {[["CPM", usd(p.cpm_benchmark)], ["CTR", p.ctr_benchmark ? pct(p.ctr_benchmark) : "—"],
                ["CPC", usd(p.cpc_benchmark)],
                ["CPA ước tính", usd(p.cpa_estimated)]].map(([k, v]) => (
                <div key={k} className="p-3 rounded-[10px] border border-line">
                  <div className="text-[12px] text-muted">{k}</div>
                  <div className="text-[16px] font-semibold text-ink tabular-nums">{v}</div>
                </div>
              ))}
            </div>
            <p className={`text-[12px] mt-2 ${feasible === false ? "text-error" : "text-muted"}`}>
              {feasible === false
                ? `CPA ước tính ${usd(p.cpa_estimated)} đang vượt CPA trần ${usd(p.cpa_target_max)} — cần tăng giá bán, tăng AOV bằng combo/upsell hoặc tối ưu CVR.`
                : feasible === true
                  ? `CPA ước tính ${usd(p.cpa_estimated)} nằm dưới CPA trần ${usd(p.cpa_target_max)} — dư địa tài chính an toàn.`
                  : "Chưa đủ dữ liệu benchmark để ước tính CPA."}
            </p>
            <p className="text-[11px] text-disabled mt-1">
              CPA ước tính = CPC benchmark ÷ CVR giả định {pct(cfg.assumed_cvr)}. Đây là ước lượng
              để sàng lọc, không thay cho số liệu chạy thật.
            </p>
          </div>

          {/* Điểm thành phần S3 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            {[["S3.1 — Dư địa giá bán", p.s31, `P_target ${usd(p.p_target)} · ≥ $35–40 là dư địa tốt cho COD`],
              ["S3.2 — Dung lượng trần giá vốn", p.s32, `COGS_max ${usd(p.cogs_max)} · ≥ $12–15 thì dễ sourcing`]]
              .map(([label, v, note]) => (
              <div key={label} className="p-3 rounded-xl border border-line">
                <div className="flex items-center justify-between">
                  <span className="text-[13px] font-medium text-ink">{label}</span>
                  <span className="text-lg font-bold tabular-nums" style={{
                    color: (v || 0) >= 8 ? "#16A34A" : (v || 0) >= 5 ? "#F59E0B" : "#DC2626" }}>
                    {v ?? "—"}</span>
                </div>
                <div className="h-1.5 rounded-full bg-hover overflow-hidden mt-2">
                  <div className="h-full rounded-full" style={{ width: `${(v || 0) * 10}%`,
                    background: (v || 0) >= 8 ? "#16A34A" : (v || 0) >= 5 ? "#F59E0B" : "#DC2626" }} />
                </div>
                <p className="text-[11px] text-muted mt-1.5">{note}</p>
              </div>
            ))}
          </div>

          {/* Từng điểm giá đối thủ */}
          <div>
            <h5 className="text-[13px] font-semibold text-ink mb-2">
              Điểm giá đối thủ thu thập được ({(p.prices || []).length})
            </h5>
            {!(p.prices || []).length ? (
              <p className="text-[13px] text-warning border border-line rounded-[10px] px-3 py-3">
                Không truy cập được link đối thủ nào của SKU này.
              </p>
            ) : (
              <div className="border border-line rounded-[10px] divide-y divide-line">
                {p.prices.map((pr) => {
                  const src = PRICE_SRC[pr.source] || ["slate", pr.source];
                  return (
                    <div key={pr.id} className={`p-3 flex items-center gap-3 ${
                      pr.source === "ads_landing" ? ADS_BG : ECOM_BG}`}>
                      <Badge tone={src[0]}>{src[1]}</Badge>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-medium text-ink truncate">
                          {pr.shop_name || pr.platform || "—"}
                        </div>
                        <div className="text-[12px] text-muted truncate">{pr.raw_text}</div>
                      </div>
                      <div className="text-right shrink-0">
                        <div className="text-sm font-semibold text-ink tabular-nums">{usd(pr.price_usd)}</div>
                        <div className="text-[11px] text-muted tabular-nums">
                          {pr.price_local} {pr.currency}
                        </div>
                      </div>
                      <Badge tone={pr.extracted_by === "demo" ? "amber"
                        : pr.extracted_by === "claude" ? "blue" : "slate"}>
                        {pr.extracted_by === "demo" ? "mô phỏng"
                          : pr.extracted_by === "claude" ? "AI bóc" : "regex"}
                      </Badge>
                      {pr.url && (
                        <a href={pr.url} target="_blank" rel="noreferrer"
                          className="text-primary hover:underline text-[12px] shrink-0">Mở →</a>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </Drawer>
    );
  }

  /* Bản đồ giá: mỗi SKU một thanh Pmin→Pmax + điểm giá mục tiêu, chồng lên các
     đường trần chi phí (giá vốn cho phép, CPA trần, CPA ước tính) — nhìn là biết
     SKU nào còn dư địa, SKU nào chi phí ads đã ăn hết biên. */
  function PriceChart({ rows, meta }) {
    const { Card, ChartView, Badge } = U();
    const cfg = meta.pricing || {};
    const data = (rows || []).filter((r) => r.p_min != null && r.p_max != null);
    const skipped = (rows || []).length - data.length;
    if (!data.length) return null;

    const labels = data.map((r) => (r.name.length > 28 ? r.name.slice(0, 27) + "…" : r.name));
    const line = (label, key, color, dash) => ({
      type: "line", label, borderColor: color, backgroundColor: color,
      borderWidth: 2, borderDash: dash || [], pointRadius: 3, pointHoverRadius: 5,
      data: data.map((r) => r[key] ?? null), spanGaps: true, order: 1,
    });

    return (
      <Card title="Bản đồ giá & trần chi phí (USD)" action={
        <Badge tone="slate">Bấm vào chú thích để ẩn/hiện từng đường</Badge>}>
        <ChartView type="bar" height={110 + data.length * 52}
          data={{
            labels,
            datasets: [
              { label: "Dải giá thị trường (thấp nhất → cao nhất)",
                data: data.map((r) => [r.p_min, r.p_max]),
                backgroundColor: "#BFDBFE", borderColor: "#3B82F6", borderWidth: 1,
                borderSkipped: false, borderRadius: 4, barPercentage: 0.55, order: 5 },
              { type: "line", label: "Giá bán mục tiêu", showLine: false,
                data: data.map((r) => r.p_target ?? null),
                borderColor: "#16A34A", backgroundColor: "#16A34A",
                pointStyle: "rectRot", pointRadius: 8, pointHoverRadius: 10, order: 0 },
              line("Trần giá vốn cho phép (COGS_max)", "cogs_max", "#F59E0B"),
              line("CPA trần cho phép", "cpa_target_max", "#7C3AED"),
              line("CPA ước tính từ benchmark ads", "cpa_estimated", "#DC2626", [6, 4]),
            ],
          }}
          options={{
            indexAxis: "y",
            interaction: { mode: "index", intersect: false },
            plugins: {
              legend: { position: "bottom",
                labels: { font: { family: "Inter", size: 11 }, color: "#6B7280",
                          boxWidth: 12, boxHeight: 12, padding: 14, usePointStyle: true,
                          /* giữ chú thích theo thứ tự khai báo, không theo thứ tự vẽ */
                          sort: (a, b) => a.datasetIndex - b.datasetIndex } },
              tooltip: { callbacks: { label: (c) => {
                const v = c.raw;
                if (Array.isArray(v)) return `${c.dataset.label}: $${v[0]} – $${v[1]}`;
                return v == null ? null : `${c.dataset.label}: $${Number(v).toFixed(2)}`;
              } } },
            },
            scales: {
              x: { beginAtZero: true, grid: { color: "#F1F3F7" }, border: { display: false },
                   ticks: { font: { family: "Inter", size: 11 }, color: "#9CA3AF",
                            callback: (v) => `$${v}` } },
              y: { grid: { display: false },
                   ticks: { font: { family: "Inter", size: 11 }, color: "#6B7280" } },
            },
          }} />
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-3 text-[12px] text-muted">
          <span>
            Thanh xanh là dải giá đối thủ đang bán; kim cương xanh lá là giá bán mục tiêu —
            luôn nằm trong nửa trên của dải.
          </span>
          <span>
            Đường đỏ (CPA ước tính) vượt qua đường tím (CPA trần) nghĩa là chi phí quảng cáo dự kiến
            đã ăn hết phần doanh thu dành cho ads · CVR giả định {pct(cfg.assumed_cvr)}.
          </span>
          {skipped > 0 && <span>{skipped} SKU chưa có điểm giá nên không vẽ được.</span>}
        </div>
      </Card>
    );
  }

  /* 3.0 — Cấu trúc tài chính: user đặt % doanh thu cho giá vốn và cho quảng cáo */
  function FinanceConfig({ session, meta, onSaved }) {
    const { Card, Button, Badge } = U();
    const toast = U().useToast();
    const cfg = meta.pricing || {};
    const [cogs, setCogs] = useState(Math.round((session.cogs_share ?? cfg.cogs_share ?? 0.4) * 100));
    const [ads, setAds] = useState(Math.round((session.ads_share ?? cfg.ads_share ?? 0.3) * 100));
    const [busy, setBusy] = useState(false);
    useEffect(() => {
      setCogs(Math.round((session.cogs_share ?? cfg.cogs_share ?? 0.4) * 100));
      setAds(Math.round((session.ads_share ?? cfg.ads_share ?? 0.3) * 100));
    }, [session.cogs_share, session.ads_share]);

    const margin = 100 - cogs - ads;
    const dirty = Math.round((session.cogs_share ?? 0.4) * 100) !== cogs
      || Math.round((session.ads_share ?? 0.3) * 100) !== ads;

    const save = async () => {
      if (margin <= 0) return toast("Giá vốn + quảng cáo phải nhỏ hơn 100% doanh thu");
      setBusy(true);
      try {
        await api(`/api/niche/sessions/${session.id}/finance`, { method: "POST",
          body: { cogs_share: cogs / 100, ads_share: ads / 100 } });
        toast("Đã lưu cấu trúc tài chính — chạy lại 3.1 để áp dụng vào trần giá vốn");
        onSaved();
      } catch (e) { toast("Lỗi: " + e.message); } finally { setBusy(false); }
    };

    const bars = [
      ["Giá vốn + ship về kho", cogs, "#F59E0B"],
      ["Quảng cáo (CPA trần)", ads, "#7C3AED"],
      ["Vận hành + lợi nhuận", Math.max(0, margin), "#16A34A"],
    ];

    return (
      <Card title="Cấu trúc tài chính trên mỗi đơn hàng" action={
        <Badge tone={margin >= 20 ? "green" : margin > 0 ? "amber" : "red"}>
          Biên còn lại {margin}%
        </Badge>}>
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div className="lg:col-span-2 space-y-4">
            {/* thanh phân bổ 100% doanh thu */}
            <div>
              <div className="flex h-8 rounded-lg overflow-hidden border border-line">
                {bars.map(([label, v, color]) => (v <= 0 ? null : (
                  <div key={label} style={{ width: `${v}%`, background: color }}
                    className="flex items-center justify-center text-white text-[12px] font-semibold">
                    {v >= 8 ? `${v}%` : ""}
                  </div>
                )))}
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-[12px] text-muted">
                {bars.map(([label, v, color]) => (
                  <span key={label} className="inline-flex items-center gap-1.5">
                    <span className="w-3 h-3 rounded" style={{ background: color }} />
                    {label} — {v}%
                  </span>
                ))}
              </div>
            </div>

            {[["Giá vốn (COGS)", cogs, setCogs, "#F59E0B",
               "Gồm giá nhập + ship về kho EU. Đây chính là trần đàm phán cho đội sourcing."],
              ["Chi phí quảng cáo (CPA)", ads, setAds, "#7C3AED",
               "Phần doanh thu chấp nhận chi cho ads mỗi đơn — thành CPA trần cho phép."]]
              .map(([label, val, setter, color, hint]) => (
              <div key={label}>
                <div className="flex items-center justify-between gap-3 mb-1">
                  <span className="text-[13px] font-medium text-gray-700">{label}</span>
                  <span className="text-[15px] font-bold tabular-nums" style={{ color }}>{val}%</span>
                </div>
                <input type="range" min="5" max="80" value={val} className="w-full accent-primary"
                  onChange={(e) => setter(Number(e.target.value))} />
                <p className="text-[12px] text-muted mt-1">{hint}</p>
              </div>
            ))}
          </div>

          <div className="space-y-3">
            <div className="p-4 rounded-xl bg-bg border border-line text-[13px] space-y-2">
              <div className="font-medium text-gray-700">Áp dụng cho mọi SKU trong shortlist:</div>
              <div className="flex justify-between"><span className="text-muted">Trần giá vốn</span>
                <span className="tabular-nums text-ink">P_target × {cogs}%</span></div>
              <div className="flex justify-between"><span className="text-muted">CPA trần</span>
                <span className="tabular-nums text-ink">P_target × {ads}%</span></div>
              <div className="flex justify-between"><span className="text-muted">Biên còn lại</span>
                <span className="tabular-nums font-semibold"
                  style={{ color: margin >= 20 ? "#16A34A" : "#DC2626" }}>{margin}%</span></div>
            </div>
            <Button variant="primary" icon="check" className="w-full" disabled={busy || !dirty}
              onClick={save}>
              {busy ? "Đang lưu…" : dirty ? "Lưu cấu trúc tài chính" : "Đã lưu"}
            </Button>
            {dirty && (
              <p className="text-[12px] text-warning">
                Đổi tỷ lệ xong cần chạy lại 3.1 để tính lại trần giá vốn và CPA trần.
              </p>
            )}
          </div>
        </div>
      </Card>
    );
  }

  function PricingSection({ session, meta, onChanged }) {
    const { Card, Button, Badge, Empty, Spinner, DataTable } = U();
    const [data, setData] = useState(null);
    const [picking, setPicking] = useState(false);
    const [detail, setDetail] = useState(null);
    const running = session.step3_status === "running";
    const cfg = meta.pricing || {};
    const cogsPct = Math.round((session.cogs_share ?? cfg.cogs_share ?? 0.4) * 100);
    const adsPct = Math.round((session.ads_share ?? cfg.ads_share ?? 0.3) * 100);

    const load = useCallback(() => {
      api(`/api/niche/sessions/${session.id}/pricing`).then((r) => setData(r.products));
    }, [session.id]);
    useEffect(load, [load]);
    useEffect(() => { if (!running) load(); }, [running]);

    const stages = meta.step3_stages || [];
    const stepSession = { progress: session.step3_progress, stage: session.step3_stage,
                          status: session.step3_status, message: session.step3_message };

    if (!session.step2_status || session.step2_status !== "done") {
      return <ComingSoon note="Cần hoàn tất Bước 2 để có danh sách SKU tiềm năng trước khi phân tích giá." />;
    }

    if (!session.step3_status || (session.step3_status === "draft" && !running)) {
      return (
        <>
          <div className="rounded-xl border border-dashed border-gray-300 bg-bg px-6 py-10 text-center">
            <p className="text-sm font-medium text-ink">Chưa phân tích giá đối thủ cho ngách này</p>
            <p className="text-[13px] text-muted mt-1 mb-4">
              Chọn SKU tiềm năng từ bảng xếp hạng Bước 2, hệ thống sẽ mở từng link ads &amp; link sàn
              để lấy giá bán thực tế, rồi tính giá bán mục tiêu và trần giá vốn cho đội sourcing.
            </p>
            <div className="inline-flex flex-col items-start gap-1 text-left text-[12px] text-muted mb-4">
              {stages.map((st) => <span key={st.key}>{st.label}</span>)}
            </div>
            <div>
              <Button variant="primary" icon="search" onClick={() => setPicking(true)}>
                Chọn SKU &amp; phân tích giá
              </Button>
            </div>
          </div>
          {picking && <ShortlistDrawer session={session} meta={meta}
            onClose={() => setPicking(false)} onDone={() => { setPicking(false); onChanged(); }} />}
        </>
      );
    }

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted">
            Chạy lúc: <span className="text-ink font-medium">{dt(session.last_step3_at)}</span>
            <span> · giá vốn {cogsPct}% · ads {adsPct}%</span>
            {session.price_source && <span> · giá {session.price_source === "scraper" ? "scrape thật" : "mô phỏng"}</span>}
            {session.benchmark_source && <span> · benchmark {session.benchmark_source === "meta_api" ? "Meta API" : "tham chiếu"}</span>}
          </div>
          <Button variant="primary" icon="refresh" disabled={running} onClick={() => setPicking(true)}>
            {running ? "Đang phân tích…" : "Chọn lại SKU & chạy lại"}
          </Button>
        </div>

        {(running || session.step3_status === "error") && (
          <StepProgress session={stepSession} steps={stages} prefix="3" />
        )}

        {/* chạy xong nhưng có link không lấy được giá -> nói thẳng, đừng để hiểu nhầm là số thật */}
        {!running && session.step3_status === "done" && session.step3_message && (
          <div className="p-3 rounded-xl bg-amber-50 border border-amber-200 text-[13px] text-amber-800">
            {session.step3_message}
          </div>
        )}

        {!data ? <Spinner /> : !data.length ? (
          <Empty icon="package" title="Chưa có SKU nào trong shortlist" />
        ) : (
          <>
          <PriceChart rows={data} meta={meta} />
          <div className="h-4" />
          <Card title={`Dải giá đối thủ & trần tài chính (${data.length} SKU)`}
            action={<Badge tone="slate">FX {cfg.fx_source === "api" ? "API" : "bảng tĩnh"} · quy về USD</Badge>}>
            <DataTable rows={data} pageSize={10} searchKeys={["name"]}
              onRowClick={(r) => setDetail(r)}
              columns={[
                { key: "name", label: "SKU / Cụm sản phẩm",
                  cellClass: STICK_NAME.replace("left-[68px]", "left-0"),
                  headClass: STICK_NAME_HEAD.replace("left-[68px]", "left-0"),
                  render: (r) => (
                    <div className="flex items-center gap-2.5 min-w-[210px]">
                      <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                      <div className="min-w-0">
                        <div className="font-medium text-ink truncate max-w-[200px]">{r.name}</div>
                        <div className="text-[12px] text-muted">S2 {r.s2}/10</div>
                      </div>
                    </div>
                  ) },
                { key: "s3", label: "Điểm S3", align: "right",
                  render: (r) => (
                    <span className={`text-lg font-bold tabular-nums ${
                      r.s3 >= 8 ? "text-success" : r.s3 >= 5 ? "text-warning" : "text-error"}`}>
                      {r.s3 ?? "—"}</span>
                  ) },
                { key: "price_points", label: "Số điểm giá", align: "right",
                  render: (r) => (r.price_points
                    ? <span className="tabular-nums">{r.price_points}</span>
                    : <span className="text-[12px] text-gray-400">N/A</span>) },
                { key: "p_min", label: "Giá thấp nhất", align: "right",
                  render: (r) => <span className="tabular-nums">{usd(r.p_min)}</span> },
                { key: "p_avg", label: "Giá trung bình", align: "right",
                  render: (r) => <span className="tabular-nums">{usd(r.p_avg)}</span> },
                { key: "p_max", label: "Giá cao nhất", align: "right",
                  render: (r) => <span className="tabular-nums">{usd(r.p_max)}</span> },
                { key: "p_target", label: "Giá bán mục tiêu", align: "right",
                  cellClass: "bg-[#ECFDF5]", headClass: "!text-emerald-900 bg-[#A7F3D0] border-l-2 border-emerald-400",
                  render: (r) => <span className="tabular-nums font-bold text-success">{usd(r.p_target)}</span> },
                { key: "cogs_max", label: "Trần giá vốn", align: "right",
                  cellClass: "bg-[#ECFDF5]", headClass: "!text-emerald-900 bg-[#A7F3D0]",
                  render: (r) => <span className="tabular-nums font-bold text-ink">{usd(r.cogs_max)}</span> },
                { key: "cpa_target_max", label: "CPA trần", align: "right",
                  render: (r) => <span className="tabular-nums">{usd(r.cpa_target_max)}</span> },
                { key: "cpa_estimated", label: "CPA ước tính", align: "right",
                  render: (r) => (
                    <span className={`tabular-nums ${r.ads_feasible === false ? "text-error font-medium" : ""}`}>
                      {usd(r.cpa_estimated)}</span>
                  ) },
                { key: "s31", label: "S3.1 giá bán", align: "right",
                  render: (r) => <span className="tabular-nums text-muted">{r.s31 ?? "—"}</span> },
                { key: "s32", label: "S3.2 giá vốn", align: "right",
                  render: (r) => <span className="tabular-nums text-muted">{r.s32 ?? "—"}</span> },
              ]} />
            <div className="flex flex-wrap items-center gap-4 mt-3 text-[13px] text-muted">
              <span className="inline-flex items-center gap-1.5">
                <span className="w-3.5 h-3.5 rounded bg-[#A7F3D0] border border-emerald-400" />
                Kết quả định giá cho đội sourcing
              </span>
              <span>Trần giá vốn = Giá bán mục tiêu × {cogsPct}% · bấm vào dòng để xem từng điểm giá.</span>
            </div>
          </Card>
          </>
        )}

        {picking && <ShortlistDrawer session={session} meta={meta}
          onClose={() => setPicking(false)} onDone={() => { setPicking(false); onChanged(); }} />}
        {detail && <PricingDrawer product={detail} session={session} meta={meta}
          onClose={() => setDetail(null)} />}
      </div>
    );
  }

  /* =================== VÙNG 4 — Nguồn hàng & báo giá =================== */
  const SUP_BG = "bg-[#FFF7ED]";                      /* nguồn sỉ — cam ấm như dữ liệu sàn */
  const QUOTE_BG = "bg-[#EEF2FF]";                    /* số đội mua hàng nhập — tím lạnh */
  const QUOTE_HEAD = "!text-indigo-900 bg-[#C7D2FE] border-l-2 border-indigo-400";
  const SUP_HEAD = "!text-orange-900 bg-[#FED7AA] border-l-2 border-orange-400";

  /* Form đội mua hàng nhập số đàm phán thực tế + danh sách NCC cào được */
  function SourcingDrawer({ product: p, session, meta, onClose, onSaved }) {
    const { Drawer, Button, Badge, Field, Input, Empty } = U();
    const toast = U().useToast();
    const [sel, setSel] = useState(p.supplier_id || null);
    const [form, setForm] = useState({
      negotiated_cogs: p.negotiated_cogs || "", logistics_fee: p.logistics_fee || "",
      lead_time_days: p.lead_time_days || "", note: p.sourcing_note || "",
      quoted_by: p.quoted_by || "",
    });
    const [busy, setBusy] = useState(false);
    const cfg = meta.sourcing || {};

    const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }));
    const num = (v) => (v === "" || v == null ? null : Number(v));

    const fillHint = async (sup) => {
      const h = await api(`/api/niche/suppliers/${sup.id}/quote-hint`);
      setSel(sup.id);
      setForm((f) => ({ ...f, negotiated_cogs: h.negotiated_cogs, logistics_fee: h.logistics_fee,
                        lead_time_days: h.lead_time_days }));
      toast("Đã điền số gợi ý — đội mua hàng sửa lại theo thực tế đàm phán");
    };

    const save = async () => {
      setBusy(true);
      try {
        await api(`/api/niche/products/${p.id}/quote`, { method: "POST", body: {
          supplier_id: sel, negotiated_cogs: num(form.negotiated_cogs),
          logistics_fee: num(form.logistics_fee), lead_time_days: num(form.lead_time_days),
          note: form.note || null, quoted_by: form.quoted_by || null } });
        toast("Đã lưu báo giá & chấm lại điểm S4");
        onSaved();
      } catch (e) { toast("Lỗi: " + e.message); } finally { setBusy(false); }
    };

    // xem trước landed cost ngay khi gõ, không cần đợi lưu
    const landed = (num(form.negotiated_cogs) || 0) + (num(form.logistics_fee) || 0);
    const over = p.cogs_max && landed > p.cogs_max;
    const share = p.p_target ? (num(form.logistics_fee) || 0) / p.p_target : null;

    return (
      <Drawer title={p.name}
        subtitle={`Trần giá vốn ${usd(p.cogs_max)} · giá bán mục tiêu ${usd(p.p_target)} · kho nhận ${session.warehouse || "EU"}`}
        onClose={onClose} width="max-w-3xl"
        footer={
          <div className="flex items-center justify-between gap-3">
            <span className={`text-[13px] ${over ? "text-error font-medium" : "text-muted"}`}>
              {landed ? (over
                ? `Landed cost ${usd(landed)} vượt trần ${usd(p.cogs_max)}`
                : `Landed cost ${usd(landed)} — còn dưới trần ${usd(p.cogs_max)}`) : "Chưa nhập báo giá"}
            </span>
            <div className="flex gap-2">
              <Button onClick={onClose}>Đóng</Button>
              <Button variant="primary" icon="check" disabled={busy} onClick={save}>
                {busy ? "Đang lưu…" : "Lưu báo giá & chấm điểm"}
              </Button>
            </div>
          </div>}>
        <div className="space-y-6">
          {/* --- 4.1 NCC cào tự động --- */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <h5 className="text-[13px] font-semibold text-ink">
                Nhà cung cấp cào được ({(p.suppliers || []).length})
              </h5>
              <Badge tone="slate">
                {cfg.supplier_source === "api" ? "API nguồn sỉ" : "demo"}
              </Badge>
            </div>
            {!(p.suppliers || []).length ? (
              <Empty icon="package" title="Chưa cào được nhà cung cấp nào" />
            ) : (
              <div className="border border-line rounded-[10px] divide-y divide-line">
                {p.suppliers.map((x) => {
                  const on = sel === x.id;
                  const cheap = p.cogs_max && x.listed_cogs_usd <= p.cogs_max;
                  return (
                    <div key={x.id} className={`p-3 flex items-center gap-3 ${on ? "bg-softblue" : SUP_BG}`}>
                      <button onClick={() => setSel(x.id)}
                        className={`w-4 h-4 shrink-0 rounded-full border flex items-center justify-center ${
                          on ? "border-primary" : "border-gray-300"}`}>
                        {on && <span className="w-2 h-2 rounded-full bg-primary" />}
                      </button>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <ShopLogo platform={x.platform} />
                          <span className="text-sm font-medium text-ink truncate">{x.supplier_name}</span>
                        </div>
                        <div className="text-[12px] text-muted truncate">
                          {x.title} · MOQ {x.moq} · {x.years_active} năm · mua lại {pct(x.repeat_rate)}
                          {" "}· {x.rating}★ · khớp {x.match_type === "image" ? "ảnh" : "từ khoá"} {pct(x.match_score)}
                        </div>
                      </div>
                      <div className="text-right shrink-0">
                        <div className={`text-sm font-semibold tabular-nums ${cheap ? "text-success" : "text-error"}`}>
                          {usd(x.listed_cogs_usd)}
                        </div>
                        <div className="text-[11px] text-muted tabular-nums">
                          {x.listed_cogs} {x.currency}
                        </div>
                      </div>
                      <div className="text-right shrink-0 w-16">
                        <div className="text-[12px] text-muted">uy tín</div>
                        <div className="text-sm font-semibold text-ink tabular-nums">{x.trust_score}</div>
                      </div>
                      <div className="flex flex-col gap-1 shrink-0">
                        <button onClick={() => fillHint(x)}
                          className="text-primary hover:underline text-[12px] whitespace-nowrap">
                          Điền số gợi ý
                        </button>
                        {x.url && (
                          <a href={x.url} target="_blank" rel="noreferrer"
                            className="text-muted hover:text-ink text-[12px]">Mở →</a>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* --- 4.2 Đội mua hàng nhập số thực tế --- */}
          <div className={`p-4 rounded-xl border border-indigo-200 ${QUOTE_BG}`}>
            <h5 className="text-[13px] font-semibold text-ink mb-1">
              Đội mua hàng nhập số đàm phán thực tế
            </h5>
            <p className="text-[12px] text-muted mb-3">
              Số cào tự động chỉ là giá niêm yết. Điểm S4 chấm trên số đàm phán thật sau khi chốt MOQ.
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <Field label="Giá vốn đàm phán (USD)">
                <Input type="number" step="0.01" value={form.negotiated_cogs}
                  onChange={set("negotiated_cogs")} placeholder="0.00" />
              </Field>
              <Field label="Phí logistics về kho (USD)"
                hint={share != null && share > 0 ? `${pct(share)} giá bán` : null}>
                <Input type="number" step="0.01" value={form.logistics_fee}
                  onChange={set("logistics_fee")} placeholder="0.00" />
              </Field>
              <Field label="Lead time (ngày)" hint="Từ chốt đơn tới khi nhập kho EU">
                <Input type="number" value={form.lead_time_days}
                  onChange={set("lead_time_days")} placeholder="0" />
              </Field>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
              <Field label="Người nhập">
                <Input value={form.quoted_by} onChange={set("quoted_by")} placeholder="Tên / bộ phận" />
              </Field>
              <Field label="Ghi chú đàm phán">
                <Input value={form.note} onChange={set("note")} placeholder="Điều kiện, MOQ đã chốt…" />
              </Field>
            </div>
          </div>

          {/* --- 4.3 Điểm thành phần --- */}
          {p.s4 != null && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <h5 className="text-[13px] font-semibold text-ink">Điểm nguồn hàng & ship EU (S4)</h5>
                <span className="text-xl font-bold tabular-nums" style={{
                  color: p.s4 >= 8 ? "#16A34A" : p.s4 >= 5 ? "#F59E0B" : "#DC2626" }}>{p.s4}/10</span>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                {[["S4.1 — Giá vốn đàm phán", p.s41_cogs, `Landed ${usd(p.landed_cost)} / trần ${usd(p.cogs_max)}`,
                   cfg.weights && cfg.weights.cogs],
                  ["S4.2 — Chi phí logistics", p.s42_logistics,
                   `${p.logistics_share != null ? pct(p.logistics_share) : "—"} giá bán · ≤${pct(cfg.logistics_good)} là tốt`,
                   cfg.weights && cfg.weights.logistics],
                  ["S4.3 — Thời gian giao hàng", p.s43_leadtime,
                   `${p.lead_time_days || "—"} ngày · 3–5 ngày là 10 điểm`,
                   cfg.weights && cfg.weights.leadtime]].map(([label, v, note, w]) => (
                  <div key={label} className="p-3 rounded-xl border border-line">
                    <div className="flex items-center justify-between">
                      <span className="text-[13px] font-medium text-ink">{label}</span>
                      <span className="text-lg font-bold tabular-nums" style={{
                        color: (v || 0) >= 8 ? "#16A34A" : (v || 0) >= 5 ? "#F59E0B" : "#DC2626" }}>
                        {v ?? "—"}</span>
                    </div>
                    <div className="h-1.5 rounded-full bg-hover overflow-hidden mt-2">
                      <div className="h-full rounded-full" style={{ width: `${(v || 0) * 10}%`,
                        background: (v || 0) >= 8 ? "#16A34A" : (v || 0) >= 5 ? "#F59E0B" : "#DC2626" }} />
                    </div>
                    <p className="text-[11px] text-muted mt-1.5">
                      {note}{w ? ` · trọng số ${pct(w)}` : ""}
                    </p>
                  </div>
                ))}
              </div>
              {p.over_ceiling && (
                <p className="text-[13px] text-error mt-2">
                  Landed cost {usd(p.landed_cost)} đã vượt trần giá vốn {usd(p.cogs_max)} — sản phẩm
                  vi phạm ngưỡng an toàn tài chính, cần đàm phán lại hoặc nâng giá bán.
                </p>
              )}
            </div>
          )}
        </div>
      </Drawer>
    );
  }

  function SourcingSection({ session, meta, onChanged }) {
    const { Card, Button, Badge, Empty, Spinner, DataTable, Field, Input, Select } = U();
    const toast = U().useToast();
    const [data, setData] = useState(null);
    const [detail, setDetail] = useState(null);
    const [wh, setWh] = useState(session.warehouse || "EU");
    const [moq, setMoq] = useState(session.target_moq || 10);
    const running = session.step4_status === "running";
    const cfg = meta.sourcing || {};

    const load = useCallback(() => {
      api(`/api/niche/sessions/${session.id}/sourcing`).then((r) => {
        setData(r.products);
        setDetail((d) => (d ? r.products.find((x) => x.id === d.id) || null : null));
      });
    }, [session.id]);
    useEffect(load, [load]);
    useEffect(() => { if (!running) load(); }, [running]);

    const crawl = async () => {
      try {
        await api(`/api/niche/sessions/${session.id}/step4/run`, { method: "POST",
          body: { lang: LANG(), warehouse: wh, target_moq: Number(moq) } });
        onChanged();
        toast("Đang cào nhà cung cấp trên các nguồn sỉ");
      } catch (e) { toast("Lỗi: " + e.message); }
    };

    const stages = meta.step4_stages || [];
    const stepSession = { progress: session.step4_progress, stage: session.step4_stage,
                          status: session.step4_status, message: session.step4_message };

    if (session.step3_status !== "done") {
      return <ComingSoon note="Cần hoàn tất Bước 3 để có trần giá vốn (COGS_max) làm mốc đàm phán." />;
    }

    if (!session.step4_status || (session.step4_status === "draft" && !running)) {
      return (
        <div className="rounded-xl border border-dashed border-gray-300 bg-bg px-6 py-10 text-center">
          <p className="text-sm font-medium text-ink">Chưa truy tìm nguồn hàng cho ngách này</p>
          <p className="text-[13px] text-muted mt-1 mb-4">
            Hệ thống dùng ảnh &amp; bộ từ khoá của từng SKU để tìm nhà cung cấp trên 1688, Taobao,
            Alibaba và kho EU, rồi so giá niêm yết với trần giá vốn của Bước 3.
          </p>
          <div className="flex flex-wrap items-end justify-center gap-3 mb-4 text-left">
            <div className="w-40">
              <Field label="Kho nhận hàng">
                <Select value={wh} onChange={(e) => setWh(e.target.value)}>
                  <option value="EU">Kho EU</option>
                  <option value="ES">Kho Tây Ban Nha</option>
                  <option value="NL">Kho Hà Lan</option>
                </Select>
              </Field>
            </div>
            <div className="w-32">
              <Field label="MOQ mong muốn">
                <Input type="number" value={moq} onChange={(e) => setMoq(e.target.value)} />
              </Field>
            </div>
          </div>
          <div><Button variant="primary" icon="search" onClick={crawl}>
            Truy tìm nguồn hàng &amp; cào giá niêm yết
          </Button></div>
        </div>
      );
    }

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted">
            Chạy lúc: <span className="text-ink font-medium">{dt(session.last_step4_at)}</span>
            <span> · kho nhận {session.warehouse || "EU"}</span>
            {session.target_moq ? <span> · MOQ mong muốn {session.target_moq}</span> : null}
            <span> · nguồn NCC {cfg.supplier_source === "api" ? "API nguồn sỉ" : "mô phỏng"}</span>
          </div>
          <Button variant="primary" icon="refresh" disabled={running} onClick={crawl}>
            {running ? "Đang cào…" : "Cào lại nguồn hàng"}
          </Button>
        </div>

        {(running || session.step4_status === "error") && (
          <StepProgress session={stepSession} steps={stages} prefix="4" />
        )}

        {!data ? <Spinner /> : !data.length ? (
          <Empty icon="package" title="Chưa có SKU nào trong shortlist" />
        ) : (
          <Card title={`So sánh nguồn hàng & chi phí thực tế (${data.length} SKU)`}
            action={<Badge tone="slate">
              {data.filter((x) => x.s4 != null).length}/{data.length} SKU đã có báo giá
            </Badge>}>
            <DataTable rows={data} pageSize={10} searchKeys={["name"]}
              onRowClick={(r) => setDetail(r)}
              columns={[
                { key: "name", label: "SKU / Cụm sản phẩm",
                  cellClass: STICK_NAME.replace("left-[68px]", "left-0"),
                  headClass: STICK_NAME_HEAD.replace("left-[68px]", "left-0"),
                  render: (r) => (
                    <div className="flex items-center gap-2.5 min-w-[210px]">
                      <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                      <div className="min-w-0">
                        <div className="font-medium text-ink truncate max-w-[200px]">{r.name}</div>
                        <div className="text-[12px] text-muted">S3 {r.s3 ?? "—"}/10</div>
                      </div>
                    </div>
                  ) },
                { key: "s4", label: "Điểm S4", align: "right",
                  render: (r) => (r.s4 == null
                    ? <span className="text-[12px] text-gray-400" title="Chờ đội mua hàng nhập báo giá">chờ báo giá</span>
                    : <span className={`text-lg font-bold tabular-nums ${
                        r.s4 >= 8 ? "text-success" : r.s4 >= 5 ? "text-warning" : "text-error"}`}>{r.s4}</span>) },
                { key: "cogs_max", label: "Trần giá vốn", align: "right",
                  render: (r) => <span className="tabular-nums font-medium">{usd(r.cogs_max)}</span> },

                /* ---- cào tự động ---- */
                { key: "suppliers_count", label: "Số NCC", align: "right",
                  cellClass: `${SUP_BG} border-l-2 border-orange-400`, headClass: SUP_HEAD,
                  render: (r) => <span className="tabular-nums">{r.suppliers_count ?? "—"}</span> },
                { key: "listed_cogs_min", label: "Niêm yết thấp nhất", align: "right",
                  cellClass: SUP_BG, headClass: "!text-orange-900 bg-[#FED7AA]",
                  render: (r) => <span className="tabular-nums">{usd(r.listed_cogs_min)}</span> },
                { key: "listed_cogs_avg", label: "Niêm yết TB", align: "right",
                  cellClass: SUP_BG, headClass: "!text-orange-900 bg-[#FED7AA]",
                  render: (r) => <span className="tabular-nums">{usd(r.listed_cogs_avg)}</span> },
                { key: "s4_est", label: "Điểm ước tính", align: "right",
                  cellClass: SUP_BG, headClass: "!text-orange-900 bg-[#FED7AA]",
                  render: (r) => <span className="tabular-nums text-muted">{r.s4_est ?? "—"}</span> },

                /* ---- đội mua hàng nhập ---- */
                { key: "negotiated_cogs", label: "Giá đàm phán", align: "right",
                  cellClass: `${QUOTE_BG} border-l-2 border-indigo-400`, headClass: QUOTE_HEAD,
                  render: (r) => <span className="tabular-nums">{usd(r.negotiated_cogs)}</span> },
                { key: "logistics_fee", label: "Phí logistics", align: "right",
                  cellClass: QUOTE_BG, headClass: "!text-indigo-900 bg-[#C7D2FE]",
                  render: (r) => (r.logistics_fee == null ? <span className="text-[12px] text-gray-400">—</span>
                    : <span className="tabular-nums">{usd(r.logistics_fee)}
                        {r.logistics_share != null && (
                          <span className="text-[11px] text-muted"> ({pct(r.logistics_share)})</span>)}
                      </span>) },
                { key: "landed_cost", label: "Landed cost", align: "right",
                  cellClass: QUOTE_BG, headClass: "!text-indigo-900 bg-[#C7D2FE]",
                  render: (r) => (r.landed_cost == null ? <span className="text-[12px] text-gray-400">—</span>
                    : <span className={`tabular-nums font-bold ${r.over_ceiling ? "text-error" : "text-success"}`}>
                        {usd(r.landed_cost)}</span>) },
                { key: "lead_time_days", label: "Lead time", align: "right",
                  cellClass: QUOTE_BG, headClass: "!text-indigo-900 bg-[#C7D2FE]",
                  render: (r) => (r.lead_time_days
                    ? <span className="tabular-nums">{r.lead_time_days} ngày</span>
                    : <span className="text-[12px] text-gray-400">—</span>) },
                { key: "s41_cogs", label: "S4.1 giá vốn", align: "right",
                  render: (r) => <span className="tabular-nums text-muted">{r.s41_cogs ?? "—"}</span> },
                { key: "s42_logistics", label: "S4.2 logistics", align: "right",
                  render: (r) => <span className="tabular-nums text-muted">{r.s42_logistics ?? "—"}</span> },
                { key: "s43_leadtime", label: "S4.3 lead time", align: "right",
                  render: (r) => <span className="tabular-nums text-muted">{r.s43_leadtime ?? "—"}</span> },
              ]} />
            <div className="flex flex-wrap items-center gap-4 mt-3 text-[13px] text-muted">
              <span className="inline-flex items-center gap-1.5">
                <span className="w-3.5 h-3.5 rounded bg-[#FED7AA] border border-orange-400" />
                Cào tự động từ nguồn sỉ
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="w-3.5 h-3.5 rounded bg-[#C7D2FE] border border-indigo-400" />
                Đội mua hàng nhập tay
              </span>
              <span>Bấm vào dòng để xem NCC và nhập báo giá đàm phán.</span>
            </div>
          </Card>
        )}

        {detail && <SourcingDrawer product={detail} session={session} meta={meta}
          onClose={() => setDetail(null)} onSaved={() => { load(); onChanged(); }} />}
      </div>
    );
  }

  /* =================== VÙNG 4 — Tổng hợp điểm sản phẩm =================== */
  const CRIT_TONE = (v) => (v == null ? "#9CA3AF" : v >= 8 ? "#16A34A" : v >= 5.5 ? "#F59E0B" : "#DC2626");

  /* Đánh giá 1 sản phẩm: bảng điểm 6 tiêu chí + gợi ý bán hàng */
  function SummaryDrawer({ product: p, meta, onClose }) {
    const { Drawer, Badge } = U();
    const bd = p.score_breakdown || {};
    const pb = p.playbook || {};
    const crit = bd.criteria || [];

    return (
      <Drawer title={p.name}
        subtitle={`Điểm tổng ${p.total_score ?? "—"}/10 · ${crit.filter((c) => !c.missing).length}/${crit.length} tiêu chí có dữ liệu`}
        onClose={onClose} width="max-w-3xl">
        <div className="space-y-6">
          {pb.angle && (
            <div className="p-4 rounded-xl bg-softblue border border-primary/30">
              <div className="flex items-center justify-between gap-2 mb-1">
                <h5 className="text-[13px] font-semibold text-ink">Định vị bán hàng</h5>
                <Badge tone="slate">{p.playbook_source === "claude" ? "Claude AI" : "Rule engine"}</Badge>
              </div>
              <p className="text-sm text-ink">{pb.angle}</p>
            </div>
          )}

          {/* bảng điểm từng tiêu chí */}
          <div>
            <h5 className="text-[13px] font-semibold text-ink mb-2">Điểm theo từng tiêu chí</h5>
            <div className="space-y-2.5">
              {crit.map((c) => (
                <div key={c.key} className={c.missing ? "opacity-60" : ""}>
                  <div className="flex items-center justify-between gap-2 text-[13px]">
                    <span className="text-gray-700">
                      {c.label} <span className="text-disabled">×{pct(c.weight)}</span>
                    </span>
                    <span className="tabular-nums font-semibold" style={{ color: CRIT_TONE(c.score) }}>
                      {c.missing ? "chưa có dữ liệu" : c.score}
                    </span>
                  </div>
                  <div className="h-1.5 rounded-full bg-hover overflow-hidden mt-1">
                    <div className="h-full rounded-full" style={{ width: `${(c.score || 0) * 10}%`,
                      background: CRIT_TONE(c.score) }} />
                  </div>
                  {c.note && <p className="text-[12px] text-muted mt-1 leading-snug">{c.note}</p>}
                </div>
              ))}
            </div>
            {bd.coverage != null && bd.coverage < 1 && (
              <p className="text-[12px] text-warning mt-2">
                Điểm tổng đang tính trên {pct(bd.coverage)} trọng số — các tiêu chí chưa có dữ liệu
                đã bị loại khỏi công thức thay vì đoán bừa.
              </p>
            )}
          </div>

          {/* điểm mạnh / điểm yếu */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div className="p-4 rounded-xl bg-green-50 border border-green-100">
              <h5 className="text-[13px] font-semibold text-success mb-2">Điểm mạnh cần khai thác</h5>
              <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                {(pb.strengths || []).map((x, i) => <li key={i}>{x}</li>)}
              </ul>
            </div>
            <div className="p-4 rounded-xl bg-amber-50 border border-amber-100">
              <h5 className="text-[13px] font-semibold text-amber-700 mb-2">Điểm yếu phải xử lý</h5>
              <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                {(pb.weaknesses || []).map((x, i) => <li key={i}>{x}</li>)}
              </ul>
            </div>
          </div>

          {/* việc làm ngay */}
          <div>
            <h5 className="text-[13px] font-semibold text-ink mb-2">Việc làm ngay</h5>
            <div className="border border-line rounded-[10px] divide-y divide-line">
              {(pb.actions || []).map((x, i) => (
                <div key={i} className="p-3 flex items-start gap-3">
                  <span className="w-6 h-6 shrink-0 rounded-full bg-primary text-white text-[12px]
                                   font-semibold flex items-center justify-center">{i + 1}</span>
                  <span className="text-[13px] text-ink">{x}</span>
                </div>
              ))}
            </div>
          </div>

          {/* số liệu chốt */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {[["Giá bán mục tiêu", usd(p.p_target)], ["Trần giá vốn", usd(p.cogs_max)],
              ["Landed cost", usd(p.landed_cost)], ["Tệp ads", p.aud_age_top || "—"]].map(([k, v]) => (
              <div key={k} className="p-3 rounded-[10px] border border-line">
                <div className="text-[12px] text-muted">{k}</div>
                <div className="text-[15px] font-semibold text-ink tabular-nums">{v}</div>
              </div>
            ))}
          </div>
        </div>
      </Drawer>
    );
  }

  function SummarySection({ session, meta, onChanged }) {
    const { Card, Button, Badge, Empty, Spinner, DataTable } = U();
    const toast = U().useToast();
    const [data, setData] = useState(null);
    const [busy, setBusy] = useState(false);
    const [detail, setDetail] = useState(null);
    const weights = meta.total_scoring || [];

    const load = useCallback(() => {
      api(`/api/niche/sessions/${session.id}/summary`).then((r) => setData(r.products));
    }, [session.id]);
    useEffect(load, [load]);

    const run = async () => {
      setBusy(true);
      try {
        const r = await api(`/api/niche/sessions/${session.id}/summarize`,
          { method: "POST", body: { lang: LANG() } });
        setData(r.products);
        toast(`Đã chấm điểm tổng cho ${r.products.length} sản phẩm`);
        onChanged();
      } catch (e) { toast("Lỗi: " + e.message); } finally { setBusy(false); }
    };

    if (session.step2_status !== "done") {
      return <ComingSoon note="Cần chạy Bước 2 để có danh sách sản phẩm trước khi chấm điểm tổng." />;
    }

    return (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted flex flex-wrap items-center gap-x-1.5">
            <span>Điểm tổng gộp cả 4 bước theo trọng số:</span>
            {weights.map((w, i) => (
              <span key={w.key}>
                {i ? "· " : ""}<span>{w.label}</span> <span className="tabular-nums">{pct(w.weight)}</span>
              </span>
            ))}
          </div>
          <Button variant="primary" icon="refresh" disabled={busy} onClick={run}>
            {busy ? "Đang chấm điểm…" : data && data.length ? "Chấm lại điểm tổng" : "Chấm điểm tổng"}
          </Button>
        </div>

        {!data ? <Spinner /> : !data.length ? (
          <div className="rounded-xl border border-dashed border-gray-300 bg-bg px-6 py-10 text-center">
            <p className="text-sm font-medium text-ink">Chưa chấm điểm tổng</p>
            <p className="text-[13px] text-muted mt-1 mb-4">
              Hệ thống sẽ gộp điểm mùa vụ, chất lượng tệp, độ khớp tệp ads, sức hút sản phẩm,
              dư địa giá và nguồn hàng thành một điểm duy nhất cho từng SKU, kèm gợi ý bán hàng.
            </p>
            <Button variant="primary" icon="play" disabled={busy} onClick={run}>
              {busy ? "Đang chấm điểm…" : "Chấm điểm tổng"}
            </Button>
          </div>
        ) : (
          <Card title={`Bảng điểm tổng hợp (${data.length} SKU)`}
            action={<Badge tone="slate">
              {data[0] && data[0].playbook_source === "claude" ? "Gợi ý bởi Claude AI" : "Gợi ý bởi rule engine"}
            </Badge>}>
            <DataTable rows={data} pageSize={10} searchKeys={["name"]}
              onRowClick={(r) => setDetail(r)}
              columns={[
                { key: "name", label: "SKU / Cụm sản phẩm",
                  cellClass: STICK_NAME.replace("left-[68px]", "left-0"),
                  headClass: STICK_NAME_HEAD.replace("left-[68px]", "left-0"),
                  render: (r) => (
                    <div className="flex items-center gap-2.5 min-w-[210px]">
                      <Thumb src={r.image_url} className="w-9 h-9 shrink-0" rounded="rounded-lg" />
                      <div className="min-w-0">
                        <div className="font-medium text-ink truncate max-w-[200px]">{r.name}</div>
                        <div className="text-[12px] text-muted truncate max-w-[200px]">
                          {(r.playbook || {}).angle || r.matched_keyword}
                        </div>
                      </div>
                    </div>
                  ) },
                { key: "total_score", label: "Điểm tổng", align: "right",
                  cellClass: "bg-[#EEF2FF]", headClass: "!text-indigo-900 bg-[#C7D2FE] border-l-2 border-indigo-400",
                  render: (r) => (
                    <span className="text-xl font-bold tabular-nums" style={{ color: CRIT_TONE(r.total_score) }}>
                      {r.total_score ?? "—"}</span>
                  ) },
                ...(weights.map((w) => ({
                  key: `c_${w.key}`, label: w.label, align: "right",
                  render: (r) => {
                    const c = ((r.score_breakdown || {}).criteria || []).find((x) => x.key === w.key);
                    if (!c || c.missing) return <span className="text-[12px] text-gray-400">—</span>;
                    return <span className="tabular-nums font-medium" style={{ color: CRIT_TONE(c.score) }}>
                      {c.score}</span>;
                  },
                }))),
              ]} />
            <div className="flex flex-wrap items-center gap-4 mt-3 text-[13px] text-muted">
              <span>Bấm vào dòng để xem đánh giá chi tiết và gợi ý bán hàng cho từng sản phẩm.</span>
              <span>Tiêu chí “—” là chưa có dữ liệu — đã bị loại khỏi công thức, không đoán bừa.</span>
            </div>
          </Card>
        )}

        {detail && <SummaryDrawer product={detail} meta={meta} onClose={() => setDetail(null)} />}
      </div>
    );
  }

  /* Mục lục phụ bên trái — theo dõi số liệu & nhảy tới từng vùng */
  function SideNav({ items, active, onGo }) {
    return (
      <nav className="sticky top-20 space-y-1 max-h-[calc(100vh-6rem)] overflow-y-auto pr-1">
        <div className="text-[11px] font-semibold uppercase tracking-wide text-disabled px-3 pb-1.5">
          Mục lục
        </div>
        {items.map((it, i) => {
          const on = active === it.id;
          return (
            <button key={it.id} onClick={() => onGo(it.id)}
              className={`w-full text-left pl-2.5 pr-3 py-2.5 rounded-[10px] border-l-[3px] transition-all
                          flex items-start gap-2.5 ${
                on ? "border-primary bg-softblue shadow-soft"
                   : "border-transparent hover:bg-hover hover:border-gray-200"}`}>
              <span className={`w-6 h-6 shrink-0 rounded-lg text-[12px] font-bold flex items-center
                                justify-center transition-colors ${
                on ? "bg-primary text-white" : "bg-hover text-muted"}`}>
                {i + 1}
              </span>
              <span className="min-w-0 flex-1">
                <span className={`block text-[13px] leading-snug ${
                  on ? "font-bold text-ink" : "font-normal text-gray-600"}`}>
                  {it.label}
                </span>
                {it.value != null && (
                  <span className="block text-[12px] tabular-nums mt-0.5"
                    style={{ color: CRIT_TONE(it.value) }}>{it.value}/10 · {it.note}</span>
                )}
                {it.value == null && it.note && (
                  <span className="block text-[12px] text-muted mt-0.5">{it.note}</span>
                )}
              </span>
            </button>
          );
        })}
      </nav>
    );
  }

  /* =================== MÀN HÌNH B — hồ sơ ngách =================== */
  function Profile({ session, onBack, onRerun, onRefresh, meta }) {
    const { Card, Button, Badge, Spinner, KpiCard } = U();
    const [data, setData] = useState(null);
    const [audQuality, setAudQuality] = useState(null);
    const running = session.status === "running";

    /* mục lục phụ: cuộn tới vùng + tự sáng vùng đang xem */
    const secRefs = { market: useRef(null), products: useRef(null),
                      finance: useRef(null), summary: useRef(null) };
    const [activeSec, setActiveSec] = useState("market");
    const SPY_OFFSET = 120;      // chừa chỗ cho thanh header dính phía trên
    const goto = (id) => {
      const el = secRefs[id] && secRefs[id].current;
      if (!el) return;
      const top = el.getBoundingClientRect().top + window.scrollY - SPY_OFFSET;
      window.scrollTo({ top, behavior: "smooth" });
      setActiveSec(id);          // sáng ngay, không đợi scroll event
    };
    useEffect(() => {
      const onScroll = () => {
        let cur = null;
        Object.entries(secRefs).forEach(([k, r]) => {
          if (r.current && r.current.getBoundingClientRect().top <= SPY_OFFSET + 20) cur = k;
        });
        // chạm đáy trang -> luôn sáng vùng cuối, tránh vùng ngắn không bao giờ active
        const atBottom = window.innerHeight + window.scrollY >= document.body.scrollHeight - 60;
        setActiveSec(atBottom ? "summary" : (cur || "market"));
      };
      window.addEventListener("scroll", onScroll, { passive: true });
      window.addEventListener("resize", onScroll);
      onScroll();
      return () => {
        window.removeEventListener("scroll", onScroll);
        window.removeEventListener("resize", onScroll);
      };
    }, [data]);

    const load = useCallback(() => {
      api(`/api/niche/sessions/${session.id}/profile`).then(setData);
    }, [session.id]);
    useEffect(load, [load]);
    useEffect(() => { if (!running && data) load(); }, [running]);

    const s = session;
    const toc = [
      { id: "market", label: "Quy mô & biến động thị trường", value: s.step1_score,
        note: s.demand_label || "mùa vụ" },
      { id: "products", label: "Sản phẩm tiềm năng", value: s.step2_score,
        note: s.step2_status === "done" ? "S2 cao nhất" : "chưa quét" },
      { id: "finance", label: "Cấu trúc tài chính", value: s.step3_score,
        note: s.step4_score != null ? `S3 · S4 ${s.step4_score}/10` : "S3 định giá" },
      { id: "summary", label: "Tổng hợp điểm sản phẩm", value: null,
        note: "điểm tổng & gợi ý bán hàng" },
    ];
    const kws = (data && data.keywords) || [];
    const aud = (data && data.audience) || [];
    const segs = (data && data.segments) || {};

    return (
      <div>
        <button onClick={onBack} className="inline-flex items-center gap-1.5 text-[13px] text-muted hover:text-ink mb-3">
          <NIcon name="arrowLeft" className="w-4 h-4" /> Tất cả ngách đã nghiên cứu
        </button>

        <div className="flex flex-wrap items-start justify-between gap-4 mb-6">
          <div>
            <h1 className="text-[28px] font-bold text-ink tracking-tight leading-tight">{s.raw_keyword}</h1>
            <div className="flex flex-wrap items-center gap-2 mt-1.5">
              <Badge tone="blue"><Markets session={s} max={4} /></Badge>
              {s.demand_type && <Badge tone={DEMAND_TONE[s.demand_type]}>{s.demand_label}</Badge>}
              <Badge tone="slate">{s.volume_source === "google_ads" ? "Google Ads API" : "Số liệu demo"}</Badge>
              <span className="text-[13px] text-muted">Chạy lúc: {dt(s.last_run_at)}</span>
            </div>
          </div>
          <Button variant="primary" icon="refresh" disabled={running} onClick={onRerun}>
            {running ? "Đang chạy…" : "Chạy lại phân tích"}
          </Button>
        </div>

        {running && <div className="mb-6"><StepProgress session={s} steps={meta.steps} /></div>}
        {!data ? <Spinner /> : (
          <div className="space-y-10">
            <div className="flex gap-6 items-stretch">
              {/* mục lục phụ — dính theo màn hình, sáng đậm vùng đang xem.
                 Dùng items-stretch để aside cao bằng cột nội dung; nếu để items-start
                 thì aside co lại bằng chiều cao của chính nó và sticky mất tác dụng. */}
              <aside className="hidden lg:block w-60 shrink-0 relative">
                <SideNav items={toc} active={activeSec} onGo={goto} />
              </aside>

              <div className="min-w-0 flex-1 space-y-10">
                {/* ---------------- VÙNG 1 ---------------- */}
                <section ref={secRefs.market} id="sec-market">
                  <SectionHead index="1" title="Quy mô & biến động thị trường"
                    subtitle="Bộ từ khoá bản địa · dung lượng tìm kiếm 12 tháng · mùa vụ · tệp đối tượng" />
                  <div className="space-y-5">
                    <TrendBlock kws={kws} source={s.kw_source} />
                    <SeasonalityBlock s={s} />
                    <AudienceBuilder sessionId={s.id} audience={aud} segments={segs}
                      bands={meta.audience_bands} onSegmentsChanged={load} onQuality={setAudQuality} />
                    <AudienceScoreBlock quality={audQuality} session={s} />
                    <Card title="Tổng kết Bước 1 — rủi ro & hành động" action={
                      <Badge tone="slate">{s.ai_source === "claude" ? "Claude AI" : "Rule engine"}</Badge>}>
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        <div className="p-4 rounded-xl bg-amber-50 border border-amber-100">
                          <h4 className="text-[15px] font-semibold text-amber-700 mb-2">Rủi ro cần lưu ý</h4>
                          <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                            {(s.ai_risks || []).map((r, i) => <li key={i}>{r}</li>)}
                          </ul>
                        </div>
                        <div className="p-4 rounded-xl bg-green-50 border border-green-100">
                          <h4 className="text-[15px] font-semibold text-success mb-2">Hành động tiếp theo</h4>
                          <ul className="space-y-1.5 text-[13px] text-ink list-disc pl-4">
                            {(s.ai_actions || []).map((a, i) => <li key={i}>{a}</li>)}
                          </ul>
                        </div>
                      </div>
                    </Card>
                  </div>
                </section>

                {/* ---------------- VÙNG 2 ---------------- */}
                <section ref={secRefs.products} id="sec-products">
                  <SectionHead index="2" title="Sản phẩm tiềm năng"
                    subtitle="Spy quảng cáo · quét sàn TMĐT · tệp đối tượng thực tế của ads · xếp hạng theo S2"
                    right={s.step2_score != null && (
                      <div className="text-right">
                        <div className="text-2xl font-bold text-ink tabular-nums">{s.step2_score}</div>
                        <div className="text-[11px] text-muted">S2 cao nhất</div>
                      </div>)} />
                  <ProductMatrix session={s} meta={meta} onChanged={onRefresh} />
                </section>

                {/* ---------------- VÙNG 3 ---------------- */}
                <section ref={secRefs.finance} id="sec-finance">
                  <SectionHead index="3" title="Cấu trúc tài chính"
                    subtitle="Giá đối thủ · giá bán mục tiêu · trần giá vốn · nguồn cung & báo giá thực tế"
                    right={(s.step3_score != null || s.step4_score != null) && (
                      <div className="flex gap-5 text-right">
                        {s.step3_score != null && (
                          <div>
                            <div className="text-2xl font-bold text-ink tabular-nums">{s.step3_score}</div>
                            <div className="text-[11px] text-muted">S3 định giá</div>
                          </div>)}
                        {s.step4_score != null && (
                          <div>
                            <div className="text-2xl font-bold text-ink tabular-nums">{s.step4_score}</div>
                            <div className="text-[11px] text-muted">S4 nguồn hàng</div>
                          </div>)}
                      </div>)} />
                  <div className="space-y-6">
                    <FinanceConfig session={s} meta={meta} onSaved={onRefresh} />
                    <div>
                      <h3 className="text-[15px] font-semibold text-ink mb-3">
                        3.1 — Giá đối thủ &amp; trần tài chính
                      </h3>
                      <PricingSection session={s} meta={meta} onChanged={onRefresh} />
                    </div>
                    <div>
                      <h3 className="text-[15px] font-semibold text-ink mb-3">
                        3.2 — Nguồn cung &amp; báo giá thực tế
                      </h3>
                      <SourcingSection session={s} meta={meta} onChanged={onRefresh} />
                    </div>
                  </div>
                </section>

                {/* ---------------- VÙNG 4 ---------------- */}
                <section ref={secRefs.summary} id="sec-summary">
                  <SectionHead index="4" title="Tổng hợp điểm sản phẩm"
                    subtitle="Gộp điểm cả 4 bước theo trọng số · bấm từng sản phẩm để xem đánh giá & gợi ý bán hàng" />
                  <SummarySection session={s} meta={meta} onChanged={onRefresh} />
                </section>
              </div>
            </div>
          </div>
        )}
      </div>
    );
  }

  /* =================== Root =================== */
  function NicheResearch() {
    const { PageHead, Spinner } = U();
    const [meta, setMeta] = useState(null);
    const [sessions, setSessions] = useState([]);
    const [session, setSession] = useState(null);
    const [creating, setCreating] = useState(false);

    const loadSessions = useCallback(async () => {
      const list = await api("/api/niche/sessions");
      setSessions(list);
      return list;
    }, []);
    useEffect(() => { api("/api/niche/meta").then(setMeta); loadSessions(); }, []);

    const open = async (id) => setSession(await api(`/api/niche/sessions/${id}`));
    const refresh = useCallback(async () => {
      if (!session) return;
      const s = await api(`/api/niche/sessions/${session.id}`);
      setSession(s);
      setSessions((ls) => ls.map((x) => (x.id === s.id ? s : x)));
    }, [session && session.id]);

    useEffect(() => {
      if (!session || (session.status !== "running" && session.step2_status !== "running"
                       && session.step3_status !== "running"
                       && session.step4_status !== "running")) return;
      const t = setInterval(refresh, 1300);
      return () => clearInterval(t);
    }, [session && session.id, session && session.status, refresh]);

    const rerun = async () => {
      await api(`/api/niche/sessions/${session.id}/run`, { method: "POST", body: { lang: LANG() } });
      refresh();
    };

    if (!meta) return <Spinner />;
    return (
      <div>
        {!session && (
          <PageHead title="Nghiên cứu ngách"
            subtitle="Quy mô & biến động thị trường · sản phẩm tiềm năng · đối thủ cạnh tranh · nguồn hàng" />
        )}
        {session ? (
          <Profile session={session} meta={meta} onRerun={rerun} onRefresh={refresh}
            onBack={() => { setSession(null); loadSessions(); }} />
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

  window.NichePage = NicheResearch;
})();
