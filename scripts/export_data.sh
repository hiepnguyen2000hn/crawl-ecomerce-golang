#!/usr/bin/env bash
# Export crawled data from Postgres to CSV for manual review.
#
# Usage: ./scripts/export_data.sh [outdir]
# Defaults to ./exports. Override the connection with PGHOST/PGPORT/PGUSER/PGDATABASE.
set -euo pipefail

OUTDIR="${1:-exports}"
export PGHOST="${PGHOST:-localhost}"
export PGPORT="${PGPORT:-5433}"
export PGUSER="${PGUSER:-crawl}"
export PGPASSWORD="${PGPASSWORD:-crawl}"
export PGDATABASE="${PGDATABASE:-crawl}"

mkdir -p "$OUTDIR"

dump() { # dump <name> <query>
  psql -q -c "\copy ($2) TO '$OUTDIR/$1.csv' WITH (FORMAT csv, HEADER true)"
  # count via SQL: CSV fields contain embedded newlines, so wc -l would overcount
  echo "$OUTDIR/$1.csv  ($(psql -tAc "select count(*) from ($2) s") rows)"
}

dump jobs "select id, type, status, params::text, created_at, updated_at from jobs order by created_at"

dump trend "select t.id, t.job_id, t.keyword, t.trend_data::text, t.fetched_at
            from trend_raw t order by t.fetched_at"

dump fbads "select f.id, f.job_id, j.params->>'query' as query, f.ad_archive_id, f.page_id,
                   f.page_name, f.is_active, f.start_date, f.end_date, f.title, f.body,
                   f.cta_text, f.cta_type, f.link_url, f.fetched_at
            from fbads_raw f join jobs j on j.id = f.job_id
            order by f.fetched_at, f.ad_archive_id"

dump products "select p.id, p.job_id, p.ad_id, a.page_name, p.product_name, p.price, p.currency,
                      p.sku, p.url, p.created_at
               from products p left join fbads_raw a on a.id = p.ad_id
               order by p.created_at"

dump ai_results "select r.id, r.job_id, j.type as job_type, r.provider, r.model, r.prompt_type,
                        r.input_ref, r.output::text, r.created_at
                 from ai_results r join jobs j on j.id = r.job_id
                 order by r.created_at"

dump niche_sessions "select id, raw_keyword, countries::text, status, step1_score,
                            avg_monthly_searches, demand_type, demand_label, kw_source,
                            volume_source, ai_source, peak_months::text, low_months::text,
                            trend_direction, volatility, fluctuation_ratio, seasonality_note,
                            ai_summary, ai_risks::text, ai_actions::text, last_run_at, created_at
                     from niche_sessions order by created_at"
