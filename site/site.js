// 深浅色开关、复制命令，以及首屏演示窗口里能点的部分。

/* 深浅色：右上角和功能卡片里的开关是同一个设置 */
const root = document.documentElement;
const themeButtons = document.querySelectorAll("[data-theme-set]");
function currentTheme() {
  return root.dataset.theme || "system";
}
function showTheme() {
  for (const b of themeButtons) b.setAttribute("aria-checked", String(b.dataset.themeSet === currentTheme()));
}
for (const b of themeButtons) {
  b.addEventListener("click", () => {
    const t = b.dataset.themeSet;
    if (t === "system") delete root.dataset.theme;
    else root.dataset.theme = t;
    try {
      if (t === "system") localStorage.removeItem("theme");
      else localStorage.setItem("theme", t);
    } catch {}
    showTheme();
  });
}
showTheme();

/* 复制部署命令 */
for (const btn of document.querySelectorAll("[data-copy]")) {
  btn.addEventListener("click", async () => {
    const text = document.getElementById(btn.dataset.copy).textContent;
    try {
      await navigator.clipboard.writeText(text);
      btn.textContent = "已复制";
    } catch {
      btn.textContent = "请手动选中";
    }
    setTimeout(() => (btn.textContent = "复制"), 2000);
  });
}

/* 演示：问候语和日期按北京时间 */
const now = new Date();
const hour = Number(new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Shanghai", hour: "numeric", hour12: false }).format(now));
const greeting = hour < 5 ? "夜深了" : hour < 11 ? "早上好" : hour < 13 ? "中午好" : hour < 18 ? "下午好" : "晚上好";
const dateText = new Intl.DateTimeFormat("zh-CN", { timeZone: "Asia/Shanghai", month: "long", day: "numeric", weekday: "long" }).format(now);
document.querySelector("[data-greeting]").textContent = greeting;
document.querySelector("[data-today]").textContent = dateText;

/* 演示：点「立即运行」，小号的任务一项项跑完 */
const run = document.querySelector("[data-run2]");
const rows = [...document.querySelectorAll("[data-rows2] li")];
const count = document.querySelector("[data-count2]");
const foot = document.querySelector("[data-foot2]");
const summary = document.querySelector("[data-summary]");
const hint = document.querySelector("[data-hint]");
const quick = matchMedia("(prefers-reduced-motion: reduce)").matches;
let finished = 6;
run.classList.add("pulse");

run.addEventListener("click", async () => {
  run.disabled = true;
  run.classList.remove("pulse");
  run.lastChild.textContent = "运行中";
  foot.textContent = "正在运行";
  let done = 0;
  for (const li of rows) {
    const g = li.querySelector(".g");
    const note = li.querySelector(".note");
    li.className = "is-run";
    g.textContent = "●";
    note.textContent = "正在做…";
    await new Promise((r) => setTimeout(r, quick ? 0 : 520));
    const skip = note.hasAttribute("data-skip");
    li.className = skip ? "is-skip" : "is-ok";
    g.textContent = skip ? "–" : "✓";
    note.textContent = note.dataset.note;
    count.textContent = String(++done);
    summary.textContent = `2 个账号，今天 12 项完成了 ${++finished} 项`;
  }
  document.querySelector("[data-tl-run]").hidden = false;
  document.querySelector("[data-tl-next]").hidden = true;
  foot.textContent = "下次运行 明天 09:51";
  document.querySelector("[data-acc2] .acc-meta").textContent = "硬币 42 · 刚跑完";
  run.lastChild.textContent = "已完成";
  hint.textContent = "跑完了。跳过的两项写明了为什么不用做。";
});

/* 演示：点失败的那一行，打开报告 */
const sheet = document.querySelector("[data-sheet]");
const opener = document.querySelector("[data-open-report]");
opener.addEventListener("click", () => {
  sheet.hidden = false;
  sheet.querySelector("[data-close-report]").focus();
});
sheet.querySelector("[data-close-report]").addEventListener("click", () => {
  sheet.hidden = true;
  opener.focus();
});
sheet.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    sheet.hidden = true;
    opener.focus();
  }
});
