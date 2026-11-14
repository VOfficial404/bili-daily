// 设置：运行计划、代理、结果推送、日志级别、报告保留天数和服务端信息。

import { api } from "./api.js";
import { h, mount, when } from "./dom.js";
import { sharedForm, text, number, select, saveBar } from "./form.js";

const channels = [
  ["", "不推送"],
  ["telegram", "Telegram"],
  ["pushplus", "PushPlus"],
  ["wecom", "企业微信群机器人"],
  ["dingtalk", "钉钉群机器人"],
  ["serverchan", "Server 酱"],
  ["bark", "Bark"],
  ["webhook", "自定义 Webhook"],
];

// 每个渠道要填的项：[字段, 标题, 说明]
const channelFields = {
  telegram: [["token", "机器人 Token", "从 BotFather 获取。"], ["target", "chat_id", "接收消息的聊天。"], ["endpoint", "反代地址", "留空使用官方服务。"]],
  pushplus: [["token", "PushPlus Token"]],
  wecom: [["token", "机器人 Webhook 地址", "群机器人的完整地址。"]],
  dingtalk: [["token", "机器人 Webhook 地址", "群机器人的完整地址。"], ["target", "加签密钥", "没开加签时留空。"]],
  serverchan: [["token", "SendKey"]],
  bark: [["token", "设备密钥"], ["endpoint", "服务器地址", "留空使用官方服务。"]],
  webhook: [["token", "Webhook 地址", "会收到 {\"title\": …, \"text\": …}。"]],
};

export function render(app) {
  const form = sharedForm(app);
  const el = h("div");
  const body = h("div.settings");

  const build = () => {
    const proxyResult = h("p.test-result", { role: "status" });
    const notifyResult = h("p.test-result", { role: "status" });
    const notifyFields = h("div.form-grid");
    const drawNotify = () => {
      const ch = form.get(["notify", "channel"]);
      mount(notifyFields, (channelFields[ch] || []).map(([k, label, hint]) =>
        text(form, ["notify", k], label, hint, { type: k === "token" ? "password" : "text" })));
      testNotify.disabled = !ch;
    };
    const testProxy = h("button.btn.btn-small", {
      type: "button",
      onclick: async () => {
        testProxy.disabled = true;
        proxyResult.className = "test-result";
        proxyResult.textContent = "正在测试…";
        try {
          const e = await api.testProxy(form.get(["proxy"]));
          proxyResult.textContent = `B 站看到的出口：${e.addr}${e.place ? "（" + e.place + (e.isp ? " " + e.isp : "") + "）" : ""}`;
          proxyResult.classList.add("is-ok");
        } catch (err) {
          proxyResult.textContent = err.message;
          proxyResult.classList.add("is-error");
        } finally {
          testProxy.disabled = false;
        }
      },
    }, "测试代理");
    const testNotify = h("button.btn.btn-small", {
      type: "button",
      onclick: async () => {
        testNotify.disabled = true;
        notifyResult.className = "test-result";
        notifyResult.textContent = "正在发送…";
        try {
          await api.testNotify(form.get(["notify"]));
          notifyResult.textContent = "已发送，去看看收到没有。";
          notifyResult.classList.add("is-ok");
        } catch (err) {
          notifyResult.textContent = err.message;
          notifyResult.classList.add("is-error");
        } finally {
          testNotify.disabled = !form.get(["notify", "channel"]);
        }
      },
    }, "发送测试");

    const eg = app.st.egress;
    mount(body,
      h("section.panel", null,
        h("h2.section-title", null, "运行计划"),
        h("div.form-grid", null,
          text(form, ["run_at"], "每天开始时间（北京时间）", "24 小时制，例如 09:30。", { type: "time" }),
          number(form, ["jitter"], "随机推迟最多几分钟", "每个账号每天推迟的时间不同。", { min: 0, max: 720 }),
          select(form, ["pace"], "请求节奏", [["slow", "慢速"], ["normal", "正常"], ["fast", "快速"]], "遇到风控或请求太频繁时改成慢速。"),
          text(form, ["proxy"], "代理", "所有 B 站请求经过它，留空直连。支持 HTTP、HTTPS、SOCKS5。", { placeholder: "socks5://127.0.0.1:1080" }),
        ),
        h("div.inline-actions", null, testProxy, proxyResult),
        eg ? h("p.muted.small", null, eg.error ? "上次检查出口失败：" + eg.error : `现在的出口：${eg.addr} ${eg.place || ""} ${eg.isp || ""}，${when(eg.checked_at)} 检查`) : null,
      ),
      h("section.panel", null,
        h("h2.section-title", null, "结果推送"),
        h("div.form-grid", null,
          select(form, ["notify", "when"], "什么时候推送", [["never", "不推送"], ["failed", "有失败时"], ["always", "每次运行后"]], "只推送定时运行的结果。"),
          select(form, ["notify", "channel"], "推送到", channels, null, drawNotify),
        ),
        notifyFields,
        h("div.inline-actions", null, testNotify, notifyResult),
      ),
      h("section.panel", null,
        h("h2.section-title", null, "日志和报告"),
        h("div.form-grid", null,
          select(form, ["log_level"], "记录到哪一级", [["debug", "调试"], ["info", "信息"], ["warn", "警告"], ["error", "错误"]], "排查问题时改成「调试」。"),
          number(form, ["report_retention_days"], "报告保留几天", "1～365 天，更早的报告自动删掉。每个账号最多留最近 60 份。", { min: 1, max: 365 }),
        ),
      ),
      h("section.panel", null,
        h("h2.section-title", null, "服务端"),
        h("dl.facts", null,
          h("div.fact", null, h("dt", null, "版本"), h("dd", null, app.st.version)),
          h("div.fact", null, h("dt", null, "服务器时间"), h("dd", null, when(app.st.now))),
        ),
      ),
    );
    drawNotify();
  };

  form.attach(build);
  mount(el,
    h("header.page-head", null,
      h("div", null,
        h("h1.page-title", null, "设置"),
        h("p.page-desc", null, "所有账号共用，保存后马上生效。"),
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
    },
  };
}
