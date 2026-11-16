// 和服务端接口打交道：带令牌发请求，把错误变成给人看的话。接口说明见 docs/DESIGN.md。

const KEY = "bili-daily-token";

export const token = {
  get() {
    return sessionStorage.getItem(KEY) || localStorage.getItem(KEY) || "";
  },
  // remember 为 true 时关掉浏览器也记得；否则只在这个标签页里有效
  set(value, remember) {
    this.clear();
    (remember ? localStorage : sessionStorage).setItem(KEY, value);
  },
  clear() {
    sessionStorage.removeItem(KEY);
    localStorage.removeItem(KEY);
  },
};

export class APIError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

async function call(method, path, body) {
  const opts = { method, headers: { Authorization: "Bearer " + token.get() } };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  let resp;
  try {
    resp = await fetch(path, opts);
  } catch {
    throw new APIError(0, "连不上服务器，请检查网络或服务是否在运行");
  }
  const text = await resp.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new APIError(resp.status, `服务器返回 HTTP ${resp.status}，内容看不懂`);
    }
  }
  if (!resp.ok) {
    throw new APIError(resp.status, (data && data.error) || `服务器返回 HTTP ${resp.status}`);
  }
  return data;
}

const q = (params) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "" && v !== null) s.set(k, v);
  const out = s.toString();
  return out ? "?" + out : "";
};

const mid = (m) => encodeURIComponent(m);

export const api = {
  ping: () => call("GET", "/api/ping"),
  state: () => call("GET", "/api/state"),
  qrStart: () => call("POST", "/api/qr"),
  qrPoll: (id) => call("GET", "/api/qr/" + encodeURIComponent(id)),
  runAll: () => call("POST", "/api/run"),
  run: (m, tasks) => call("POST", `/api/accounts/${mid(m)}/run`, { tasks: tasks || [] }),
  cancel: (m) => call("DELETE", `/api/accounts/${mid(m)}/run`),
  refresh: (m) => call("POST", `/api/accounts/${mid(m)}/refresh`),
  patch: (m, patch) => call("PATCH", `/api/accounts/${mid(m)}`, patch),
  remove: (m) => call("DELETE", `/api/accounts/${mid(m)}`),
  groups: (m) => call("GET", `/api/accounts/${mid(m)}/groups`),
  runs: (params) => call("GET", "/api/runs" + q(params || {})),
  runDetail: (id) => call("GET", "/api/runs/" + encodeURIComponent(id)),
  saveSettings: (s) => call("PUT", "/api/settings", s),
  testNotify: (n) => call("POST", "/api/notify/test", n),
  testProxy: (proxy) => call("POST", "/api/proxy/test", { proxy }),
  logs: (params) => call("GET", "/api/logs" + q(params || {})),
  upProfile: (m) => call("GET", `/api/ups/${mid(m)}`),
  followings: (m, pn, query) => call("GET", `/api/accounts/${mid(m)}/followings` + q({ pn, q: query })),
};

// mergeSettings 把表单里改过的项（edited 相对 base 的差别）放到最新的设置 latest 上，
// 别的客户端在表单打开期间改过的项不会被旧值盖掉。和 api.MergeSettings 的规则一样。
export function mergeSettings(base, edited, latest) {
  const out = structuredClone(latest);
  apply(base, edited, out);
  return out;
}

function apply(base, edited, dst) {
  for (const k of Object.keys(edited)) {
    const e = edited[k], b = base ? base[k] : undefined;
    if (isObject(e) && isObject(b) && isObject(dst[k])) {
      apply(b, e, dst[k]);
    } else if (JSON.stringify(e) !== JSON.stringify(b)) {
      dst[k] = structuredClone(e);
    }
  }
}

const isObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);
