// spareglass host page: shown on the host computer itself. It lists the
// address to open on the display device, asks the owner to approve new
// browsers, and manages remembered browsers. The token comes in the URL
// fragment (#...) that spareglass opened; every API call sends it.
"use strict";

const TEXT = {
  en: {
    running: "Running. Keep this page open to approve new devices.",
    pendingTitle: "A device wants to connect",
    pendingAsk: (name, slot) => `${name} wants to be display ${slot}.`,
    pendingCode: "Number shown on that device:",
    pendingHelp: "Allow only if the same number is on the device's screen.",
    allow: "Allow",
    deny: "Don't allow",
    connectTitle: "Connect a device",
    connectHelp: "On the tablet or computer you want to use as a display, open this address in a web browser, or scan the QR code with its camera:",
    otherURLs: "If it does not open, try:",
    noURL: "This computer has no local network address. Connect it to the same Wi-Fi as the other device.",
    connectNext: "Then tap Connect on that device. Both devices must be on the same Wi-Fi.",
    displaysTitle: "Connected displays",
    displaysEmpty: "None yet.",
    displayRow: (slot, name, w, h) => `Display ${slot}: ${name}` + (w ? ` (${w}×${h})` : ""),
    devicesTitle: "Allowed devices",
    devicesEmpty: "None yet. Devices you allow are remembered here.",
    devicePaired: (date) => `allowed ${date}`,
    forget: "Remove",
    forgetConfirm: (name) => `Remove ${name}? It will have to be allowed again to connect.`,
    stop: "Quit spareglass",
    stopHelp: "Quitting removes the extra displays.",
    stopConfirm: "Quit spareglass? Connected displays will close.",
    stopped: "spareglass has quit. You can close this tab.",
    lost: "spareglass is not running. Start it again to use this page.",
    noToken: "Open this page from the address spareglass shows when it starts.",
    failed: "That did not work. Try again.",
    titlePending: (n) => `(${n}) spareglass`,
  },
  ja: {
    running: "動作中です。新しい機器を許可するときのために、このページは開いたままにしてください。",
    pendingTitle: "接続を許可しますか",
    pendingAsk: (name, slot) => `${name} が ${slot} 枚目の画面として接続しようとしています。`,
    pendingCode: "その機器に出ている番号:",
    pendingHelp: "機器の画面に同じ番号が出ているときだけ許可してください。",
    allow: "許可する",
    deny: "許可しない",
    connectTitle: "機器をつなぐ",
    connectHelp: "画面として使いたいタブレットやパソコンのブラウザでこのアドレスを開くか、カメラで QR コードを読み取ってください。",
    otherURLs: "開けないときはこちらも試してください:",
    noURL: "このパソコンがネットワークにつながっていません。もう一方の機器と同じ Wi-Fi につないでください。",
    connectNext: "開いたら、その機器で「Connect」を押します。2 台とも同じ Wi-Fi につないでおいてください。",
    displaysTitle: "接続中の画面",
    displaysEmpty: "まだありません。",
    displayRow: (slot, name, w, h) => `${slot} 枚目: ${name}` + (w ? `（${w}×${h}）` : ""),
    devicesTitle: "許可した機器",
    devicesEmpty: "まだありません。許可した機器はここに残り、次からはすぐつながります。",
    devicePaired: (date) => `${date} に許可`,
    forget: "削除",
    forgetConfirm: (name) => `${name} を削除しますか? もう一度つなぐには、改めて許可が必要です。`,
    stop: "spareglass を終了する",
    stopHelp: "終了すると、増やした画面は消えます。",
    stopConfirm: "spareglass を終了しますか? 接続中の画面は切れます。",
    stopped: "spareglass を終了しました。このタブは閉じてかまいません。",
    lost: "spareglass が動いていません。使うときはもう一度起動してください。",
    noToken: "spareglass を起動したときに開いたページから使ってください。",
    failed: "うまくいきませんでした。もう一度試してください。",
    titlePending: (n) => `(${n}) spareglass`,
  },
};

const lang = (navigator.languages || [navigator.language]).some((l) => /^ja\b/i.test(l)) ? "ja" : "en";
const t = TEXT[lang];
document.documentElement.lang = lang;
for (const el of document.querySelectorAll("[data-t]")) el.textContent = t[el.dataset.t];

