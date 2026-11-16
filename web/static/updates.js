// 新版本提示：服务端每隔几小时查一次 GitHub 上的发布，比正在运行的版本新的标着 newer。

import { h, dateLabel } from "./dom.js";

// newer 是比服务端新的版本，从新到旧
const newer = (st) => (st.releases || []).filter((r) => r.newer);

// 「今天」页每分钟重画一次，展开过的提示保持展开
let open = false;

// notes 把发布说明（Markdown 列表）变成列表；没写时指向发布页
function notes(r) {
  const lines = String(r.notes || "").split("\n").map((l) => l.trim()).filter(Boolean);
  if (!lines.length) return h("p.muted.small", null, "这个版本没有写更新说明，详见发布页。");
  const items = [];
  let list = null;
  for (const l of lines) {
    const m = l.match(/^[-*]\s+(.*)$/);
    if (m) {
      if (!list) items.push(list = h("ul.update-list"));
      list.append(h("li", null, m[1]));
    } else {
      list = null;
      items.push(h("p.small", null, l.replace(/^#+\s*/, "")));
    }
  }
  return items;
}

// updateNotice 是「今天」页顶上的提示，没有新版本时为 null
export function updateNotice(st) {
  const list = newer(st);
  if (!list.length) return null;
  const latest = list[0];
  return h("details.update", { "data-k": "update", open, ontoggle: (e) => { open = e.target.open; } },
    h("summary", null,
      h("strong", null, "有新版本 " + latest.version),
      h("span.muted", null, `正在运行 ${st.version}，点开看更新了什么`),
    ),
    list.map((r) => h("section.update-release", null,
      h("h3.update-version", null, r.version, r.published_at ? h("span.muted.small", null, " · " + dateLabel(r.published_at)) : null),
      notes(r),
    )),
    st.update_hint ? h("p.update-hint", null, "更新方法：", h("code", null, st.update_hint)) : null,
    h("a.btn.btn-small", { href: latest.url, target: "_blank", rel: "noopener noreferrer" }, "打开发布页"),
  );
}

// updateStatus 是设置页「版本」后面的说明；还没查到发布时为空
export function updateStatus(st) {
  if (!(st.releases || []).length) return "";
  const list = newer(st);
  return list.length ? `有新版本 ${list[0].version}` : "已是最新版本";
}
