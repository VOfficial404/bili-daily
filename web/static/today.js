// 今天：一条 24 小时的时间轴（北京时间）看清每个账号什么时候跑、跑到哪了，下面是每个账号今天各任务的结果。

import { api } from "./api.js";
import { h, mount, icon, svgEl, live, statusText, statusGlyph, clock, dateLabel, weekday, minuteOfDay, dayKey, when, thousands, num, accountName } from "./dom.js";
import { addAccount, avatar, levelBadge } from "./accounts.js";
import { updateNotice } from "./updates.js";

// taskOn 表示设置里这个任务开着（只能手动运行的任务不算）
export function taskOn(s, id) {
  const t = s.tasks[id];
  return Boolean(t && id !== "unfollow" && t.enabled);
}

// todayPlan 列出账号今天要做的任务，和今天已经有的结果对上
export function todayPlan(st, acc) {
  const done = new Map((acc.today || []).map((s) => [s.id, s]));
  const out = [];
  for (const t of st.tasks) {
    const ran = done.get(t.id);
    const planned = !t.manual && taskOn(st.settings, t.id) && !(acc.disabled_tasks || []).includes(t.id);
    if (!planned && !ran) continue;
    out.push({ id: t.id, title: t.title, status: ran ? ran.status : "", note: ran ? ran.note : "" });
  }
  return out;
}

const finished = (s) => s === "ok" || s === "skipped";

export function render(app) {
  const el = h("div.today");
  const st = () => app.st;
  return live(el, () => [st().accounts, st().tasks, st().settings, st().releases, clock(st().now), [...app.busy]], () => draw(app, el));
}

function draw(app, el) {
  const st = app.st;
  const now = st.now || new Date().toISOString();
  let total = 0, done = 0, problems = 0;
  for (const acc of st.accounts) {
    for (const l of todayPlan(st, acc)) {
      total++;
      if (finished(l.status)) done++;
      if (l.status === "failed" || l.status === "partial") problems++;
    }
  }
  let summary;
  if (!st.accounts.length) summary = "还没有账号。扫码添加一个，之后每天会自动跑。";
  else if (problems) summary = `${st.accounts.length} 个账号，今天 ${total} 项任务完成了 ${done} 项，${problems} 项有问题。`;
  else if (total && done === total) summary = `${st.accounts.length} 个账号，今天的 ${total} 项任务都做完了。`;
  else summary = `${st.accounts.length} 个账号，今天 ${total} 项任务完成了 ${done} 项。`;

  mount(el,
    h("header.page-head", null,
      h("div", null,
        h("h1.page-title", null, `${dateLabel(now)} ${weekday(now)}`),
        h("p.page-desc", { class: problems ? "has-problem" : "" }, summary),
      ),
      h("div.page-actions", null,
        h("button.btn", { type: "button", "data-k": "add", disabled: app.busy.has("qr"), onclick: () => addAccount(app) }, icon("plus"), "添加账号"),
        st.accounts.length ? h("button.btn.btn-primary", {
          type: "button",
          "data-k": "all",
          disabled: app.busy.has("all"),
          onclick: () => app.act("all", "已开始给所有账号跑一轮", () => api.runAll()),
        }, icon("play"), "全部立即运行") : null,
      ),
    ),
    updateNotice(st),
    st.accounts.length ? timeline(st, now) : empty(app),
    h("div.cards", null, st.accounts.map((acc) => card(app, acc, now))),
  );
}

function empty(app) {
  return h("section.empty", null,
    h("p", null, "用 B 站 App 扫一下码就能添加账号。添加后会马上跑第一轮，以后每天到点自动跑。"),
    h("button.btn.btn-primary", { type: "button", onclick: () => addAccount(app) }, icon("plus"), "添加账号"),
  );
}

/* 时间轴：横轴是北京时间的一天，按百分比摆放，圆点在任何宽度下都是圆的。粉色带是每天开始运行的时间段 */

