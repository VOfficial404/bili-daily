// 账号：列表、单个账号的详情（资料、开关、操作），以及扫码添加账号。

import { api } from "./api.js";
import { h, mount, icon, live, switchRow, num, thousands, when, dayKey, accountName } from "./dom.js";
import { modal, confirm, handleError, refresh, toast } from "./app.js";
import { taskOn } from "./today.js";

// levelBadge 沿用 B 站自己的等级配色
export function levelBadge(level) {
  const l = Math.max(0, Math.min(6, level || 0));
  return h("span", { class: "lv lv-" + l, title: `等级 ${l}` }, "LV" + l);
}

// avatar 是头像；没有头像或加载失败时用名字的第一个字
export function avatar(acc, size) {
  const box = h("span.avatar", { style: { width: size + "px", height: size + "px", fontSize: Math.round(size * 0.42) + "px" }, "aria-hidden": "true" });
  const initial = () => box.replaceChildren(Array.from(accountName(acc))[0] || "?");
  const face = (acc.face || "").replace(/^http:/, "https:");
  if (face.startsWith("https://")) {
    const img = h("img", { src: face, alt: "", referrerpolicy: "no-referrer", loading: "lazy", decoding: "async" });
    img.addEventListener("error", initial, { once: true });
    box.append(img);
  } else {
    initial();
  }
  return box;
}

function loginLeft(acc) {
  if (!acc.cookie_expires) return "登录有效期未知";
  const days = Math.floor((new Date(acc.cookie_expires) - Date.now()) / 864e5);
  return days > 0 ? `登录还有 ${days} 天` : "登录今天到期";
}

/* 扫码添加账号 */

export function addAccount(app) {
  if (app.busy.has("qr")) return;
  app.busy.add("qr");
  let timer = 0, id = "";
  const img = h("img.qr-img", { alt: "B 站登录二维码", width: 200, height: 200 });
  const box = h("div.qr-box", null, img);
  const state = h("p.qr-state", { role: "status" });
  const retry = h("button.btn.btn-primary", { type: "button", hidden: true, onclick: () => load() }, "换一张");
  const setState = (text, kind) => {
    state.textContent = text;
    state.className = "qr-state" + (kind ? " is-" + kind : "");
  };
  const close = modal((close) => h("div.qr", null,
    h("h2.dialog-title", null, "添加 B 站账号"),
    h("p.muted", null, "打开 B 站 App，点右上角的「扫一扫」扫这个码，再在手机上确认。已经添加过的账号会更新登录。"),
    box,
    state,
    h("div.dialog-actions", null, retry, h("button.btn", { type: "button", onclick: () => close() }, "关闭")),
  ), () => {
    clearTimeout(timer);
    app.busy.delete("qr");
    app.update();
  });

  async function load() {
    clearTimeout(timer);
    retry.hidden = true;
    box.classList.remove("is-expired");
    setState("正在获取二维码…");
    try {
      const q = await api.qrStart();
      id = q.id;
      img.src = "data:image/png;base64," + q.png;
      setState("等待扫码", "waiting");
      timer = setTimeout(check, 2000);
    } catch (err) {
      if (err.status === 401) return close(), handleError(err);
      setState("获取二维码失败：" + err.message, "error");
      retry.hidden = false;
    }
  }

  async function check() {
    try {
      const q = await api.qrPoll(id);
      if (!img.isConnected) return;
      switch (q.state) {
        case "scanned":
          setState("已扫码，请在手机上确认", "scanned");
          break;
        case "expired":
          setState("二维码过期了，换一张再扫", "expired");
          box.classList.add("is-expired");
          retry.hidden = false;
          return;
        case "success":
          close();
          toast("账号已添加，马上开始第一轮");
          refresh();
          return;
        default:
          setState("等待扫码", "waiting");
      }
    } catch (err) {
      if (err.status === 401) return close(), handleError(err);
      if (!img.isConnected) return;
      setState("查询扫码状态失败：" + err.message + "，正在重试", "error");
    }
    timer = setTimeout(check, 2000);
  }

  app.update();
  load();
}

/* 页面 */

export function render(app, arg) {
  return arg ? detail(app, arg) : list(app);
}

function list(app) {
  const el = h("div");
  return live(el, () => [app.st.accounts, app.busy.has("qr")], () => {
    const accs = app.st.accounts;
    mount(el,
      h("header.page-head", null,
        h("div", null,
          h("h1.page-title", null, "账号"),
          h("p.page-desc", null, "每个账号的登录状态、等级和资产。点进去可以单独关掉某些任务。"),
        ),
        h("div.page-actions", null,
          h("button.btn.btn-primary", { type: "button", "data-k": "add", disabled: app.busy.has("qr"), onclick: () => addAccount(app) }, icon("plus"), "添加账号"),
        ),
      ),
      accs.length
        ? h("ul.rows", null, accs.map((acc) => h("li", null, h("a.row", { href: "#/accounts/" + encodeURIComponent(acc.mid), "data-k": "acc-" + acc.mid },
          avatar(acc, 40),
          h("span.row-main", null,
            h("span.row-title", null, h("span.row-name", null, accountName(acc)), levelBadge(acc.level), acc.vip ? h("span.pill", null, acc.vip) : null),
            h("span.row-sub", { class: acc.logged_in ? "" : "is-error" },
              acc.logged_in ? `硬币 ${num(acc.coins)} · B 币券 ${num(acc.b_coupon)} · ${loginLeft(acc)}` : "登录已失效，需要重新扫码"),
          ),
          !acc.enabled ? h("span.pill.pill-muted", null, "不参加每日计划") : null,
          icon("chevron"),
        ))))
        : h("section.empty", null,
          h("p", null, "还没有账号。用 B 站 App 扫码添加，添加后马上跑第一轮。"),
          h("button.btn.btn-primary", { type: "button", onclick: () => addAccount(app) }, icon("plus"), "添加账号"),
        ),
    );
  });
}

