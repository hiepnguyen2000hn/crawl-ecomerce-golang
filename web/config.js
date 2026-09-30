/* config.js — runtime config cho web/ (site tĩnh, không build step, không .env).
   Đây là NƠI DUY NHẤT cần sửa khi đổi domain/server lúc deploy — không đụng vào
   app.jsx hay product-research-client.js.

   Mặc định "" (same-origin, dùng path tương đối): đúng khi có reverse proxy
   (Nginx/Caddy) forward /api/niche/* và /api/v1/* về đúng backend nội bộ —
   xem docs/superpowers/plans/2026-09-30-niche-product-research-integration.md.
   Nếu backend chạy ở domain/port riêng (không qua reverse proxy same-origin),
   điền thẳng origin đầy đủ, ví dụ "https://ai.yourdomain.com" — và nhớ bật CORS
   phía backend đó cho phép origin của trang web này. */
window.APP_CONFIG = {
  NICHE_API_BASE: "",
  PR_API_BASE: "",
};
