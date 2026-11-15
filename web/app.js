// spareglass browser client.
// URL: http://<host>/<display number>. A browser seen for the first time asks
// the host to approve it; the credential it gets back is kept in localStorage
// so later visits connect straight away.
"use strict";

const $ = (id) => document.getElementById(id);
const video = $("screen");
const statusEl = $("status");
const codeEl = $("code");
const button = $("connect");

const slot = parseInt(location.pathname.slice(1), 10) || 1;
const STORAGE_KEY = "spareglass.device";

let pc = null;
let control = null;

$("title").textContent = `Use this screen as display ${slot}`;
document.title = `spareglass · display ${slot}`;

function setStatus(text, isError = false) {
  statusEl.textContent = text;
  statusEl.classList.toggle("error", isError);
}

function showCode(code) {
  codeEl.hidden = !code;
  codeEl.textContent = code || "";
}

function loadDevice() {
  try {
    const d = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
    return d && d.id && d.secret ? d : null;
  } catch {
    return null;
  }
}

function saveDevice(d) {
  try {
    if (d) localStorage.setItem(STORAGE_KEY, JSON.stringify(d));
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Private browsing may block storage; pairing then lasts for this page only.
  }
}

let device = loadDevice();

function browserName() {
  const ua = navigator.userAgent;
  const browser =
    /Edg\//.test(ua) ? "Edge" :
    /Firefox\//.test(ua) ? "Firefox" :
    /Chrome\//.test(ua) ? "Chrome" :
    /Safari\//.test(ua) ? "Safari" : "Browser";
  const os =
    /iPad/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1) ? "iPad" :
    /iPhone/.test(ua) ? "iPhone" :
    /Android/.test(ua) ? "Android" :
    /Windows/.test(ua) ? "Windows" :
    /Mac OS X/.test(ua) ? "Mac" :
    /CrOS/.test(ua) ? "ChromeOS" :
    /Linux/.test(ua) ? "Linux" : "device";
  return `${browser} on ${os}`;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function api(path, options = {}) {
  const res = await fetch(path, options);
  const body = await res.json().catch(() => ({}));
  return { res, body };
}

// pair asks the host to approve this browser and waits for the answer.
async function pair() {
  const { res, body } = await api("/api/pair", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name: browserName(), slot }),
  });
  if (!res.ok) throw new Error(body.error || `Host answered ${res.status}`);

  showCode(body.code);
  setStatus("Approve this code on the host computer: answer y in the spareglass terminal.");

  const deadline = Date.now() + 2 * 60 * 1000;
  while (Date.now() < deadline) {
    await sleep(1000);
    const st = await api(`/api/pair/${body.id}`);
    if (!st.res.ok) throw new Error(st.body.error || "Pairing request was lost. Try again.");
    switch (st.body.status) {
      case "approved":
        showCode(null);
        device = { id: st.body.device_id, secret: st.body.secret };
        saveDevice(device);
        return;
      case "denied":
        showCode(null);
        throw new Error("The host declined this browser.");
      case "expired":
        showCode(null);
        throw new Error("Nobody answered on the host. Tap Connect to ask again.");
    }
  }
  showCode(null);
  throw new Error("Nobody answered on the host. Tap Connect to ask again.");
}

function screenInfo(type) {
  return {
    type,
    width: window.screen.width,
    height: window.screen.height,
    dpr: window.devicePixelRatio || 1,
    orientation: (screen.orientation && screen.orientation.type) || "",
  };
}

function waitForIce(peer, timeoutMs = 3000) {
  if (peer.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const done = () => {
      peer.removeEventListener("icegatheringstatechange", check);
      resolve();
    };
    const check = () => peer.iceGatheringState === "complete" && done();
    peer.addEventListener("icegatheringstatechange", check);
    setTimeout(done, timeoutMs);
  });
}

class NotPaired extends Error {}

async function startSession() {
  pc = new RTCPeerConnection();
  pc.addTransceiver("video", { direction: "recvonly" });

  pc.ontrack = (ev) => {
    if ("jitterBufferTarget" in ev.receiver) ev.receiver.jitterBufferTarget = 0;
    video.srcObject = ev.streams[0] || new MediaStream([ev.track]);
  };

  pc.onconnectionstatechange = () => {
    if (pc && (pc.connectionState === "failed" || pc.connectionState === "closed")) {
      disconnect("The connection to the host was lost. Tap Connect to retry.", true);
    }
  };

  control = pc.createDataChannel("control");
  control.onopen = () => control.send(JSON.stringify(screenInfo("hello")));
  control.onmessage = (ev) => handleStatus(JSON.parse(ev.data));

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIce(pc);

  const { res, body } = await api("/api/session", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Device ${device.id}:${device.secret}`,
    },
    body: JSON.stringify({ sdp: pc.localDescription.sdp, slot }),
  });
  if (res.status === 401) throw new NotPaired();
  if (!res.ok) throw new Error(body.error || `Host answered ${res.status}`);
  await pc.setRemoteDescription({ type: "answer", sdp: body.sdp });
}

async function connect() {
  button.disabled = true;

  if (!device) {
    setStatus("Asking the host…");
    await pair();
    // The tap that started pairing can no longer open full screen, so ask for one more.
    button.disabled = false;
    setStatus("Approved. Tap Connect to start.");
    return;
  }

  setStatus("Connecting…");
  if (document.documentElement.requestFullscreen) {
    document.documentElement.requestFullscreen().catch(() => {});
  }
  try {
    await startSession();
  } catch (err) {
    if (err instanceof NotPaired) {
      saveDevice(null);
      device = null;
      closePeer();
      button.disabled = false;
      setStatus("The host no longer knows this browser. Tap Connect to pair again.", true);
      return;
    }
    throw err;
  }
}

function handleStatus(msg) {
  if (msg.type !== "status") return;
  switch (msg.state) {
    case "streaming":
      document.body.classList.add("live");
      break;
    case "display-only":
      setStatus(`Display ${slot} was created on the host (${msg.width}×${msg.height}), but it cannot be streamed: ${msg.detail}`, true);
      break;
    case "no-display":
      setStatus(`Connected, but the host cannot create a display: ${msg.detail}`, true);
      break;
    default:
      if (msg.state === "error") document.body.classList.remove("live");
      setStatus(msg.detail || msg.state, msg.state === "error");
  }
}

function closePeer() {
  if (pc) pc.close();
  pc = null;
  control = null;
  video.srcObject = null;
}

function disconnect(message, isError = false) {
  document.body.classList.remove("live");
  closePeer();
  showCode(null);
  button.disabled = false;
  setStatus(message, isError);
}

let resizeTimer = 0;
function onResize() {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(() => {
    if (control && control.readyState === "open") {
      control.send(JSON.stringify(screenInfo("resize")));
    }
  }, 400);
}

setStatus(device ? "This browser is paired. Tap Connect." : "Tap Connect, then approve this browser on the host.");
button.addEventListener("click", () => {
  connect().catch((err) => disconnect(err.message || String(err), true));
});
window.addEventListener("resize", onResize);
