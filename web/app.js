// spareglass browser client.
// Negotiates one WebRTC session with the host and shows the virtual display.
"use strict";

const $ = (id) => document.getElementById(id);
const video = $("screen");
const statusEl = $("status");
const button = $("connect");

const token = new URLSearchParams(location.hash.slice(1)).get("t");

let pc = null;
let control = null;

function setStatus(text, isError = false) {
  statusEl.textContent = text;
  statusEl.classList.toggle("error", isError);
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

async function connect() {
  button.disabled = true;
  setStatus("Connecting…");

  // Full screen needs a user gesture, so request it first.
  if (document.documentElement.requestFullscreen) {
    document.documentElement.requestFullscreen().catch(() => {});
  }

  pc = new RTCPeerConnection();
  pc.addTransceiver("video", { direction: "recvonly" });

  pc.ontrack = (ev) => {
    if ("jitterBufferTarget" in ev.receiver) ev.receiver.jitterBufferTarget = 0;
    video.srcObject = ev.streams[0] || new MediaStream([ev.track]);
  };

  pc.onconnectionstatechange = () => {
    if (pc.connectionState === "failed" || pc.connectionState === "closed") {
      disconnect("The connection to the host was lost. Connect again to retry.", true);
    }
  };

  control = pc.createDataChannel("control");
  control.onopen = () => control.send(JSON.stringify(screenInfo("hello")));
  control.onmessage = (ev) => handleStatus(JSON.parse(ev.data));

  const offer = await pc.createOffer();
  await pc.setLocalDescription(offer);
  await waitForIce(pc);

  const res = await fetch("/api/session", {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ sdp: pc.localDescription.sdp }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.error || `Host answered ${res.status}`);
  }
  await pc.setRemoteDescription({ type: "answer", sdp: body.sdp });
}

function handleStatus(msg) {
  if (msg.type !== "status") return;
  switch (msg.state) {
    case "streaming":
      document.body.classList.add("live");
      break;
    case "display-only":
      setStatus(`Display created on the host (${msg.width}×${msg.height}). Video streaming is not implemented yet.`);
      break;
    case "no-display":
      setStatus(`Connected, but the host cannot create a display: ${msg.detail}`, true);
      break;
    default:
      setStatus(msg.detail || msg.state, msg.state === "error");
  }
}

function disconnect(message, isError = false) {
  document.body.classList.remove("live");
  if (pc) pc.close();
  pc = null;
  control = null;
  video.srcObject = null;
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

if (!token) {
  button.disabled = true;
  setStatus("This page needs a pairing link. Open the URL or scan the QR code shown by spareglass on the host.", true);
} else {
  button.addEventListener("click", () => {
    connect().catch((err) => disconnect(`Could not connect: ${err.message}`, true));
  });
  window.addEventListener("resize", onResize);
}
