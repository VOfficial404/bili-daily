// 在样式加载前定下深浅色，避免打开页面时闪一下。选择存在本机，「跟随系统」时不写属性。
try {
  const t = localStorage.getItem("theme");
  if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
} catch {}
