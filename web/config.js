/* config.js — runtime config cho web/ (site tĩnh, không build step).

   KHÔNG sửa file này nữa. Cấu hình giờ nằm ở .env (gốc repo):

     LEVELUP_API_BASE=https://research-data-ai.relipasoft.com
     NICHE_API_BASE=
     WEB_PORT=5500

   Chạy `node scripts/serve-web.js` — server đọc .env rồi SINH RA nội dung
   config.js lúc phục vụ, đè lên file này. File này chỉ là fallback khi mở
   web/ bằng một static server khác.

   Vì sao cả hai base đều để "": trình duyệt gọi same-origin về dev server,
   còn dev server mới là bên proxy sang levelup_ai. Bắt buộc phải vậy —
   levelup_ai không trả CORS header và đáp preflight OPTIONS bằng 405, nên
   gọi thẳng cross-origin từ trình duyệt sẽ bị chặn. */
window.APP_CONFIG = {
  NICHE_API_BASE: "",
  PR_API_BASE: "",
};
