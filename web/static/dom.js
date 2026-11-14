// 建 DOM 的小工具、图标和格式化。所有文字都用 textContent 放进页面，不拼 HTML。

export function h(tag, props, ...children) {
  const [name, ...classes] = tag.split(".");
  const el = document.createElement(name || "div");
  if (classes.length) el.className = classes.join(" ");
  if (props) {
    for (const [k, v] of Object.entries(props)) {
      if (v === undefined || v === null || v === false) continue;
      if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
      else if (k === "class") el.className += (el.className ? " " : "") + v;
      else if (k === "style") Object.assign(el.style, v);
      else if (k in el && typeof v !== "string") el[k] = v;
      else el.setAttribute(k, v === true ? "" : v);
    }
  }
  append(el, children);
  return el;
}

function append(el, children) {
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    if (Array.isArray(c)) append(el, c);
    else el.append(c instanceof Node ? c : String(c));
  }
}

export function mount(el, ...children) {
  el.replaceChildren();
  append(el, children);
}

/* 图标：线条风格，24×24，颜色跟随文字 */

const SVG = "http://www.w3.org/2000/svg";
const paths = {
  today: "M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z",
  accounts: "M16 19v-1a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4v1M9.5 10a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7ZM21 19v-1a4 4 0 0 0-3-3.87M16 3.13a3.5 3.5 0 0 1 0 6.75",
  tasks: "M9 6h11M9 12h11M9 18h11M4 6l1 1 2-2M4 12l1 1 2-2M4 18l1 1 2-2",
  reports: "M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5ZM14 3v5h5M9 13h6M9 17h4",
  settings: "M4 7h10M18 7h2M4 17h2M10 17h10M14 5v4M8 15v4",
  logs: "M4 5h16v14H4zM8 10l2 2-2 2M12 14h4",
  play: "M7 5v14l11-7L7 5Z",
  plus: "M12 5v14M5 12h14",
  refresh: "M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6",
  back: "M15 18l-6-6 6-6",
  chevron: "M9 6l6 6-6 6",
  logout: "M15 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4M10 17l5-5-5-5M15 12H4",
  stop: "M7 7h10v10H7z",
  close: "M6 6l12 12M18 6 6 18",
};

export function icon(name) {
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  svg.classList.add("icon");
  const p = document.createElementNS(SVG, "path");
  p.setAttribute("d", paths[name]);
  svg.append(p);
  return svg;
}

// brand 是界面里的小标记：一台小电视，屏幕里一枚对勾。和应用图标是同一个图案
export function brand() {
  const svg = document.createElementNS(SVG, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  svg.classList.add("brand-mark");
  for (const [tag, attrs] of [
    ["path", { d: "M7.5 3.5l2.5 3.3M16.5 3.5l-2.5 3.3", class: "ant" }],
    ["rect", { x: 2.5, y: 6.5, width: 19, height: 14, rx: 4, class: "body" }],
    ["path", { d: "M8 13.6l2.9 2.7 5.3-5.4", class: "tick" }],
  ]) {
    const el = document.createElementNS(SVG, tag);
    for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
    svg.append(el);
  }
  return svg;
}

export function svgEl(tag, attrs, ...children) {
  const el = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, v);
  for (const c of children) if (c) el.append(c);
  return el;
}

/* 状态 */

export const statusText = {
  ok: "完成",
  partial: "部分完成",
  failed: "失败",
  skipped: "不需要做",
  running: "进行中",
  "": "还没运行",
};

export const statusGlyph = { ok: "✓", partial: "◐", failed: "✕", skipped: "–", running: "●", "": "" };

export const triggerText = { schedule: "定时", manual: "手动", catchup: "补跑" };

/* 时间：B 站按北京时间算每天，界面上的时间也都是北京时间 */

const tz = "Asia/Shanghai";
const fmtTime = new Intl.DateTimeFormat("zh-CN", { timeZone: tz, hour: "2-digit", minute: "2-digit", hour12: false });
const fmtDate = new Intl.DateTimeFormat("zh-CN", { timeZone: tz, month: "long", day: "numeric" });
const fmtWeek = new Intl.DateTimeFormat("zh-CN", { timeZone: tz, weekday: "short" });
const fmtDay = new Intl.DateTimeFormat("en-CA", { timeZone: tz, year: "numeric", month: "2-digit", day: "2-digit" });

export const clock = (t) => (t ? fmtTime.format(new Date(t)) : "");
export const dateLabel = (t) => fmtDate.format(new Date(t));
export const weekday = (t) => fmtWeek.format(new Date(t));
// dayKey 是北京时间的日期，例如 2026-11-14
export const dayKey = (t) => fmtDay.format(new Date(t));

// minuteOfDay 是北京时间一天里的第几分钟
export function minuteOfDay(t) {
  const [hh, mm] = clock(t).split(":").map(Number);
  return (hh % 24) * 60 + mm;
}

// when 是给人看的日期时间：今天只写时刻，其他日子带日期
export function when(t, now) {
  if (!t) return "";
  if (dayKey(t) === dayKey(now || Date.now())) return "今天 " + clock(t);
  return dateLabel(t) + " " + clock(t);
}

export const thousands = (n) => Number(n || 0).toLocaleString("zh-CN");
export const num = (n) => String(Number(n || 0));

export function duration(ms) {
  if (ms < 1000) return (ms / 1000).toFixed(1) + " 秒";
  const s = Math.round(ms / 1000);
  return s < 60 ? s + " 秒" : Math.floor(s / 60) + " 分 " + (s % 60) + " 秒";
}

export const accountName = (acc) => acc.uname || "UID " + acc.mid;

/* 定时刷新 */

// redraw 重画 el 的内容，并把焦点放回带同一个 data-k 的元素上，免得定时刷新打断键盘操作
export function redraw(el, fn) {
  const active = document.activeElement;
  const k = active && el.contains(active) ? active.getAttribute("data-k") : null;
  fn();
  if (k) {
    const again = el.querySelector(`[data-k="${CSS.escape(k)}"]`);
    if (again) again.focus({ preventScroll: true });
  }
}

// live 返回一个页面：sigOf 的结果变了才重画，状态没变时定时刷新不碰页面
export function live(el, sigOf, draw) {
  let sig = "";
  const update = () => {
    const s = JSON.stringify(sigOf());
    if (s === sig) return;
    sig = s;
    redraw(el, draw);
  };
  update();
  return { el, update };
}

// switchRow 是一行文字加一个开关
export function switchRow(key, label, hint, checked, disabled, onChange) {
  const id = "sw-" + key;
  return h("label.switch-row", { for: id, class: disabled ? "is-disabled" : "" },
    h("span.switch-text", null, h("span", null, label), hint ? h("span.muted.small", null, hint) : null),
    h("input.switch", { type: "checkbox", role: "switch", id, "data-k": key, checked, disabled, onchange: (e) => onChange(e.target.checked) }),
  );
}
