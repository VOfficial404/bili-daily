// 设置表单：在一份副本上编辑，保存时只提交改过的项。「任务」页和「设置」页共用同一份，
// 在两页之间切换不会丢掉没保存的修改。

import { api, mergeSettings } from "./api.js";
import { h, switchRow } from "./dom.js";
import { confirm, handleError, toast } from "./app.js";
import { cachedUP } from "./ups.js";

class Form {
  constructor(app) {
    this.app = app;
    this.saving = false;
    this.listeners = []; // 当前页面的监听；每个页面渲染时换成自己的
    this.rebuild = null; // 当前页面重画字段的函数
    this.reset(app.st.settings);
  }

  reset(s) {
    this.base = structuredClone(s);
    this.data = structuredClone(s);
    this.errors = new Map();
  }

  get dirty() {
    return JSON.stringify(this.base) !== JSON.stringify(this.data);
  }

  // attach 把表单交给一个页面：换上它的重画函数，清掉上一页的监听
  attach(rebuild) {
    this.listeners = [];
    this.rebuild = rebuild;
  }

  // sync 在表单没有修改时跟上服务端的设置（别的客户端改过），跟上了返回 true，页面要重画字段
  sync() {
    if (this.saving || this.dirty || JSON.stringify(this.base) === JSON.stringify(this.app.st.settings)) return false;
    this.reset(this.app.st.settings);
    return true;
  }

  get(path) {
    return path.reduce((o, k) => o[k], this.data);
  }

  set(path, v) {
    this.get(path.slice(0, -1))[path.at(-1)] = v;
    this.changed();
  }

  error(key, msg) {
    if (msg) this.errors.set(key, msg);
    else this.errors.delete(key);
    this.changed();
  }

  onChange(fn) {
    this.listeners.push(fn);
  }

  changed() {
    for (const fn of this.listeners) fn();
  }

  discard() {
    this.reset(this.app.st.settings);
    if (this.rebuild) this.rebuild();
    this.changed();
  }

  async save() {
    if (this.errors.size || this.saving) return;
    const c = this.data.tasks.charge, b = this.base.tasks.charge;
    if (c.enabled && (!b.enabled || c.up_id !== b.up_id)) {
      if (!c.up_id) return toast("先在「B 币券充电」的选项里填写充电对象的 UID", "error");
      const up = cachedUP(c.up_id);
      const who = up && up.uname ? `「${up.uname}」（UID ${c.up_id}）` : `UID ${c.up_id}`;
      const ok = await confirm({
        title: "开启 B 币券充电？",
        text: `每月 ${c.day} 号起，会把每个年度大会员账号的全部 B 币券充给 ${who}。充出去不能退回，请确认 UID 没填错。`,
        ok: "确认并保存",
      });
      if (!ok) return;
    }
    this.saving = true;
    this.changed();
    try {
      const saved = await api.saveSettings(mergeSettings(this.base, this.data, this.app.st.settings));
      this.app.st.settings = saved;
      this.reset(saved);
      if (this.rebuild) this.rebuild();
      toast("设置已保存");
    } catch (err) {
      handleError(err);
    } finally {
      this.saving = false;
      this.changed();
    }
  }
}

let shared = null;

export function sharedForm(app) {
  if (!shared) shared = new Form(app);
  return shared;
}

export function resetForm() {
  shared = null;
}

export const hasUnsaved = () => Boolean(shared && shared.dirty);

/* 字段 */

let seq = 0;

export function field(label, control, hint, err) {
  if (!control.id) control.id = "f" + ++seq;
  return h("div.field", null,
    h("label.field-label", { for: control.id }, label),
    control,
    hint ? h("p.field-hint", null, hint) : null,
    err,
  );
}

export function text(form, path, label, hint, opts = {}) {
  const input = h("input.input", {
    type: opts.type || "text",
    value: form.get(path) ?? "",
    placeholder: opts.placeholder,
    autocomplete: "off",
    "data-k": path.join("."),
    oninput: (e) => form.set(path, e.target.value),
  });
  return field(label, input, hint);
}

// number 是整数输入。填得不对时显示原因，不写进表单，保存按钮也不能点
export function number(form, path, label, hint, { min = 0, max = Infinity } = {}) {
  const key = path.join(".");
  const err = h("p.field-error", { role: "alert" });
  const input = h("input.input.input-num", {
    type: "text",
    inputmode: "numeric",
    value: String(form.get(path) ?? 0),
    "data-k": key,
    oninput: (e) => {
      const s = e.target.value.trim();
      const n = Number(s);
      let msg = "";
      if (!/^\d+$/.test(s)) msg = "请填写整数";
      else if (n < min || n > max) msg = max === Infinity ? `不能小于 ${min}` : `范围是 ${min}～${max}`;
      err.textContent = msg;
      input.setAttribute("aria-invalid", msg ? "true" : "false");
      if (!msg) form.get(path.slice(0, -1))[path.at(-1)] = n;
      form.error(key, msg);
    },
  });
  return field(label, input, hint, err);
}

export function toggle(form, path, label, hint) {
  return switchRow(path.join("."), label, hint, Boolean(form.get(path)), false, (v) => form.set(path, v));
}

export function select(form, path, label, options, hint, onPick) {
  const sel = h("select.input", {
    "data-k": path.join("."),
    onchange: (e) => {
      form.set(path, e.target.value);
      if (onPick) onPick(e.target.value);
    },
  }, options.map(([v, t]) => h("option", { value: v }, t)));
  sel.value = form.get(path) ?? "";
  return field(label, sel, hint);
}

// list 是用逗号或空格分开的一组词；numeric 时只能是数字（UID）
export function list(form, path, label, hint, numeric) {
  const key = path.join(".");
  const err = h("p.field-error", { role: "alert" });
  const input = h("input.input", {
    type: "text",
    value: (form.get(path) || []).join(", "),
    "data-k": key,
    oninput: (e) => {
      const parts = e.target.value.split(/[\s,，、]+/).filter(Boolean);
      const msg = numeric && parts.some((p) => !/^\d+$/.test(p)) ? "只能填数字 UID，用逗号或空格分开" : "";
      err.textContent = msg;
      input.setAttribute("aria-invalid", msg ? "true" : "false");
      if (!msg) form.get(path.slice(0, -1))[path.at(-1)] = numeric ? parts.map(Number) : parts;
      form.error(key, msg);
    },
  });
  return field(label, input, hint, err);
}

// saveBar 是贴在页面底部的保存条，有修改时才出现
export function saveBar(form) {
  const msg = h("span.savebar-text");
  const discard = h("button.btn", { type: "button", onclick: () => form.discard() }, "放弃修改");
  const save = h("button.btn.btn-primary", { type: "button", onclick: () => form.save() }, "保存");
  const bar = h("div.savebar", { role: "region", "aria-label": "保存设置" }, msg, discard, save);
  const sync = () => {
    bar.hidden = !form.dirty && !form.saving;
    save.disabled = form.saving || form.errors.size > 0;
    discard.disabled = form.saving;
    save.textContent = form.saving ? "正在保存…" : "保存";
    msg.textContent = form.errors.size ? "有填得不对的项，改好后才能保存" : "有没保存的修改，所有账号共用";
  };
  form.onChange(sync);
  sync();
  return bar;
}
