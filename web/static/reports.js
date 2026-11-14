// 报告：每一轮运行留下的报告，按天分组；详情里列出每个任务做了哪几步，失败的写着原因和怎么办。

import { api } from "./api.js";
import { h, mount, icon, statusText, statusGlyph, triggerText, clock, dateLabel, weekday, dayKey, when, duration, accountName } from "./dom.js";
import { handleError } from "./app.js";

const reasonText = {
  not_login: "登录失效",
  risk: "被风控拦截",
  rate: "请求太频繁",
  ineligible: "不满足条件",
  insufficient: "余额不够",
  network: "网络问题",
  changed: "B 站接口有变化",
  canceled: "已中止",
  other: "其他原因",
};

export function render(app, arg) {
  return arg ? detail(app, arg) : list(app);
}

function list(app) {
  const el = h("div");
  const body = h("div.report-list", { "aria-busy": "true" }, h("p.muted", null, "正在加载…"));
  let mid = "";
  let gen = 0;

  const load = async () => {
    const g = ++gen;
    try {
      const runs = await api.runs({ mid, limit: 100 });
      if (g === gen) draw(runs || []);
    } catch (err) {
      if (g !== gen) return;
      mount(body, h("p.form-error", null, "加载报告失败：" + err.message));
      handleError(err);
    }
  };

  const draw = (runs) => {
    body.removeAttribute("aria-busy");
    if (!runs.length) {
      mount(body, h("section.empty", null, h("p", null, "还没有报告。每跑一轮都会留下一份。")));
      return;
    }
    const days = new Map();
    for (const r of runs) {
      const k = dayKey(r.started);
      if (!days.has(k)) days.set(k, []);
      days.get(k).push(r);
    }
    mount(body, [...days.values()].map((rs) => h("section.day", null,
      h("h2.day-title", null, `${dateLabel(rs[0].started)} ${weekday(rs[0].started)}`),
      h("ul.rows", null, rs.map((r) => h("li", null, h("a.row.run-row", { href: "#/reports/" + encodeURIComponent(r.id) },
        h("span.run-time", null, clock(r.started)),
        h("span.glyph", { class: "is-" + r.status, "aria-hidden": "true" }, statusGlyph[r.status]),
        h("span.row-main", null,
          h("span.row-title", null, h("span.row-name", null, r.uname || "UID " + r.mid), h("span.run-status", { class: "is-" + r.status }, statusText[r.status])),
          h("span.row-sub", null, `${triggerText[r.trigger] || r.trigger} · ${r.summary}`),
        ),
        icon("chevron"),
      )))),
    )));
  };

  const filter = h("select.input.input-auto", {
    "aria-label": "只看一个账号",
    onchange: (e) => { mid = e.target.value; load(); },
  }, h("option", { value: "" }, "全部账号"), app.st.accounts.map((a) => h("option", { value: a.mid }, accountName(a))));

  mount(el,
    h("header.page-head", null,
      h("div", null,
        h("h1.page-title", null, "报告"),
        h("p.page-desc", null, "每一轮运行都会留下一份：每个任务做了哪几步，失败的写着原因和怎么办。"),
      ),
      h("div.page-actions", null,
        filter,
        h("button.btn", { type: "button", onclick: load }, icon("refresh"), "刷新"),
      ),
    ),
    body,
  );
  load();

  // 有账号跑完新的一轮时自动刷新列表
  let sig = JSON.stringify(app.st.accounts.map((a) => a.last_run && a.last_run.id));
  return {
    el,
    update() {
      const s = JSON.stringify(app.st.accounts.map((a) => a.last_run && a.last_run.id));
      if (s !== sig) {
        sig = s;
        load();
      }
    },
  };
}

function detail(app, id) {
  const el = h("div");
  const back = h("a.back", { href: "#/reports" }, icon("back"), "所有报告");
  mount(el, back, h("p.muted", null, "正在加载…"));

  api.runDetail(id).then((r) => {
    const ms = new Date(r.ended) - new Date(r.started);
    mount(el,
      back,
      h("header.detail-head", null,
        h("span.big-glyph", { class: "is-" + r.status, "aria-hidden": "true" }, statusGlyph[r.status]),
        h("div.grow", null,
          h("h1.page-title", null, `${r.uname || "UID " + r.mid} · ${statusText[r.status]}`),
          h("p.page-desc", null, `${triggerText[r.trigger] || r.trigger}运行，${when(r.started)} 开始，用时 ${ms > 0 ? duration(ms) : "不到 1 秒"}。${r.summary}`),
        ),
      ),
      (r.tasks || []).map((t) => h("section.panel.report-task", { class: "is-" + t.status },
        h("header.report-task-head", null,
          h("span.glyph", { class: "is-" + t.status, "aria-hidden": "true" }, statusGlyph[t.status]),
          h("h2.section-title.grow", null, t.title),
          h("span.run-status", { class: "is-" + t.status }, statusText[t.status]),
          h("span.muted.small", null, duration(t.millis || 0)),
        ),
        t.note ? h("p.report-note", null, t.note) : null,
        (t.steps || []).length ? h("ol.steps", null, t.steps.map((s) => h("li.step", { class: "is-" + s.status },
          h("span.glyph", { class: "is-" + s.status, "aria-hidden": "true" }, statusGlyph[s.status]),
          h("div.step-main", null,
            h("div.step-line", null, h("span.step-name", null, s.name), s.detail ? h("span.muted", null, s.detail) : null),
            s.status === "failed" || s.status === "partial" ? h("div.step-why", null,
              h("p", null, h("strong", null, reasonText[s.reason] || "失败"), s.code ? `（B 站返回 ${s.code}）` : ""),
              s.hint ? h("p", null, s.hint) : null,
            ) : null,
          ),
        ))) : null,
      )),
    );
  }).catch((err) => {
    mount(el, back, h("section.empty", null, h("p", null, err.status === 404 ? "找不到这份报告，可能已经被清理了。" : "加载失败：" + err.message)));
    if (err.status === 401) handleError(err);
  });
  return { el };
}