function timeline(st, now) {
  const laneH = 26, top = 22;
  const H = top + st.accounts.length * laneH + 6;
  const [hh, mm] = (st.settings.run_at || "09:30").split(":").map(Number);
  const start = hh * 60 + mm;
  const end = Math.min(1439, start + (st.settings.jitter || 0));
  const pct = (m) => (m / 1440) * 100;
  const P = (m) => pct(m) + "%";
  const today = dayKey(now);

  const svg = svgEl("svg", { width: "100%", height: H, class: "tl-svg", role: "img", "aria-label": `每天 ${st.settings.run_at} 开始运行，随机推迟最多 ${st.settings.jitter} 分钟` });
  svg.append(svgEl("rect", { x: P(start), y: 0, width: Math.max(0.3, pct(end) - pct(start)) + "%", height: H, class: "tl-window" }));
  for (let hr = 0; hr <= 24; hr += 3) svg.append(svgEl("line", { x1: P(hr * 60), x2: P(hr * 60), y1: top - 4, y2: H, class: "tl-grid" }));
  st.accounts.forEach((acc, i) => {
    const y = top + i * laneH + laneH / 2;
    svg.append(svgEl("line", { x1: 0, x2: "100%", y1: y, y2: y, class: "tl-lane" }));
    const last = acc.last_run;
    if (last && dayKey(last.started) === today) {
      const a = minuteOfDay(last.started);
      const ended = last.ended && new Date(last.ended) > new Date(last.started) ? minuteOfDay(last.ended) : a;
      const b = Math.max(a + 8, ended);
      svg.append(svgEl("rect", { x: P(a), y: y - 5, width: pct(b) - pct(a) + "%", height: 10, rx: 5, class: "tl-run is-" + last.status }));
    }
    if (acc.running) svg.append(svgEl("circle", { cx: P(minuteOfDay(acc.running.started)), cy: y, r: 6, class: "tl-run is-running" }));
    if (acc.next_run && dayKey(acc.next_run) === today) svg.append(svgEl("circle", { cx: P(minuteOfDay(acc.next_run)), cy: y, r: 5, class: "tl-next" }));
  });
  svg.append(svgEl("line", { x1: P(minuteOfDay(now)), x2: P(minuteOfDay(now)), y1: 0, y2: H, class: "tl-now" }));

  const hours = h("div.tl-hours", { "aria-hidden": "true" });
  for (let hr = 0; hr <= 24; hr += 3) hours.append(h("span", { style: { left: (hr / 24) * 100 + "%" } }, String(hr).padStart(2, "0")));
  const names = h("div.tl-names", { "aria-hidden": "true" }, st.accounts.map((acc) => h("span", null, accountName(acc))));

  return h("section.timeline", { "aria-label": "今天的时间轴（北京时间）" },
    names,
    h("div.tl-body", null, hours, svg),
    h("p.tl-legend", null,
      h("span.lg-item", null, h("span.lg.lg-window"), `每天 ${st.settings.run_at} 起，随机推迟 0~${st.settings.jitter} 分钟`),
      h("span.lg-item", null, h("span.lg.lg-run"), "今天跑过的一轮"),
      h("span.lg-item", null, h("span.lg.lg-next"), "下次运行"),
      h("span.lg-item", null, h("span.lg.lg-now"), "现在 " + clock(now)),
    ),
  );
}

/* 账号卡片 */

function card(app, acc, now) {
  const lines = todayPlan(app.st, acc);
  const done = lines.filter((l) => finished(l.status)).length;
  const busy = app.busy.has(acc.mid);
  const run = acc.running;

  let foot;
  if (run) {
    foot = h("div.card-running", null,
      h("span.spinner", { "aria-hidden": "true" }),
      h("span.grow", null, run.total ? `正在做：${run.task}（${run.done}/${run.total}）` : `正在做：${run.task || "准备中"}`),
      h("button.btn.btn-small", { type: "button", "data-k": "cancel-" + acc.mid, disabled: busy, onclick: () => app.act(acc.mid, "已请求中止", () => api.cancel(acc.mid)) }, icon("stop"), "中止"),
    );
  } else {
    foot = h("div.card-foot", null,
      h("span.muted.grow", null, !acc.enabled ? "没参加每日计划" : acc.next_run ? "下次运行 " + when(acc.next_run, now) : ""),
      acc.last_run ? h("a.link", { href: "#/reports/" + encodeURIComponent(acc.last_run.id) }, "上一轮报告") : null,
      h("button.btn.btn-small", {
        type: "button",
        "data-k": "run-" + acc.mid,
        disabled: busy || !acc.logged_in,
        onclick: () => app.act(acc.mid, "已开始运行", () => api.run(acc.mid)),
      }, icon("play"), "立即运行"),
    );
  }

  const exp = acc.next_exp ? h("div.exp", { title: `经验 ${thousands(acc.exp)} / ${thousands(acc.next_exp)}` },
    h("div.exp-bar", null, h("div.exp-fill", { style: { width: Math.min(100, (acc.exp / acc.next_exp) * 100) + "%" } })),
    h("span.muted", null, `${thousands(acc.exp)} / ${thousands(acc.next_exp)}`),
  ) : null;

  return h("article.card", null,
    h("header.card-head", null,
      avatar(acc, 44),
      h("div.grow", null,
        h("div.card-title", null,
          h("a.card-name", { href: "#/accounts/" + encodeURIComponent(acc.mid) }, accountName(acc)),
          levelBadge(acc.level),
          acc.vip ? h("span.pill", null, acc.vip) : null,
        ),
        h("p.muted.small", null, `硬币 ${num(acc.coins)} · B 币券 ${num(acc.b_coupon)}`),
        exp,
      ),
      h("div.card-count", { title: `今天完成 ${done} / ${lines.length}` }, h("strong", null, String(done)), h("span", null, "/" + lines.length)),
    ),
    !acc.logged_in ? h("div.alert.is-error", null,
      h("span.grow", null, "登录已失效，任务暂停了。重新扫码就能恢复。"),
      h("button.btn.btn-small.btn-primary", { type: "button", onclick: () => addAccount(app) }, "重新扫码"),
    ) : null,
    lines.length ? h("ul.task-list", null, lines.map((l) => h("li.task-row", { class: "is-" + (l.status || "pending") },
      h("span.glyph", { "aria-hidden": "true" }, statusGlyph[l.status] || "·"),
      h("span.task-title", null, l.title),
      h("span.task-note", { title: l.note || "" }, l.note || statusText[l.status]),
    ))) : h("p.card-empty.muted", null, "今天没有要做的任务。"),
    foot,
  );
}
