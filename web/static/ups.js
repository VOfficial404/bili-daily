// 选 UP 主：选上的显示成头像、名字和 UID，可以从账号的关注里点选，也可以粘贴 UID 或空间链接。
// 查到的资料记在这里，换页回来不再重查；查不到只提示，不影响保存。

import { api } from "./api.js";
import { h, icon, accountName } from "./dom.js";
import { avatar } from "./accounts.js";
import { modal, handleError } from "./app.js";

const found = new Map(); // UID → 资料，只记查到的
const pending = new Map(); // UID → 正在查的请求，同一个 UID 同时只查一次

// validUID 是规整后的正整数 UID；不是时返回 ""
export function validUID(v) {
  const s = String(v ?? "").trim();
  return /^\d+$/.test(s) && /[1-9]/.test(s) ? s.replace(/^0+/, "") : "";
}

// parseUID 从粘贴的内容里取 UID：纯数字、「UID:123」或 space.bilibili.com/123 这样的空间链接
export function parseUID(v) {
  const s = String(v ?? "").trim();
  const m = s.match(/space\.bilibili\.com\/(\d+)/i) || s.match(/^uid\s*[:：]?\s*(\d+)$/i) || s.match(/^(\d+)$/);
  return m ? validUID(m[1]) : "";
}

// cachedUP 是查到过的资料，没查过或没查到时是 null
export const cachedUP = (mid) => found.get(validUID(mid)) || null;

function remember(p) {
  if (p && p.mid) found.set(p.mid, { mid: p.mid, uname: p.uname || "", face: String(p.face || "").replace(/^\/\//, "https://") });
}

function lookup(mid) {
  let p = pending.get(mid);
  if (!p) {
    p = api.upProfile(mid).then((r) => {
      remember({ ...r, mid });
      return found.get(mid);
    }).finally(() => pending.delete(mid));
    pending.set(mid, p);
  }
  return p;
}

// upPicker 编辑表单里 path 处的 UP 主：single 时是一个 UID 数字（0 表示没选），否则是有先后的 UID 数组。
// 表单改动后调 refresh 跟上；离开页面或重画时调 stop，之后到的结果不再碰页面。
export function upPicker(app, form, path, label, hint, single) {
  const errors = new Map(); // UID → 没查到的原因
  let mids = [];
  let key = null;
  let stopped = false;

  const read = () => {
    const v = form.get(path);
    const list = single ? [v] : v || [];
    return [...new Set(list.map(validUID).filter(Boolean))];
  };
  const write = (next) => form.set(path, single ? Number(next[0] || 0) : next.map(Number));

  const add = (mid) => {
    if (single) write([mid]);
    else if (!mids.includes(mid)) write([...mids, mid]);
  };
  const remove = (mid) => write(mids.filter((m) => m !== mid));
  const move = (i, d) => {
    const next = mids.slice();
    [next[i], next[i + d]] = [next[i + d], next[i]];
    write(next);
  };

  const list = h("ul.up-list");
  const item = (m, i) => {
    const p = found.get(m);
    const err = errors.get(m);
    const actions = h("span.up-actions", null,
      single ? null : [
        h("button.btn.btn-ghost.btn-small", { type: "button", "aria-label": "往前移", title: "往前移", disabled: i === 0, onclick: () => move(i, -1) }, "↑"),
        h("button.btn.btn-ghost.btn-small", { type: "button", "aria-label": "往后移", title: "往后移", disabled: i === mids.length - 1, onclick: () => move(i, 1) }, "↓"),
      ],
      h("button.btn.btn-ghost.btn-small", { type: "button", "aria-label": "移除 UID " + m, title: "移除", onclick: () => remove(m) }, icon("close")),
    );
    return h("li.up-item.up-chosen", { class: err ? "is-error" : null },
      single ? null : h("span.up-rank", null, String(i + 1)),
      avatar(p || { mid: m }, 32),
      h("span.up-text", null,
        p ? h("span.up-name", null, p.uname || "（没有昵称）") : null,
        h("span.up-uid", null, "UID " + m),
        p ? null : h("span.up-note", null, err ? "查不到资料：" + err : "正在查询…"),
      ),
      actions,
    );
  };
  const draw = () => {
    if (stopped) return;
    list.replaceChildren(...(mids.length ? mids.map(item) : [h("li.up-empty.muted.small", null, single ? "还没有选充电对象" : "还没有选，会从关注和排行榜里挑")]));
  };
  const load = () => {
    for (const m of mids) {
      if (found.has(m) || errors.has(m)) continue;
      lookup(m).then(
        () => errors.delete(m),
        (err) => errors.set(m, (err && err.message) || "查询失败"),
      ).then(() => {
        if (mids.includes(m)) draw();
      });
    }
  };

  const input = h("input.input", { id: "up-" + path.join("-"), type: "text", autocomplete: "off", placeholder: "粘贴 UID 或空间链接", "data-k": path.join(".") + ".add" });
  const err = h("p.field-error", { role: "alert" });
  const submit = () => {
    const mid = parseUID(input.value);
    err.textContent = mid ? "" : "填 UID 或 UP 主空间链接，例如 space.bilibili.com/123";
    input.setAttribute("aria-invalid", mid ? "false" : "true");
    if (!mid) return;
    input.value = "";
    add(mid);
  };
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submit();
    }
  });
  input.addEventListener("input", () => {
    err.textContent = "";
    input.setAttribute("aria-invalid", "false");
  });

  const pick = () => pickFollowing(app, single, () => mids, (p) => {
    remember(p);
    if (single) return add(p.mid);
    if (mids.includes(p.mid)) remove(p.mid);
    else add(p.mid);
  });

  const el = h("div.field.up-picker", null,
    h("label.field-label", { for: input.id }, label),
    list,
    h("div.up-add", null,
      h("button.btn.btn-small.btn-primary", { type: "button", "data-k": path.join(".") + ".pick", onclick: pick }, icon("accounts"), "从关注里选"),
      input,
      h("button.btn.btn-small", { type: "button", onclick: submit }, icon("plus"), "添加"),
    ),
    hint ? h("p.field-hint", null, hint) : null,
    err,
  );

  const refresh = () => {
    if (stopped) return;
    const next = read();
    const k = next.join(",");
    if (k === key) return;
    key = k;
    mids = next;
    draw();
    load();
  };
  const stop = () => {
    stopped = true;
  };

  refresh();
  return { el, refresh, stop };
}

