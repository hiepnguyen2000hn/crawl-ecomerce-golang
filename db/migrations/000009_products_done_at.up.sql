-- products_done_at: product-extractor set khi đã xử lý xong mọi link_url của job fbads.
-- NULL = chưa xong. Tách khỏi jobs.status vì status thuộc ai-service. Service research (levelup_ai)
-- chờ cột này để biết sản phẩm trang đích đã đủ; thiếu cột thì nó phải chờ 180s "im lặng" cho mỗi job.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS products_done_at TIMESTAMPTZ;