const $ = (id) => document.getElementById(id);
const token = decodeURIComponent(location.hash.slice(1));
let timer = 0;
let gone = false;

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { "X-Spareglass-Token": token, ...(body ? { "Content-Type": "application/json" } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status}`);
  return res.status === 204 ? null : res.json();
}

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    e.setAttribute(k, v);
  }
  e.append(...children);
  return e;
}

function showGone(text) {
  gone = true;
  clearTimeout(timer);
  $("gone-text").textContent = text;
  $("gone").hidden = false;
  document.querySelector("main").hidden = true;
  document.title = "spareglass";
}

async function act(button, fn) {
  button.disabled = true;
  try {
    await fn();
  } catch {
    button.disabled = false;
    alert(t.failed);
  }
  await refresh();
}

// changed reports whether a section's data differs from what it shows, so
// the page rebuilds a section (and its buttons) only when something changed.
const shown = {};
function changed(section, data) {
  const key = JSON.stringify(data);
  if (shown[section] === key) return false;
  shown[section] = key;
  return true;
}

function renderPending(list) {
  if (!changed("pending", list)) return;
  $("pending").hidden = list.length === 0;
  $("pending-list").replaceChildren(...list.map((r) => {
    const allow = el("button", { type: "button" }, t.allow);
    const deny = el("button", { type: "button", class: "secondary" }, t.deny);
    allow.addEventListener("click", () => act(allow, () => api("POST", `/api/pair/${encodeURIComponent(r.id)}`, { allow: true })));
    deny.addEventListener("click", () => act(deny, () => api("POST", `/api/pair/${encodeURIComponent(r.id)}`, { allow: false })));
    return el("li", { class: "card", "data-code": r.code },
      el("p", {}, t.pendingAsk(r.name, r.slot)),
      el("p", { class: "code-label" }, t.pendingCode),
      el("p", { class: "code" }, r.code),
      el("p", { class: "muted" }, t.pendingHelp),
      el("div", { class: "actions" }, allow, deny));
  }));
  document.title = list.length ? t.titlePending(list.length) : "spareglass";
}

function renderURLs(urls) {
  if (!changed("urls", urls)) return;
  $("url").textContent = urls[0] || "";
  $("url").hidden = urls.length === 0;
  $("no-url").hidden = urls.length > 0;
  $("qr").hidden = urls.length === 0;
  if (urls.length) $("qr").src = `/qr.png?${Date.now()}`;
  $("other-urls").hidden = urls.length < 2;
  $("other-urls").textContent = urls.length > 1 ? `${t.otherURLs} ${urls.slice(1).join("  ")}` : "";
}

function renderDisplays(list) {
  if (!changed("displays", list)) return;
  $("displays-empty").hidden = list.length > 0;
  $("displays-list").replaceChildren(...list.map((d) =>
    el("li", {}, t.displayRow(d.slot, d.device, d.width, d.height))));
}

function renderDevices(list) {
  if (!changed("devices", list)) return;
  $("devices-empty").hidden = list.length > 0;
  $("devices-list").replaceChildren(...list.map((d) => {
    const forget = el("button", { type: "button", class: "secondary small" }, t.forget);
    forget.addEventListener("click", () => {
      if (confirm(t.forgetConfirm(d.name))) act(forget, () => api("DELETE", `/api/devices/${encodeURIComponent(d.id)}`));
    });
    return el("li", { "data-device": d.id },
      el("span", {}, d.name, " ", el("span", { class: "muted" }, t.devicePaired(new Date(d.created).toLocaleDateString(lang)))),
      forget);
  }));
}

async function refresh() {
  clearTimeout(timer);
  let st;
  try {
    st = await api("GET", "/api/state");
  } catch (e) {
    if (gone) return;
    if (/: 403$/.test(e.message)) return showGone(t.noToken);
    return showGone(t.lost);
  }
  if (gone) return; // quit while this request was in flight
  $("problem").hidden = !st.display_problem;
  $("problem").textContent = st.display_problem || "";
  renderPending(st.pending);
  renderURLs(st.urls);
  renderDisplays(st.displays);
  renderDevices(st.devices);
  timer = setTimeout(refresh, 1000);
}

$("stop").addEventListener("click", async () => {
  if (!confirm(t.stopConfirm)) return;
  try {
    await api("POST", "/api/stop");
    showGone(t.stopped);
  } catch {
    alert(t.failed);
  }
});

refresh();
