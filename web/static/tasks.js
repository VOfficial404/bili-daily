// 任务：每个任务的总开关和选项（所有账号共用），以及「运行一次」。

import { api } from "./api.js";
import { h, mount, icon, accountName } from "./dom.js";
import { modal, confirm, toast, handleError } from "./app.js";
import { sharedForm, toggle, number, text, list, saveBar } from "./form.js";

// 有选项的任务；其余只有开关
const withOptions = new Set(["daily", "charge", "manga", "fans_medal", "live_lottery"]);

export function render(app) {
  const form = sharedForm(app);
  const el = h("div");
  const body = h("div.task-groups");
  const runButtons = [];

  const syncRun = () => {
    for (const b of runButtons) b.disabled = form.dirty || form.saving || app.busy.has("run");
    for (const n of el.querySelectorAll(".run-hint")) n.hidden = !form.dirty;
  };

  const build = () => {
    runButtons.length = 0;
    const groups = [];
    for (const t of app.st.tasks) if (!groups.includes(t.group)) groups.push(t.group);
    mount(body, groups.map((g) => h("section.task-group", null,
      h("h2.group-title", null, g),
      app.st.tasks.filter((t) => t.group === g).map((t) => taskCard(app, form, t, runButtons)),
    )));
    syncRun();
  };

  form.attach(build);
  form.onChange(syncRun);
  mount(el,
    h("header.page-head", null,
      h("div", null,
        h("h1.page-title", null, "任务"),
        h("p.page-desc", null, "这里的开关和选项所有账号共用。某个账号不想做某项，到「账号」里单独关掉。"),
      ),
    ),
    body,
    saveBar(form),
  );
  build();
  return {
    el,
    update() {
      if (form.sync()) build();
      syncRun();
    },
  };
}

function taskCard(app, form, t, runButtons) {
  const run = h("button.btn.btn-small", { type: "button", "data-k": "run-" + t.id, onclick: () => runOnce(app, form, t) }, icon("play"), "运行一次");
  runButtons.push(run);
  const head = h("div.task-card-head", null,
    h("div.grow", null,
      h("h3.task-card-title", null, t.title),
      t.desc ? h("p.muted.small", null, t.desc) : null,
      t.needs ? h("p.muted.small", null, "账号要求：" + t.needs) : null,
      t.manual ? h("p.muted.small", null, "只能手动运行，不参加每日计划。") : null,
    ),
    run,
  );
  const sw = t.manual ? null : toggle(form, ["tasks", t.id, "enabled"], "每天自动做");
  const opts = withOptions.has(t.id)
    ? h("details.task-options", null, h("summary", null, "选项"), h("div.form-grid", null, options(form, t.id)))
    : null;
  return h("article.task-card", null, head, sw, opts, h("p.run-hint.muted.small", { hidden: true }, "先保存修改再运行。"));
}

function options(form, id) {
  const p = (k) => ["tasks", id, k];
  switch (id) {
    case "daily":
      return [
        toggle(form, p("watch"), "观看视频"),
        toggle(form, p("share"), "分享视频"),
        number(form, p("coins"), "每天投币数", "0 表示不投币。", { min: 0, max: 5 }),
        number(form, p("keep_coins"), "至少保留硬币", "余额低于这个数就不投了。"),
        number(form, p("stop_at_level"), "到这个等级后停止投币", "0 表示一直投。", { min: 0, max: 6 }),
        toggle(form, p("like"), "投币时顺便点赞"),
        list(form, p("up_ids"), "优先观看和投币的 UP 主 UID", "留空时从关注和排行榜里选。", true),
      ];
    case "charge":
      return [
        number(form, p("up_id"), "充电对象 UID", "会用掉全部 B 币券，开启前确认 UID 没填错。"),
        number(form, p("day"), "每月几号起充电", null, { min: 1, max: 28 }),
        text(form, p("comment"), "充电留言", "留空不留言。"),
      ];
    case "manga":
      return [
        toggle(form, p("read"), "签到后再记一次阅读"),
        number(form, p("comic_id"), "漫画 ID"),
        number(form, p("ep_id"), "章节 ID"),
      ];
    case "fans_medal":
      return [
        toggle(form, p("like"), "给直播间点赞"),
        text(form, p("danmaku"), "发送的弹幕", "最多 30 个字，留空不发。"),
        toggle(form, p("watch"), "挂在直播间攒观看时长"),
        number(form, p("minutes"), "每个粉丝牌挂几分钟", null, { min: 1, max: 120 }),
        number(form, p("max"), "每轮最多处理几个粉丝牌", "0 表示全部。", { min: 0, max: 200 }),
      ];
    case "live_lottery":
      return [
        number(form, p("every_hours"), "每隔几小时找一次", "0 表示只在每日运行时找。", { min: 0, max: 24 }),
        list(form, p("include"), "奖品包含这些词才参加", "留空表示都参加。"),
        list(form, p("exclude"), "奖品包含这些词就不参加"),
        toggle(form, p("follow"), "参加需要关注主播的抽奖"),
        toggle(form, p("group"), "把因抽奖关注的主播放进「天选时刻」分组", "方便之后批量取关。"),
      ];
  }
  return [];
}