// pickFollowing 打开从关注里选 UP 主的对话框。chosen() 是现在选上的 UID；single 时点一个就选上并关掉。
function pickFollowing(app, single, chosen, onPick) {
  const accs = (app.st.accounts || []).filter((a) => a.logged_in);
  let acc = accs.length ? accs[0].mid : "";
  let query = "";
  let pn = 1;
  let items = [];
  let more = false;
  let seq = 0;
  let timer = 0;
  let closed = false;

  const status = h("p.up-pick-status.muted.small", { role: "status" });
  const list = h("ul.up-pick-list");
  const moreBtn = h("button.btn.btn-small", { type: "button", hidden: true, onclick: () => load(false) }, "加载更多");

  const row = (p) => {
    const on = chosen().includes(p.mid);
    return h("li", null, h("button.up-pick", {
      type: "button",
      "aria-pressed": on ? "true" : "false",
      onclick: () => {
        onPick(p);
        if (single) return close();
        draw();
      },
    },
    avatar(p, 32),
    h("span.up-text", null, h("span.up-name", null, p.uname), h("span.up-uid", null, "UID " + p.mid)),
    h("span.up-check", { "aria-hidden": "true" }, on ? "✓" : ""),
    ));
  };
  const draw = () => list.replaceChildren(...items.map(row));

  const load = (reset) => {
    clearTimeout(timer);
    timer = 0;
    if (!acc) return;
    if (reset) {
      pn = 1;
      items = [];
      draw();
    }
    const my = ++seq;
    moreBtn.hidden = true;
    status.className = "up-pick-status muted small";
    status.replaceChildren(query ? `正在搜索「${query}」…` : "正在读取关注…（按请求节奏排队，慢节奏下要等几秒）");
    api.followings(acc, pn, query).then((r) => {
      if (closed || my !== seq) return;
      const list = (r && r.list) || [];
      for (const p of list) remember(p);
      items = items.concat(list.filter((p) => !items.some((x) => x.mid === p.mid)));
      more = Boolean(r && r.more);
      pn++;
      draw();
      moreBtn.hidden = !more;
      status.replaceChildren(items.length ? `共 ${r.total} 个，已列出 ${items.length} 个` : query ? `关注里没有昵称含「${query}」的 UP 主` : "这个账号还没有关注任何人");
    }, (err) => {
      if (closed || my !== seq) return;
      if (err && err.status === 401) return handleError(err);
      status.className = "up-pick-status small is-error";
      status.replaceChildren((err && err.message) || "读取失败", " ",
        h("button.btn.btn-small", { type: "button", onclick: () => load(items.length === 0) }, icon("refresh"), "重试"));
    });
  };

  const search = h("input.input", {
    type: "search",
    placeholder: "按昵称搜索关注",
    autocomplete: "off",
    "aria-label": "按昵称搜索关注",
    oninput: (e) => {
      query = e.target.value.trim();
      clearTimeout(timer);
      timer = setTimeout(() => load(true), 500);
    },
    onkeydown: (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        load(true);
      }
    },
  });

  const close = modal((done) => {
    const content = h("div.up-pick-dialog", null,
      h("h2.dialog-title", null, single ? "选充电对象" : "从关注里选"),
      accs.length > 1
        ? h("select.input", {
          "aria-label": "从哪个账号的关注里选",
          onchange: (e) => {
            acc = e.target.value;
            load(true);
          },
        }, accs.map((a) => h("option", { value: a.mid }, "用「" + accountName(a) + "」的关注")))
        : null,
      accs.length ? search : null,
      accs.length ? status : h("p.dialog-text", null, "先添加并登录一个账号才能从关注里选，也可以直接粘贴 UID。"),
      list,
      moreBtn,
      h("div.dialog-actions", null, h("button.btn", { type: "button", class: single ? null : "btn-primary", onclick: done }, single ? "取消" : "完成")),
    );
    return content;
  }, () => {
    closed = true;
    clearTimeout(timer);
  });
  load(true);
}
