// 日志：服务端最近的日志，每 2 秒取一次新的。

import { api } from "./api.js";
import { h, mount, accountName } from "./dom.js";

const levels = [["info", "信息及以上"], ["warn", "警告及以上"], ["error", "只看错误"], ["debug", "全部（含调试）"]];
const levelText = { debug: "调试", info: "信息", warn: "警告", error: "错误" };
const MAX = 2000;

const fmt = new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });

export function render(app) {
  const el = h("div");
  const consoleEl = h("div.console", { role: "log", "aria-live": "off", tabindex: "0" });
  let level = "info", mid = "", after = 0, timer = 0, gen = 0;

  const fetchMore = async () => {
    const g = gen;
    try {
      const entries = (await api.logs({ after, level, mid })) || [];
      if (g !== gen) return;
      const atBottom = consoleEl.scrollHeight - consoleEl.scrollTop - consoleEl.clientHeight < 40;
      if (entries.length && consoleEl.querySelector(".console-empty")) consoleEl.replaceChildren();
      for (const e of entries) {
        after = Math.max(after, e.seq);
        const acc = e.mid && app.account(e.mid);
        consoleEl.append(h("div.log", { class: "is-" + e.level },
          h("span.log-time", null, fmt.format(new Date(e.time))),
          h("span.log-level", null, levelText[e.level] || e.level),
          e.mid ? h("span.log-acc", null, acc ? accountName(acc) : e.mid) : null,
          h("span.log-text", null, e.text),
        ));
      }
      while (consoleEl.childElementCount > MAX) consoleEl.firstElementChild.remove();
      if (!consoleEl.childElementCount) consoleEl.append(h("p.console-empty.muted", null, "没有符合条件的日志。"));
      if (atBottom || entries.length === consoleEl.childElementCount) consoleEl.scrollTop = consoleEl.scrollHeight;
    } catch {
      // 连接中断由外框提示，这里下次再试
    }
    if (g === gen) timer = setTimeout(fetchMore, 2000);
  };

  const reset = () => {
    gen++;
    clearTimeout(timer);
    after = 0;
    consoleEl.replaceChildren();
    fetchMore();
  };

  mount(el,
    h("header.page-head", null,
      h("div", null,
        h("h1.page-title", null, "日志"),
        h("p.page-desc", null, "服务端最近的日志，时间是北京时间。只保留在内存里，重启后清空。"),
      ),
      h("div.page-actions", null,
        h("select.input.input-auto", { "aria-label": "级别", onchange: (e) => { level = e.target.value; reset(); } }, levels.map(([v, t]) => h("option", { value: v }, t))),
        h("select.input.input-auto", { "aria-label": "账号", onchange: (e) => { mid = e.target.value; reset(); } },
          h("option", { value: "" }, "全部账号"), app.st.accounts.map((a) => h("option", { value: a.mid }, accountName(a)))),
      ),
    ),
    consoleEl,
  );
  reset();
  return {
    el,
    leave() {
      gen++;
      clearTimeout(timer);
    },
  };
}