/* 运行一次 */

function runOnce(app, form, t) {
  const accs = app.st.accounts.filter((a) => a.logged_in);
  if (!accs.length) return toast("没有登录着的账号，先添加或重新扫码", "error");
  if (t.id === "unfollow") return unfollow(app, form, accs);
  const go = (acc) => app.act("run", `已开始给「${accountName(acc)}」运行${t.title}`, () => api.run(acc.mid, [t.id]));
  if (accs.length === 1) return go(accs[0]);
  modal((close) => h("div", null,
    h("h2.dialog-title", null, `用哪个账号运行「${t.title}」？`),
    h("div.pick-list", null, accs.map((acc) => h("button.btn.btn-wide", { type: "button", onclick: () => { close(); go(acc); } }, accountName(acc)))),
    h("div.dialog-actions", null, h("button.btn", { type: "button", onclick: () => close() }, "取消")),
  ));
}

function unfollow(app, form, accs) {
  const saved = app.st.settings.tasks.unfollow;
  const acct = h("select.input", { id: "uf-acc" }, accs.map((a) => h("option", { value: a.mid }, accountName(a))));
  const group = h("select.input", { id: "uf-group", disabled: true }, h("option", { value: "" }, "正在加载分组…"));
  const count = h("input.input.input-num", { id: "uf-count", type: "text", inputmode: "numeric", value: String(saved.count || 0) });
  const keep = h("input.input", { id: "uf-keep", type: "text", value: (saved.keep || []).join(", ") });
  const err = h("p.form-error", { role: "alert" });
  let groups = [];

  const load = async () => {
    group.disabled = true;
    mount(group, h("option", { value: "" }, "正在加载分组…"));
    try {
      groups = await api.groups(acct.value);
      mount(group, groups.length ? groups.map((g) => h("option", { value: g.name }, `${g.name}（${g.count} 人）`)) : h("option", { value: "" }, "这个账号没有关注分组"));
      if (groups.some((g) => g.name === saved.group)) group.value = saved.group;
      group.disabled = !groups.length;
    } catch (e) {
      mount(group, h("option", { value: "" }, "加载失败"));
      err.textContent = e.message;
    }
  };
  acct.addEventListener("change", load);

  const submit = async (close) => {
    err.textContent = "";
    const c = count.value.trim();
    const ks = keep.value.split(/[\s,，、]+/).filter(Boolean);
    if (!group.value) return (err.textContent = "先选一个分组");
    if (!/^\d+$/.test(c)) return (err.textContent = "取关数量请填整数，0 表示全部");
    if (ks.some((k) => !/^\d+$/.test(k))) return (err.textContent = "保留的只能填数字 UID");
    const g = groups.find((x) => x.name === group.value);
    const n = Number(c) || g.count;
    const acc = accs.find((a) => a.mid === acct.value);
    const ok = await confirm({
      title: `取关「${g.name}」里的人？`,
      text: `会用「${accountName(acc)}」取消关注这个分组里最多 ${Math.min(n, g.count)} 个人${ks.length ? `（保留 ${ks.length} 个 UID）` : ""}。取关后不能自动恢复。`,
      ok: "取关",
      danger: true,
    });
    if (!ok) return;
    close();
    // 只提交取关的选项，别的设置用服务端最新的，不受这页没保存的修改影响
    const s = structuredClone(app.st.settings);
    s.tasks.unfollow = { group: g.name, count: Number(c), keep: ks.map(Number) };
    app.act("run", "已开始批量取关", async () => {
      app.st.settings = await api.saveSettings(s);
      await api.run(acc.mid, ["unfollow"]);
    });
  };

  modal((close) => h("div.uf", null,
    h("h2.dialog-title", null, "批量取关"),
    h("p.muted", null, "取消关注一个分组里的人，例如因为抽奖关注的主播。"),
    h("div.form-grid", null,
      h("div.field", null, h("label.field-label", { for: "uf-acc" }, "账号"), acct),
      h("div.field", null, h("label.field-label", { for: "uf-group" }, "分组"), group),
      h("div.field", null, h("label.field-label", { for: "uf-count" }, "最多取关几个"), count, h("p.field-hint", null, "0 表示全部。")),
      h("div.field", null, h("label.field-label", { for: "uf-keep" }, "保留这些 UID"), keep, h("p.field-hint", null, "用逗号或空格分开。")),
    ),
    err,
    h("div.dialog-actions", null,
      h("button.btn", { type: "button", onclick: () => close() }, "取消"),
      h("button.btn.btn-danger", { type: "button", onclick: () => submit(close).catch(handleError) }, "取关"),
    ),
  ));
  load();
}
