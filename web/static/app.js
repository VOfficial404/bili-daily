// 面板的外框：登录、导航、定时取状态、提示和确认框。各页面在自己的文件里。

import { api, token, APIError } from "./api.js";
import { h, mount, icon, brand } from "./dom.js";
import * as today from "./today.js";
import * as accounts from "./accounts.js";
import * as tasks from "./tasks.js";
import * as reports from "./reports.js";
import * as settings from "./settings.js";
import * as logs from "./logs.js";
import { resetForm, hasUnsaved } from "./form.js";

const pages = [
  { id: "today", title: "今天", icon: "today", mod: today },
  { id: "accounts", title: "账号", icon: "accounts", mod: accounts },
  { id: "tasks", title: "任务", icon: "tasks", mod: tasks },
  { id: "reports", title: "报告", icon: "reports", mod: reports },
  { id: "settings", title: "设置", icon: "settings", mod: settings },
  { id: "logs", title: "日志", icon: "logs", mod: logs },
];

const root = document.getElementById("app");

export const app = {
  st: null, // 最近一次 GET /api/state
  busy: new Set(), // 正在提交的操作，按钮据此禁用
  route: { page: "today", arg: "" },
  view: null, // 当前页面：{ el, update?, leave? }

  go(path) {
    location.hash = "#/" + path;
  },

  // act 执行一个操作：期间 key 对应的按钮禁用，完成后提示并刷新状态
  async act(key, done, fn) {
    if (this.busy.has(key)) return;
    this.busy.add(key);
    this.update();
    try {
      await fn();
      if (done) toast(done);
      await poll();
    } catch (err) {
      handleError(err);
    } finally {
      this.busy.delete(key);
      this.update();
    }
  },

  // update 让当前页面按最新状态重画能变的部分
  update() {
    if (this.view && this.view.update) this.view.update();
    renderChrome();
  },

  title(id) {
    const t = this.st && this.st.tasks.find((t) => t.id === id);
    return t ? t.title : id;
  },

  account(mid) {
    return this.st && this.st.accounts.find((a) => a.mid === mid);
  },
};

/* 提示 */

const toasts = h("div.toasts", { role: "status", "aria-live": "polite" });

export function toast(text, kind) {
  const el = h("div.toast", { class: kind === "error" ? "is-error" : "" }, text);
  toasts.append(el);
  setTimeout(() => el.remove(), kind === "error" ? 7000 : 3500);
}

function handleError(err) {
  if (err instanceof APIError && err.status === 401) {
    token.clear();
    showLogin("令牌不对或已更换，请重新登录。");
    return;
  }
  toast(err.message || String(err), "error");
}
export { handleError, poll as refresh };

/* 确认框 */

// confirm 打开一个确认框，用户确认时返回 true
export function confirm({ title, text, ok, danger }) {
  return new Promise((resolve) => {
    const dlg = h("dialog.dialog", { "aria-label": title },
      h("h2.dialog-title", null, title),
      h("p.dialog-text", null, text),
      h("div.dialog-actions", null,
        h("button.btn", { type: "button", onclick: () => close(false) }, "取消"),
        h("button.btn", { type: "button", class: danger ? "btn-danger" : "btn-primary", onclick: () => close(true) }, ok),
      ),
    );
    const close = (v) => {
      dlg.close();
      dlg.remove();
      resolve(v);
    };
    dlg.addEventListener("cancel", (e) => {
      e.preventDefault();
      close(false);
    });
    document.body.append(dlg);
    dlg.showModal();
  });
}

// modal 打开一个自定义内容的对话框，返回关闭它的函数
export function modal(build, onClose) {
  const dlg = h("dialog.dialog");
  const close = () => {
    if (!dlg.isConnected) return;
    dlg.close();
    dlg.remove();
    if (onClose) onClose();
  };
  dlg.addEventListener("cancel", (e) => {
    e.preventDefault();
    close();
  });
  mount(dlg, build(close));
  document.body.append(dlg);
  dlg.showModal();
  return close;
}

/* 登录 */

