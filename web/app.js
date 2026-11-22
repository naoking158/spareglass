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
const cursorEl = $("cursor");

const slot = parseInt(location.pathname.slice(1), 10) || 1;
const STORAGE_KEY = "spareglass.device";

let pc = null;
let control = null;
let cursorChannel = null;
let statsTimer = 0;

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
  setStatus("Approve this code on the host computer: click Allow on the spareglass page (or answer y in its terminal).");

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

// Stream stats: every STATS_INTERVAL_MS the client reports per-frame delays
// to the host, which logs them next to what it sent (serve -v).
const STATS_INTERVAL_MS = 5000;
const presented = { count: 0, receiveToPresent: 0 };

function watchFrames() {
  if (!("requestVideoFrameCallback" in video)) return;
  const onFrame = (_now, meta) => {
    if (meta.receiveTime > 0) {
      presented.count++;
      presented.receiveToPresent += meta.presentationTime - meta.receiveTime;
    }
    video.requestVideoFrameCallback(onFrame);
  };
  video.requestVideoFrameCallback(onFrame);
}

async function readStats(peer) {
  const out = { at: performance.now(), rtt: 0 };
  (await peer.getStats()).forEach((r) => {
    if (r.type === "inbound-rtp" && r.kind === "video") out.video = r;
    if (r.type === "candidate-pair" && r.nominated && r.state === "succeeded") out.rtt = (r.currentRoundTripTime || 0) * 1000;
  });
  return out;
}

function diffStats(prev, cur) {
  const a = prev.video || {};
  const b = cur.video || {};
  const d = (k) => (b[k] || 0) - (a[k] || 0);
  const perFrame = (total, count) => (count > 0 ? (total * 1000) / count : 0);
  const decoded = d("framesDecoded");
  const ms = cur.at - prev.at;
  const r1 = (x) => Math.round(x * 10) / 10;
  return {
    type: "stats",
    interval_ms: Math.round(ms),
    fps: r1(ms > 0 ? (decoded * 1000) / ms : 0),
    jitter_buffer_ms: r1(perFrame(d("jitterBufferDelay"), d("jitterBufferEmittedCount"))),
    decode_ms: r1(perFrame(d("totalDecodeTime"), decoded)),
    processing_ms: r1(perFrame(d("totalProcessingDelay"), decoded)),
    receive_to_present_ms: r1(presented.count > 0 ? presented.receiveToPresent / presented.count : 0),
    rtt_ms: r1(cur.rtt),
    kbps: r1(ms > 0 ? (d("bytesReceived") * 8) / ms : 0),
    frames_dropped: d("framesDropped"),
    freezes: d("freezeCount"),
    packets_lost: d("packetsLost"),
    keyframes: d("keyFramesDecoded"),
  };
}

function startStats(peer, channel) {
  stopStats();
  let prev = null;
  const tick = async () => {
    const cur = await readStats(peer);
    if (prev && channel.readyState === "open") channel.send(JSON.stringify(diffStats(prev, cur)));
    presented.count = 0;
    presented.receiveToPresent = 0;
    prev = cur;
  };
  tick().catch(() => {});
  statsTimer = setInterval(() => tick().catch(() => {}), STATS_INTERVAL_MS);
}

function stopStats() {
  clearInterval(statsTimer);
  statsTimer = 0;
}

// Pointer overlay: a macOS host leaves the pointer out of the video and sends
// its position on the "cursor" channel in display points. The arrow is drawn
// at that point of the letterboxed video, scaled like the video.
const CURSOR_TIP = 1; // the arrow's tip in its viewBox units (points)
const cursor = { seq: 0, x: 0, y: 0, visible: false };
let displaySize = null; // [width, height] in points, from the "streaming" status

function placeCursor() {
  const vw = video.videoWidth;
  const vh = video.videoHeight;
  if (!cursor.visible || !displaySize || !vw || !vh || !document.body.classList.contains("live")) {
    cursorEl.toggleAttribute("hidden", true);
    return;
  }
  const scale = Math.min(video.clientWidth / vw, video.clientHeight / vh);
  const width = vw * scale;
  const height = vh * scale;
  const perPoint = width / displaySize[0];
  const left = (video.clientWidth - width) / 2 + cursor.x * perPoint - CURSOR_TIP * perPoint;
  const top = (video.clientHeight - height) / 2 + cursor.y * (height / displaySize[1]) - CURSOR_TIP * perPoint;
  cursorEl.style.width = `${cursorEl.viewBox.baseVal.width * perPoint}px`;
  cursorEl.style.height = `${cursorEl.viewBox.baseVal.height * perPoint}px`;
  cursorEl.style.transform = `translate(${left}px, ${top}px)`;
  cursorEl.toggleAttribute("hidden", false);
}

function onCursor(ev) {
  const msg = JSON.parse(ev.data);
  if (msg.seq <= cursor.seq) return; // unordered channel: a late message is stale
  Object.assign(cursor, msg);
  placeCursor();
}

function resetCursor() {
  Object.assign(cursor, { seq: 0, visible: false });
  displaySize = null;
  cursorEl.toggleAttribute("hidden", true);
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
  control.onopen = () => {
    control.send(JSON.stringify(screenInfo("hello")));
    startStats(pc, control);
  };
  control.onmessage = (ev) => handleStatus(JSON.parse(ev.data));
  cursorChannel = pc.createDataChannel("cursor", { ordered: false, maxRetransmits: 0 });
  cursorChannel.onmessage = onCursor;
  // Chrome reports a peer the host closed only as "disconnected", but the
  // data channel closes at once.
  const channel = control;
  control.onclose = () => {
    if (control === channel) disconnect("The host closed this connection. Tap Connect to retry.", true);
  };

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
      displaySize = [msg.width, msg.height];
      document.body.classList.add("live");
      placeCursor();
      break;
    case "display-only":
      disconnect(`The host can create display ${slot} but cannot stream it: ${msg.detail}`, true);
      break;
    case "no-display":
      disconnect(`The host cannot create a display: ${msg.detail}`, true);
      break;
    case "error":
      disconnect(msg.detail || "The host reported an error.", true);
      break;
    default:
      setStatus(msg.detail || msg.state);
  }
}

function closePeer() {
  stopStats();
  if (pc) pc.close();
  pc = null;
  control = null;
  cursorChannel = null;
  video.srcObject = null;
  resetCursor();
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
window.addEventListener("resize", placeCursor);
video.addEventListener("resize", placeCursor);
watchFrames();
