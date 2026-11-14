// UP 主预览：按填的 UID 显示头像、名字和 UID，免得填错人。
// 查到的资料记在这里，换页回来不再重查；查不到只提示，不影响保存。

import { api } from "./api.js";
import { h } from "./dom.js";
import { avatar } from "./accounts.js";

const found = new Map(); // UID → 资料，只记查到的
const pending = new Map(); // UID → 正在查的请求，同一个 UID 同时只查一次

// validUID 是规整后的正整数 UID；不是时返回 ""
export function validUID(v) {
  const s = String(v ?? "").trim();
  return /^\d+$/.test(s) && /[1-9]/.test(s) ? s.replace(/^0+/, "") : "";
}

// cachedUP 是查到过的资料，没查过或没查到时是 null
export const cachedUP = (mid) => found.get(validUID(mid)) || null;

function lookup(mid) {
  let p = pending.get(mid);
  if (!p) {
    p = api.upProfile(mid).then((r) => {
      const prof = { mid, uname: (r && r.uname) || "", face: String((r && r.face) || "").replace(/^\/\//, "https://") };
      found.set(mid, prof);
      return prof;
    }).finally(() => pending.delete(mid));
    pending.set(mid, p);
  }
  return p;
}

// upPreview 显示 read() 给出的那些 UID 对应的 UP 主。表单改动后调 refresh，停顿 500 毫秒才去查；
// 离开页面或重画时调 stop，之后到的结果不再碰页面。
export function upPreview(read) {
  const el = h("ul.up-list", { "aria-live": "polite", hidden: true });
  const errors = new Map(); // UID → 这次没查到的原因
  let items = new Map(); // UID → { el, sig }，状态没变的行不重建，头像不闪
  let mids = [];
  let key = null;
  let timer = 0;
  let stopped = false;

  const sigOf = (m) => (found.has(m) ? "ok" : errors.has(m) ? "err" : "wait");

  const item = (m) => {
    const p = found.get(m);
    if (p) {
      return h("li.up-item", null, avatar(p, 28),
        h("span.up-name", null, p.uname || "（没有昵称）"),
        h("span.up-uid", null, "UID " + m));
    }
    const err = errors.get(m);
    return h("li.up-item", { class: err ? "is-error" : null }, avatar({ mid: m }, 28),
      h("span.up-uid", null, "UID " + m),
      h("span.up-note", null, err ? "查不到资料：" + err : "正在查询…"));
  };

  const draw = () => {
    if (stopped) return;
    const next = new Map();
    for (const m of mids) {
      const sig = sigOf(m);
      const old = items.get(m);
      next.set(m, old && old.sig === sig ? old : { el: item(m), sig });
    }
    items = next;
    el.replaceChildren(...[...next.values()].map((x) => x.el));
    el.hidden = !mids.length;
  };

  const load = () => {
    timer = 0;
    for (const m of mids) {
      if (found.has(m)) continue;
      errors.delete(m);
      lookup(m).then(
        () => errors.delete(m),
        (err) => errors.set(m, (err && err.message) || "查询失败"),
      ).then(() => {
        // 填的 UID 已经改掉了，就不管这个结果
        if (mids.includes(m)) draw();
      });
    }
    draw();
  };

  // refresh 读一遍现在的 UID；now 为 true 时马上查（打开页面时），否则等输入停顿
  const refresh = (now) => {
    if (stopped) return;
    const next = [...new Set((read() || []).map(validUID).filter(Boolean))];
    const k = next.join(",");
    if (k === key) return;
    key = k;
    mids = next;
    clearTimeout(timer);
    timer = 0;
    if (!mids.some((m) => !found.has(m))) return draw();
    if (now) return load();
    draw();
    timer = setTimeout(load, 500);
  };

  const stop = () => {
    stopped = true;
    clearTimeout(timer);
    timer = 0;
  };

  refresh(true);
  return { el, refresh, stop };
}