function showLogin(message) {
  stopPolling();
  resetForm();
  app.st = null;
  const input = h("input.input", { id: "login-token", type: "password", autocomplete: "current-password", required: true, placeholder: "粘贴令牌" });
  const remember = h("input", { type: "checkbox", id: "login-remember" });
  const error = h("p.form-error", { role: "alert" }, message || "");
  const submit = h("button.btn.btn-primary.btn-wide", { type: "submit" }, "登录");
  const form = h("form.login-card", {
    onsubmit: async (e) => {
      e.preventDefault();
      submit.disabled = true;
      error.textContent = "";
      token.set(input.value.trim(), remember.checked);
      try {
        app.st = await api.state();
        start();
      } catch (err) {
        token.clear();
        error.textContent = err.status === 401 ? "令牌不对。在服务器上运行 bili-daily token 查看。" : err.message;
        submit.disabled = false;
        input.focus();
      }
    },
  },
    h("div.login-brand", null, brand(), h("span", null, "Bili Daily")),
    h("h1.login-title", null, "每天替你做完 B 站的日常任务"),
    h("p.muted", null, "输入服务端的令牌登录。令牌在服务器上用 bili-daily token 查看，Docker 里是 docker compose exec bili-daily bili-daily token。"),
    h("label.field", { for: "login-token" }, h("span.field-label", null, "令牌"), input),
    h("label.check", null, remember, h("span", null, "在这个浏览器上记住")),
    error,
    submit,
  );
  mount(root, h("main.login", null, form));
  input.focus();
}

/* 外框 */

const shell = {
  nav: null,
  status: null,
  main: null,
  tabbar: null,
};

function buildShell() {
  shell.nav = h("nav.nav", { "aria-label": "页面" });
  shell.tabbar = h("nav.tabbar", { "aria-label": "页面" });
  shell.status = h("div.conn");
  shell.main = h("main.page", { id: "main", tabindex: "-1" });
  mount(root,
    h("a.skip", { href: "#main", onclick: (e) => { e.preventDefault(); shell.main.focus(); } }, "跳到内容"),
    h("header.topbar", null,
      h("div.topbar-inner", null,
        h("a.wordmark", { href: "#/today" }, brand(), h("span", null, "Bili Daily")),
        shell.nav,
        shell.status,
      ),
    ),
    shell.main,
    shell.tabbar,
  );
}

function renderChrome() {
  if (!shell.nav) return;
  const links = (cls) => pages.map((p) => h("a", {
    class: cls,
    href: "#/" + p.id,
    "aria-current": app.route.page === p.id ? "page" : false,
  }, icon(p.icon), h("span", null, p.title)));
  mount(shell.nav, links("nav-link"));
  mount(shell.tabbar, links("tab"));
  const st = app.st;
  mount(shell.status,
    h("span.conn-text", { title: st ? "服务端版本 " + st.version : "" }, h("span.dot", { class: lostAt ? "is-bad" : "is-ok" }), lostAt ? "连接中断，正在重试" : "已连接"),
    h("button.btn.btn-ghost.btn-icon", { type: "button", title: "退出登录", "aria-label": "退出登录", onclick: logout }, icon("logout")),
  );
}

async function logout() {
  const ok = await confirm({ title: "退出登录？", text: "这只会让这个浏览器忘记令牌；服务器上的任务照常运行。", ok: "退出登录" });
  if (!ok) return;
  token.clear();
  showLogin();
}

/* 路由 */

function parseRoute() {
  const [page, ...rest] = location.hash.replace(/^#\/?/, "").split("/");
  return { page: pages.some((p) => p.id === page) ? page : "today", arg: decodeURIComponent(rest.join("/")) };
}

async function renderPage() {
  if (!app.st) return;
  if (app.view && app.view.leave) app.view.leave();
  app.route = parseRoute();
  const page = pages.find((p) => p.id === app.route.page);
  document.title = page.title + " - Bili Daily";
  app.view = page.mod.render(app, app.route.arg);
  mount(shell.main, app.view.el);
  renderChrome();
  window.scrollTo(0, 0);
}

/* 状态轮询：每 4 秒取一次，出错时在右上角显示连接中断 */

let timer = 0;
let lostAt = 0;

async function poll() {
  try {
    app.st = await api.state();
    if (lostAt) {
      lostAt = 0;
      toast("已重新连上服务器");
    }
    app.update();
  } catch (err) {
    if (err instanceof APIError && err.status === 401) return handleError(err);
    if (!lostAt) {
      lostAt = Date.now();
      renderChrome();
    }
  }
}

function startPolling() {
  stopPolling();
  timer = setInterval(() => {
    if (!document.hidden) poll();
  }, 4000);
}

function stopPolling() {
  clearInterval(timer);
  timer = 0;
}

function start() {
  buildShell();
  renderPage();
  startPolling();
}

document.addEventListener("visibilitychange", () => {
  if (!document.hidden && timer) poll();
});
window.addEventListener("hashchange", renderPage);
// 设置改了没保存就关页面时，让浏览器先问一句
window.addEventListener("beforeunload", (e) => {
  if (hasUnsaved()) e.preventDefault();
});
document.body.append(toasts);

(async () => {
  if (!token.get()) return showLogin();
  try {
    app.st = await api.state();
    start();
  } catch (err) {
    if (err.status === 401) token.clear();
    showLogin(err.status === 401 ? "" : err.message);
  }
})();