const back = () => h("a.back", { href: "#/accounts" }, icon("back"), "所有账号");

function detail(app, mid) {
  const el = h("div");
  return live(el, () => [app.account(mid), app.st.tasks, app.st.settings.tasks, app.busy.has(mid), app.busy.has("qr")], () => {
    const acc = app.account(mid);
    if (!acc) {
      mount(el, back(), h("section.empty", null, h("p", null, "找不到这个账号，可能已经退出了。")));
      return;
    }
    const busy = app.busy.has(mid);
    const vip = acc.vip ? acc.vip + (acc.vip_due ? `，${dayKey(acc.vip_due)} 到期` : "") : "不是大会员";
    const days = acc.cookie_expires ? Math.floor((new Date(acc.cookie_expires) - Date.now()) / 864e5) : null;
    const cookie = days === null ? "有效期未知" : `还有 ${Math.max(0, days)} 天（${dayKey(acc.cookie_expires)}），会自动续期`;
    const fact = (k, v) => h("div.fact", null, h("dt", null, k), h("dd", null, v));
    const disabled = acc.disabled_tasks || [];

    mount(el,
      back(),
      h("header.detail-head", null,
        avatar(acc, 64),
        h("div.grow", null,
          h("h1.page-title", null, accountName(acc)),
          h("p.detail-badges", null, levelBadge(acc.level), acc.vip ? h("span.pill", null, acc.vip) : null, h("span.muted", null, "UID " + acc.mid)),
        ),
      ),
      !acc.logged_in ? h("div.alert.is-error.alert-block", null,
        h("span.grow", null, "登录已失效，这个账号的任务暂停了。重新扫码登录同一个账号就能恢复。"),
        h("button.btn.btn-small.btn-primary", { type: "button", disabled: app.busy.has("qr"), onclick: () => addAccount(app) }, "重新扫码"),
      ) : null,
      h("section.panel", null,
        h("h2.section-title", null, "资料"),
        h("dl.facts", null,
          fact("经验", acc.next_exp ? `${thousands(acc.exp)} / ${thousands(acc.next_exp)}` : "已满级"),
          fact("硬币", num(acc.coins)),
          fact("B 币券", num(acc.b_coupon)),
          fact("大会员", vip),
          fact("登录", cookie),
          fact("资料更新于", acc.updated_at ? when(acc.updated_at) : "暂无"),
        ),
      ),
      h("section.panel", null,
        h("h2.section-title", null, "每日计划"),
        switchRow("plan", "参加每日计划", "关掉后不会每天定时运行，手动运行不受影响。", acc.enabled, busy,
          (v) => app.act(mid, v ? "已加入每日计划" : "已退出每日计划", () => api.patch(mid, { enabled: v }))),
        h("h3.sub-title", null, "这个账号做的任务"),
        h("p.muted.small", null, "任务的总开关和选项在「任务」页，这里只能给这个账号单独关掉。"),
        h("div.switch-grid", null, app.st.tasks.filter((t) => !t.manual).map((t) => {
          const global = taskOn(app.st.settings, t.id);
          const on = global && !disabled.includes(t.id);
          const hint = !global ? "「任务」页里关着" : t.needs ? "需要" + t.needs : "";
          return switchRow("task-" + t.id, t.title, hint, on, busy || !global, (v) => {
            const next = disabled.filter((x) => x !== t.id);
            if (!v) next.push(t.id);
            app.act(mid, "账号任务已更新", () => api.patch(mid, { disabled_tasks: next }));
          });
        })),
      ),
      h("div.actions", null,
        h("button.btn.btn-primary", { type: "button", "data-k": "run", disabled: busy || !acc.logged_in || Boolean(acc.running), onclick: () => app.act(mid, "已开始运行", () => api.run(mid)) }, icon("play"), acc.running ? "正在运行" : "立即运行"),
        h("button.btn", { type: "button", "data-k": "refresh", disabled: busy, onclick: () => app.act(mid, "资料已刷新", () => api.refresh(mid)) }, icon("refresh"), "刷新资料"),
        h("span.spacer"),
        h("button.btn.btn-danger", { type: "button", "data-k": "logout", disabled: busy, onclick: () => logout(app, acc) }, icon("logout"), "退出账号"),
      ),
    );
  });
}

async function logout(app, acc) {
  const ok = await confirm({
    title: `退出「${accountName(acc)}」？`,
    text: "会在 B 站注销这份登录，并删除服务器上这个账号的数据和报告。之后要用需要重新扫码。",
    ok: "退出账号",
    danger: true,
  });
  if (!ok) return;
  app.act(acc.mid, "已退出账号", async () => {
    await api.remove(acc.mid);
    app.go("accounts");
  });
}
