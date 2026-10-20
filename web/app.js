(() => {
  const el = {
    boot: document.getElementById("boot"),
    bootMsg: document.getElementById("boot-msg"),
    rediscover: document.getElementById("rediscover"),
    busPick: document.getElementById("bus-pick"),
    buses: document.getElementById("buses"),
    mixerView: document.getElementById("mixer-view"),
    mixer: document.getElementById("mixer"),
    busTitle: document.getElementById("bus-title"),
    changeBus: document.getElementById("change-bus"),
    stayAwake: document.getElementById("stay-awake"),
    stayAwakeBoot: document.getElementById("stay-awake-boot"),
    meterBallistics: document.getElementById("meter-ballistics"),
    busMasterToggle: document.getElementById("bus-master-toggle"),
    busMaster: document.getElementById("bus-master"),
    helpSheet: document.getElementById("help-sheet"),
    helpBody: document.getElementById("help-body"),
    helpTitle: document.getElementById("help-title"),
    helpClose: document.getElementById("help-close"),
    helpDismiss: document.getElementById("help-dismiss"),
    helpOpen: document.getElementById("help-open"),
    helpOpenBus: document.getElementById("help-open-bus"),
    installHint: document.getElementById("install-hint"),
    panOverlay: document.getElementById("pan-overlay"),
    panOverlayBar: document.getElementById("pan-overlay-bar"),
    panOverlayName: document.getElementById("pan-overlay-name"),
    panOverlayValue: document.getElementById("pan-overlay-value"),
    liveDot: document.getElementById("live-dot"),
  };

  const SID_KEY = "x32mix_sid";
  const BUS_KEY = "x32mix_bus";
  const MASTER_KEY = "x32mix_master";
  const MASTER_CH = 0; // synthetic id for bus master strip
  let sessionId = localStorage.getItem(SID_KEY) || "";
  let lastPhase = "";
  let didAutoRestore = false;
  const strips = new Map();
  let masterRefs = null;
  let selectedBusNum = 0;
  let masterVisible = localStorage.getItem(MASTER_KEY) === "1";

  // Side gutters for camera / Dynamic Island (phone landscape).
  // Must be on .mixer-view — horizontal padding on overflow-.mixer is ignored by WebKit.
  function readSafeAreaInset(side) {
    const probe = document.createElement("div");
    probe.style.cssText =
      "position:fixed;visibility:hidden;pointer-events:none;padding:0;" +
      (side === "left"
        ? "padding-left:env(safe-area-inset-left,0px);"
        : "padding-right:env(safe-area-inset-right,0px);");
    document.documentElement.appendChild(probe);
    const cs = getComputedStyle(probe);
    const px = parseFloat(side === "left" ? cs.paddingLeft : cs.paddingRight) || 0;
    probe.remove();
    return px;
  }

  function syncMixerGutters() {
    const w = window.innerWidth || 0;
    const h = window.innerHeight || 0;
    const shortSide = Math.min(w, h);
    const landscape = w > h;
    // Phones / small devices in landscape (iPad short side is much larger).
    const phoneLandscape = landscape && shortSide > 0 && shortSide <= 520;
    const safeL = readSafeAreaInset("left");
    const safeR = readSafeAreaInset("right");
    // ~56px clears Dynamic Island / camera housing on recent iPhones.
    const minPhone = 56;
    const start = phoneLandscape ? Math.max(minPhone, safeL) : Math.max(0, safeL);
    const end = phoneLandscape ? Math.max(minPhone, safeR) : Math.max(0, safeR);
    const root = el.mixerView || document.documentElement;
    root.style.setProperty("--mixer-gutter-start", start + "px");
    root.style.setProperty("--mixer-gutter-end", end + "px");
  }

  syncMixerGutters();
  window.addEventListener("resize", syncMixerGutters);
  window.addEventListener("orientationchange", () => {
    setTimeout(syncMixerGutters, 100);
    setTimeout(syncMixerGutters, 400);
  });
  if (window.visualViewport) {
    window.visualViewport.addEventListener("resize", syncMixerGutters);
  }
  // OSC send throttle: trailing debounce made the desk jump; send ~50×/s + flush on release.
  const SEND_INTERVAL_MS = 20;
  const levelPending = new Map(); // channel -> level
  const levelTimers = new Map();
  const panPending = new Map();
  const panTimers = new Map();
  // Track active drag so remote updates don't fight the finger.
  let drag = null; // { channel, kind: "level"|"pan", grabOffset? }

  function sessionHeaders(extra) {
    const h = Object.assign({ "Content-Type": "application/json" }, extra || {});
    if (sessionId) h["X-Session-ID"] = sessionId;
    return h;
  }

  function rememberSession(s) {
    if (s && s.sessionId && s.sessionId !== sessionId) {
      sessionId = s.sessionId;
      localStorage.setItem(SID_KEY, sessionId);
    }
  }

  function savedBus() {
    const n = Number(localStorage.getItem(BUS_KEY));
    return Number.isFinite(n) && n > 0 ? n : 0;
  }

  function rememberBus(bus) {
    if (bus > 0) localStorage.setItem(BUS_KEY, String(bus));
  }

  const PAN_PAD = 10;
  let panPad = null; // { channel, refs, pointerId, pad }

  function formatPanLabel(v) {
    const n = clamp01(Number(v) || 0);
    if (Math.abs(n - 0.5) < 0.015) return "C";
    if (n < 0.5) return "L" + Math.round((0.5 - n) * 100);
    return "R" + Math.round((n - 0.5) * 100);
  }

  function syncPanOverlay(pan) {
    const v = clamp01(Number(pan) || 0);
    if (el.panOverlayBar) {
      el.panOverlayBar.style.setProperty("--pan", String(v));
      el.panOverlayBar.setAttribute("aria-valuenow", String(Math.round(v * 100)));
    }
    if (el.panOverlayValue) el.panOverlayValue.textContent = formatPanLabel(v);
  }

  function setPanValue(refs, pan) {
    const v = clamp01(Number(pan) || 0);
    refs.panValue = v;
    refs.pan.style.setProperty("--pan", String(v));
    refs.pan.setAttribute("aria-valuenow", String(Math.round(v * 100)));
    if (panPad && panPad.refs === refs) syncPanOverlay(v);
  }

  function panFromPointer(barEl, clientX, pad) {
    const rect = barEl.getBoundingClientRect();
    const p = pad != null ? pad : 2;
    const usable = Math.max(1, rect.width - p * 2);
    return clamp01((clientX - rect.left - p) / usable);
  }

  function closePanPad() {
    if (!panPad) return;
    const { channel, refs, pointerId } = panPad;
    panPad = null;
    drag = null;
    refs.pan.classList.remove("dragging");
    if (el.panOverlay) el.panOverlay.hidden = true;
    document.body.classList.remove("pan-pad-open");
    flushPan(channel);
    if (pointerId != null && el.panOverlayBar) {
      try {
        el.panOverlayBar.releasePointerCapture(pointerId);
      } catch (_) {
        /* ignore */
      }
    }
  }

  function openPanPad(refs, channel) {
    if (!el.panOverlayBar || !el.panOverlay) return;
    if (panPad) closePanPad();

    const name = (refs.name && refs.name.textContent) || `Ch ${channel}`;
    if (el.panOverlayName) el.panOverlayName.textContent = name;
    syncPanOverlay(refs.panValue ?? 0.5);

    el.panOverlay.hidden = false;
    document.body.classList.add("pan-pad-open");
    refs.pan.classList.add("dragging");
    panPad = { channel, refs, pointerId: null, pad: PAN_PAD };
  }

  function applyPanPadPointer(ev) {
    if (!panPad || !el.panOverlayBar) return;
    const pan = panFromPointer(el.panOverlayBar, ev.clientX, panPad.pad);
    setPanValue(panPad.refs, pan);
    queuePan(panPad.channel, pan);
  }

  if (el.panOverlayBar) {
    el.panOverlayBar.addEventListener("pointerdown", (ev) => {
      if (!panPad) return;
      if (ev.button != null && ev.button !== 0) return;
      panPad.pointerId = ev.pointerId;
      drag = { channel: panPad.channel, kind: "pan", expanded: true };
      applyPanPadPointer(ev);
      try {
        el.panOverlayBar.setPointerCapture(ev.pointerId);
      } catch (_) {
        /* ignore */
      }
      ev.preventDefault();
    });
    el.panOverlayBar.addEventListener("pointermove", (ev) => {
      if (!panPad || panPad.pointerId !== ev.pointerId) return;
      applyPanPadPointer(ev);
      ev.preventDefault();
    });
    const endPadDrag = (ev) => {
      if (!panPad || panPad.pointerId !== ev.pointerId) return;
      closePanPad();
      ev.preventDefault();
    };
    el.panOverlayBar.addEventListener("pointerup", endPadDrag);
    el.panOverlayBar.addEventListener("pointercancel", endPadDrag);
  }

  if (el.panOverlay) {
    el.panOverlay.addEventListener("pointerdown", (ev) => {
      // Tap outside the bar closes the popup.
      if (ev.target === el.panOverlay) {
        closePanPad();
        ev.preventDefault();
      }
    });
  }

  function bindPan(refs, channel) {
    const elPan = refs.pan;
    elPan.style.cursor = "pointer";
    elPan.addEventListener("click", (ev) => {
      ev.preventDefault();
      ev.stopPropagation();
      openPanPad(refs, channel);
    });
  }

  function isDragging(channel, kind) {
    return !!(drag && drag.channel === channel && drag.kind === kind);
  }

  function clamp01(v) {
    return Math.min(1, Math.max(0, v));
  }

  function setStripIcon(el, iconIdOrSrc) {
    // Accept numeric X32 index (1–74) or "/icons/N.svg"; blank (1 / empty) keeps layout space.
    let src = "";
    if (typeof iconIdOrSrc === "number" && Number.isFinite(iconIdOrSrc)) {
      const id = Math.trunc(iconIdOrSrc);
      if (id >= 2 && id <= 74) src = `/icons/${id}.png`;
    } else {
      const s = String(iconIdOrSrc || "").trim();
      if (s.startsWith("/icons/") && /\.(png|svg)$/i.test(s)) src = s;
    }
    if (!src) {
      el.replaceChildren();
      el.classList.add("is-blank");
      return;
    }
    el.classList.remove("is-blank");
    let img = el.querySelector("img");
    if (!img) {
      img = document.createElement("img");
      img.alt = "";
      img.draggable = false;
      el.replaceChildren(img);
    }
    if (img.getAttribute("src") !== src) img.src = src;
  }

  // X32/M32 OSC fader law (pseudo-log): 0 → −∞, 0.75 → 0 dB, 1 → +10 dB.
  function levelToDb(f) {
    const x = Number(f) || 0;
    if (x <= 0) return -Infinity;
    if (x >= 0.5) return x * 40 - 30;
    if (x >= 0.25) return x * 80 - 50;
    if (x >= 0.0625) return x * 160 - 70;
    return x * 640 - 90;
  }

  function dbToLevel(db) {
    if (!Number.isFinite(db) || db <= -90) return 0;
    if (db < -60) return (db + 90) / 640;
    if (db < -30) return (db + 70) / 160;
    if (db < -10) return (db + 50) / 80;
    if (db <= 10) return (db + 30) / 40;
    return 1;
  }

  // Mixing Station–style X32 fader scale (top +10 … bottom −60).
  const FADER_SCALE_DB = [10, 5, 0, -5, -10, -20, -30, -40, -50, -60];

  function formatLevel(v) {
    const db = levelToDb(v);
    if (!Number.isFinite(db) || db <= -89.5) return "-∞";
    const r = Math.round(db);
    if (r > 0) return `+${r}`;
    return String(r);
  }

  function setFaderLevel(refs, level) {
    const v = clamp01(Number(level) || 0);
    refs.level = v;
    refs.wrap.style.setProperty("--level", String(v));
    const db = levelToDb(v);
    refs.wrap.setAttribute(
      "aria-valuenow",
      Number.isFinite(db) ? String(Math.round(db * 10) / 10) : "-inf"
    );
    if (refs.value) refs.value.textContent = formatLevel(v);
  }

  function cssLen(value, rem) {
    const v = String(value || "").trim();
    if (v.endsWith("rem")) return parseFloat(v) * rem;
    if (v.endsWith("px")) return parseFloat(v);
    const n = parseFloat(v);
    return Number.isFinite(n) ? n : 0;
  }

  function levelFromPointer(wrap, clientY) {
    const rect = wrap.getBoundingClientRect();
    const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
    const styles = getComputedStyle(wrap);
    const capH = cssLen(styles.getPropertyValue("--cap-h"), rem) || 1.15 * rem;
    const trackInset = cssLen(styles.getPropertyValue("--track-inset"), rem) || 0.45 * rem;
    const inset = trackInset + capH / 2;
    const usable = Math.max(1, rect.height - inset * 2);
    const y = clientY - rect.top - inset;
    return clamp01(1 - y / usable);
  }

  function bindFader(refs, channel) {
    const wrap = refs.wrap;
    const cap = refs.cap;
    const onMove = (ev) => {
      if (!isDragging(channel, "level")) return;
      const offset = drag && drag.grabOffset != null ? drag.grabOffset : 0;
      const level = levelFromPointer(wrap, ev.clientY - offset);
      setFaderLevel(refs, level);
      queueLevel(channel, level);
    };
    const onEnd = (ev) => {
      if (!isDragging(channel, "level")) return;
      drag = null;
      wrap.classList.remove("dragging");
      cap.classList.remove("dragging");
      flushLevel(channel);
      try {
        wrap.releasePointerCapture(ev.pointerId);
      } catch (_) {
        /* ignore */
      }
    };
    // Only the fader knob starts a drag — clicks on the track do nothing.
    cap.addEventListener("pointerdown", (ev) => {
      if (ev.button != null && ev.button !== 0) return;
      const capRect = cap.getBoundingClientRect();
      const grabOffset = ev.clientY - (capRect.top + capRect.height / 2);
      drag = { channel, kind: "level", grabOffset };
      wrap.classList.add("dragging");
      cap.classList.add("dragging");
      try {
        wrap.setPointerCapture(ev.pointerId);
      } catch (_) {
        /* ignore */
      }
      ev.preventDefault();
      ev.stopPropagation();
    });
    wrap.addEventListener("pointermove", onMove);
    wrap.addEventListener("pointerup", onEnd);
    wrap.addEventListener("pointercancel", onEnd);
  }

  function applyChannelColor(refs, css) {
    const color = css || "";
    refs.colorCss = color;
    // Darken channel color so labels stay readable on the strip.
    const dark = color
      ? `color-mix(in srgb, ${color} 38%, #1a1a1a 62%)`
      : "";
    refs.strip.style.background = dark;
    refs.scribble.style.background = dark || "var(--scribble)";
  }

  function show(node, on) {
    node.hidden = !on;
  }

  function escapeHtml(s) {
    return String(s)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;");
  }

  function renderBuses(buses, loading) {
    el.buses.replaceChildren();
    if (!buses) return;
    for (const bus of buses) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "bus-pick";
      btn.disabled = !!loading;
      btn.innerHTML = `<span class="bus-num">${bus.number}</span><span class="bus-name">${escapeHtml(bus.name || `Bus ${bus.number}`)}</span>`;
      btn.addEventListener("click", () => selectBus(bus.number));
      li.append(btn);
      el.buses.append(li);
    }
  }

  async function selectBus(bus) {
    rememberBus(bus);
    for (const btn of el.buses.querySelectorAll("button")) btn.disabled = true;
    try {
      await fetch("/api/select-bus", {
        method: "POST",
        headers: sessionHeaders(),
        body: JSON.stringify({ bus }),
      });
    } catch (_) {
      for (const btn of el.buses.querySelectorAll("button")) btn.disabled = false;
    }
  }

  function buildStrip(ch, opts) {
    const isMaster = !!(opts && opts.isMaster);
    const channelId = isMaster ? MASTER_CH : ch.number;

    const strip = document.createElement("div");
    strip.className = "strip" + (isMaster ? " is-master" : "");

    const head = document.createElement("div");
    head.className = "strip-head";
    const icon = document.createElement("div");
    icon.className = "strip-icon";
    const num = document.createElement("div");
    num.className = "strip-num";
    num.textContent = isMaster ? "M" : String(ch.number);
    head.append(icon, num);
    setStripIcon(icon, ch.iconSrc || ch.icon);

    const pan = document.createElement("div");
    pan.className = "pan";
    pan.setAttribute("role", "slider");
    pan.setAttribute("aria-label", `Pan ${ch.name || ch.number}`);
    pan.setAttribute("aria-orientation", "horizontal");
    pan.setAttribute("aria-valuemin", "0");
    pan.setAttribute("aria-valuemax", "100");
    pan.tabIndex = 0;
    const panCenter = document.createElement("div");
    panCenter.className = "pan-center";
    panCenter.setAttribute("aria-hidden", "true");
    const panThumb = document.createElement("div");
    panThumb.className = "pan-thumb";
    panThumb.setAttribute("aria-hidden", "true");
    pan.append(panCenter, panThumb);

    const mute = document.createElement("button");
    mute.type = "button";
    mute.className = "mute" + (ch.on ? "" : " muted");
    mute.textContent = "M";
    mute.setAttribute("aria-label", "Mute");
    mute.addEventListener("click", () => {
      const currentlyOn = !mute.classList.contains("muted");
      if (isMaster) postJSON("/api/bus-mute", { on: !currentlyOn });
      else postJSON("/api/mute", { channel: ch.number, on: !currentlyOn });
    });

    const valueWrap = document.createElement("div");
    valueWrap.className = "slider-value";
    const value = document.createElement("div");
    value.className = "slider-value__wrap";
    valueWrap.append(value);

    const wrap = document.createElement("div");
    wrap.className = "fader-wrap";
    wrap.setAttribute("role", "slider");
    wrap.setAttribute("aria-label", `Level ${ch.name || ch.number}`);
    wrap.setAttribute("aria-orientation", "vertical");
    wrap.setAttribute("aria-valuemin", "-90");
    wrap.setAttribute("aria-valuemax", "10");
    wrap.tabIndex = 0;

    const faderCol = document.createElement("div");
    faderCol.className = "fader-col";
    const groove = document.createElement("div");
    groove.className = "fader-groove";
    const cap = document.createElement("div");
    cap.className = "fader-cap";
    faderCol.append(groove, cap);

    const vca = document.createElement("div");
    vca.className = "vca";
    vca.setAttribute("aria-hidden", "true");
    const meter = document.createElement("div");
    meter.className = "fader-meter";
    const meterFill = document.createElement("div");
    meterFill.className = "fader-meter-fill";
    const meterPeakEl = document.createElement("div");
    meterPeakEl.className = "fader-meter-peak";
    meter.append(meterFill, meterPeakEl);
    const markers = document.createElement("div");
    markers.className = "vca-markers";
    for (const db of FADER_SCALE_DB) {
      const m = document.createElement("div");
      m.className = "vca-marker";
      m.textContent = db > 0 ? String(db) : db === 0 ? "0" : String(db);
      m.style.bottom = `${dbToLevel(db) * 100}%`;
      markers.append(m);
    }
    vca.append(meter, markers);

    wrap.append(faderCol, vca);

    const scribble = document.createElement("div");
    scribble.className = "scribble";
    const name = document.createElement("span");
    name.className = "scribble-name";
    name.textContent = ch.name || (isMaster ? "Master" : `Ch ${ch.number}`);
    scribble.append(name);

    strip.append(head, mute, pan, valueWrap, wrap, scribble);

    const refs = {
      strip,
      wrap,
      cap,
      pan,
      mute,
      icon,
      name,
      scribble,
      value,
      meterFill,
      meterPeak: meterPeakEl,
      colorCss: ch.colorCss || "",
      level: 0,
      panValue: 0.5,
      isMaster,
    };
    applyChannelColor(refs, refs.colorCss);
    setFaderLevel(refs, ch.level ?? 0);
    setPanValue(refs, ch.pan ?? 0.5);
    bindFader(refs, channelId);
    bindPan(refs, channelId);
    return refs;
  }

  function syncMasterToggleBtn() {
    const btn = el.busMasterToggle;
    if (!btn) return;
    btn.classList.toggle("on", masterVisible);
    btn.setAttribute("aria-pressed", masterVisible ? "true" : "false");
    btn.textContent = masterVisible ? "Master on" : "Master off";
  }

  function setMasterVisible(on) {
    masterVisible = !!on;
    localStorage.setItem(MASTER_KEY, masterVisible ? "1" : "0");
    syncMasterToggleBtn();
    updateBusMasterVisibility();
  }

  function updateBusMasterVisibility() {
    if (!el.busMaster) return;
    const showMaster = masterVisible && !!masterRefs;
    show(el.busMaster, showMaster);
  }

  function clearBusMaster() {
    masterRefs = null;
    if (el.busMaster) {
      el.busMaster.replaceChildren();
      show(el.busMaster, false);
    }
  }

  function renderBusMaster(master) {
    if (!el.busMaster) return;
    if (!master) {
      clearBusMaster();
      return;
    }
    const ch = {
      number: master.number,
      name: master.name || `Bus ${master.number}`,
      level: master.level,
      pan: master.pan,
      on: master.on,
      colorCss: master.colorCss,
    };
    if (!masterRefs || lastPhase !== "mixing") {
      el.busMaster.replaceChildren();
      masterRefs = buildStrip(ch, { isMaster: true });
      el.busMaster.append(masterRefs.strip);
    } else {
      if (!isDragging(MASTER_CH, "level")) setFaderLevel(masterRefs, ch.level ?? 0);
      if (!isDragging(MASTER_CH, "pan")) setPanValue(masterRefs, ch.pan ?? 0.5);
      masterRefs.mute.classList.toggle("muted", !ch.on);
      if (masterRefs.name) masterRefs.name.textContent = ch.name;
      applyChannelColor(masterRefs, ch.colorCss || "");
    }
    updateBusMasterVisibility();
  }

  function renderMixer(channels) {
    if (!channels) {
      strips.clear();
      el.mixer.replaceChildren();
      clearBusMaster();
      return;
    }

    const needRebuild =
      !strips.size ||
      strips.size !== channels.length ||
      lastPhase !== "mixing";

    if (needRebuild) {
      el.mixer.replaceChildren();
      strips.clear();
      for (const ch of channels) {
        const refs = buildStrip(ch, null);
        el.mixer.append(refs.strip);
        strips.set(ch.number, refs);
      }
    } else {
      for (const ch of channels) {
        const refs = strips.get(ch.number);
        if (!refs) continue;
        if (!isDragging(ch.number, "level")) {
          setFaderLevel(refs, ch.level ?? 0);
        }
        if (!isDragging(ch.number, "pan")) {
          setPanValue(refs, ch.pan ?? 0.5);
        }
        refs.mute.classList.toggle("muted", !ch.on);
        setStripIcon(refs.icon, ch.iconSrc || ch.icon);
        if (refs.name) refs.name.textContent = ch.name || `Ch ${ch.number}`;
        applyChannelColor(refs, ch.colorCss || "");
      }
    }
  }

  function queueThrottled(pending, timers, channel, value, sendFn) {
    pending.set(channel, value);
    if (timers.has(channel)) return;
    const tick = () => {
      const v = pending.get(channel);
      if (v == null) {
        timers.delete(channel);
        return;
      }
      pending.delete(channel);
      sendFn(channel, v);
      timers.set(
        channel,
        setTimeout(() => {
          if (pending.has(channel)) tick();
          else timers.delete(channel);
        }, SEND_INTERVAL_MS)
      );
    };
    timers.set(channel, setTimeout(tick, 0));
  }

  function flushThrottled(pending, timers, channel, sendFn) {
    const t = timers.get(channel);
    if (t) clearTimeout(t);
    timers.delete(channel);
    const v = pending.get(channel);
    pending.delete(channel);
    if (v != null) sendFn(channel, v);
  }

  function queueLevel(channel, level) {
    queueThrottled(levelPending, levelTimers, channel, level, (ch, v) => {
      if (ch === MASTER_CH) postJSON("/api/bus-level", { level: v });
      else postJSON("/api/level", { channel: ch, level: v });
    });
  }

  function flushLevel(channel) {
    flushThrottled(levelPending, levelTimers, channel, (ch, v) => {
      if (ch === MASTER_CH) postJSON("/api/bus-level", { level: v });
      else postJSON("/api/level", { channel: ch, level: v });
    });
  }

  function queuePan(channel, pan) {
    queueThrottled(panPending, panTimers, channel, pan, (ch, v) => {
      if (ch === MASTER_CH) postJSON("/api/bus-pan", { pan: v });
      else postJSON("/api/pan", { channel: ch, pan: v });
    });
  }

  function flushPan(channel) {
    flushThrottled(panPending, panTimers, channel, (ch, v) => {
      if (ch === MASTER_CH) postJSON("/api/bus-pan", { pan: v });
      else postJSON("/api/pan", { channel: ch, pan: v });
    });
  }

  async function postJSON(url, body) {
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: sessionHeaders(),
        body: JSON.stringify(body),
      });
      const sid = res.headers.get("X-Session-ID");
      if (sid) {
        sessionId = sid;
        localStorage.setItem(SID_KEY, sessionId);
      }
    } catch (_) {
      /* ignore */
    }
  }

  function applyStatus(s) {
    rememberSession(s);
    const phase = s.phase || "idle";
    const msg = s.message || "";
    const loading = phase === "searching" || /load/i.test(msg);

    if (phase === "mixing") {
      didAutoRestore = true;
      if (s.selectedBus) rememberBus(s.selectedBus);
      selectedBusNum = s.selectedBus || 0;
      show(el.boot, false);
      show(el.busPick, false);
      show(el.mixerView, true);
      el.busTitle.textContent = s.selectedBusName || `Bus ${s.selectedBus}`;
      renderMixer(s.channels || []);
      renderBusMaster(s.busMaster || null);
    } else if (phase === "connected") {
      const bus = savedBus();
      const canRestore =
        !didAutoRestore &&
        !loading &&
        bus > 0 &&
        (s.buses || []).some((b) => b.number === bus);
      if (canRestore) {
        didAutoRestore = true;
        selectBus(bus);
        show(el.mixerView, false);
        show(el.busPick, false);
        show(el.boot, true);
        el.bootMsg.textContent = "Loading last mix…";
        el.rediscover.hidden = true;
        lastPhase = phase;
        return;
      }
      if (!loading) didAutoRestore = true;

      show(el.mixerView, false);
      show(el.boot, loading);
      show(el.busPick, !loading && !!(s.buses && s.buses.length));
      el.bootMsg.textContent = msg || "Loading…";
      el.rediscover.hidden = true;
      renderBuses(s.buses, loading);
      renderMixer(null);
      if (!loading) updateInstallHint();
    } else if (phase === "error") {
      show(el.mixerView, false);
      show(el.busPick, false);
      show(el.boot, true);
      el.bootMsg.textContent = msg || "Error";
      el.rediscover.hidden = false;
      renderMixer(null);
    } else {
      show(el.mixerView, false);
      show(el.busPick, false);
      show(el.boot, true);
      el.bootMsg.textContent = msg || "Connecting…";
      el.rediscover.hidden = true;
      renderMixer(null);
    }

    lastPhase = phase;
  }

  // X32 /meters: linear FS (1.0 = 0 dBFS). Bar height −60…0 dBFS → 0…1
  // (colours on the track are separate and aligned to the fader scale marks).
  function linearToMeterHeight(linear) {
    if (!(linear > 1e-9)) return 0;
    const db = 20 * Math.log10(linear);
    const minDb = -60;
    const maxDb = 0;
    return Math.min(1, Math.max(0, (db - minDb) / (maxDb - minDb)));
  }

  // Indices 0–31 = input channels; 32 = bus master (when shown).
  const meterTarget = new Float32Array(33);
  const meterShown = new Float32Array(33);
  const meterPeak = new Float32Array(33);
  const meterPeakUntil = new Float64Array(33);
  const MASTER_METER_I = 32;
  const METER_VU_KEY = "x32mix_meter_vu";
  let meterVu = localStorage.getItem(METER_VU_KEY) === "1";

  function meterBallistics() {
    // Fast = peak-ish; VU = sluggish attack/release (classic needle feel).
    if (meterVu) {
      return { attack: 0.1, release: 0.055, peakHoldMs: 1800, peakFall: 0.018 };
    }
    return { attack: 1, release: 0.38, peakHoldMs: 1200, peakFall: 0.045 };
  }

  function syncMeterBallisticsBtn() {
    const btn = el.meterBallistics;
    if (!btn) return;
    btn.classList.toggle("on", meterVu);
    btn.setAttribute("aria-pressed", meterVu ? "true" : "false");
    btn.textContent = meterVu ? "VU on" : "VU off";
  }

  function setMeterVu(on) {
    meterVu = !!on;
    localStorage.setItem(METER_VU_KEY, meterVu ? "1" : "0");
    syncMeterBallisticsBtn();
  }

  let meterRaf = 0;

  function paintMeters() {
    meterRaf = 0;
    const now = performance.now();
    const bal = meterBallistics();
    let cont = false;
    for (let i = 0; i < 33; i++) {
      const t = meterTarget[i];
      let s = meterShown[i];
      if (t >= s) {
        s += (t - s) * bal.attack;
        if (bal.attack >= 0.999) s = t;
      } else {
        s += (t - s) * bal.release;
        if (s < 0.001) s = 0;
      }
      meterShown[i] = s;
      if (Math.abs(t - s) > 0.0008) cont = true;

      let pk = meterPeak[i];
      if (s >= pk) {
        pk = s;
        meterPeakUntil[i] = now + bal.peakHoldMs;
      } else if (now >= meterPeakUntil[i]) {
        // After hold: fall toward the current meter (catches the next peak on the way).
        pk -= bal.peakFall;
        if (pk < s) pk = s;
        if (pk < 0.001) pk = 0;
        cont = true;
      } else {
        cont = true; // still holding
      }
      meterPeak[i] = pk;

      const refs = i === MASTER_METER_I ? masterRefs : strips.get(i + 1);
      if (refs && refs.meterFill) {
        refs.meterFill.style.setProperty("--meter", s.toFixed(3));
      }
      if (refs && refs.meterPeak) {
        refs.meterPeak.style.bottom = `${pk * 100}%`;
        refs.meterPeak.style.opacity = pk > 0.004 ? "1" : "0";
      }
    }
    if (cont) meterRaf = requestAnimationFrame(paintMeters);
  }

  if (el.meterBallistics) {
    el.meterBallistics.addEventListener("click", () => setMeterVu(!meterVu));
    syncMeterBallisticsBtn();
  }

  if (el.busMasterToggle) {
    el.busMasterToggle.addEventListener("click", () => setMasterVisible(!masterVisible));
    syncMasterToggleBtn();
  }

  function applyMeters(levels) {
    if (!Array.isArray(levels)) return;
    if (!strips.size && !masterRefs) return;
    for (let i = 0; i < 32; i++) {
      const raw = Number(levels[i]);
      meterTarget[i] = linearToMeterHeight(Number.isFinite(raw) ? Math.max(0, raw) : 0);
    }
    // /meters/0: 32 ch + 8 aux + 8 FX + 16 buses
    if (masterRefs && selectedBusNum > 0) {
      const bi = 32 + 8 + 8 + (selectedBusNum - 1);
      if (bi < levels.length) {
        const raw = Number(levels[bi]);
        meterTarget[MASTER_METER_I] = linearToMeterHeight(
          Number.isFinite(raw) ? Math.max(0, raw) : 0
        );
      }
    }
    if (!meterRaf) meterRaf = requestAnimationFrame(paintMeters);
  }

  let wsReconnectTimer = null;
  let activeWS = null;

  function setLiveOnline(online) {
    if (!el.liveDot) return;
    el.liveDot.classList.toggle("is-offline", !online);
    el.liveDot.title = online ? "Online" : "Offline – server unreachable";
    el.liveDot.setAttribute("aria-label", online ? "Online" : "Offline");
  }

  function connectWS() {
    if (wsReconnectTimer) {
      clearTimeout(wsReconnectTimer);
      wsReconnectTimer = null;
    }
    if (activeWS) {
      try {
        activeWS.onopen = null;
        activeWS.onclose = null;
        activeWS.onerror = null;
        activeWS.onmessage = null;
        activeWS.close();
      } catch (_) {
        /* ignore */
      }
      activeWS = null;
    }

    setLiveOnline(false);
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const q = sessionId ? `?sid=${encodeURIComponent(sessionId)}` : "";
    let ws;
    try {
      ws = new WebSocket(`${proto}://${location.host}/ws${q}`);
    } catch (_) {
      wsReconnectTimer = setTimeout(connectWS, 1200);
      return;
    }
    activeWS = ws;
    ws.onopen = () => {
      if (activeWS === ws) setLiveOnline(true);
    };
    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === "status") applyStatus(msg.data);
        else if (msg.type === "meters") applyMeters(msg.data && msg.data.levels);
      } catch (_) {
        /* ignore */
      }
    };
    ws.onerror = () => {
      // onclose follows; mark offline immediately
      if (activeWS === ws) setLiveOnline(false);
    };
    ws.onclose = () => {
      if (activeWS === ws) {
        activeWS = null;
        setLiveOnline(false);
        wsReconnectTimer = setTimeout(connectWS, 1200);
      }
    };
  }

  el.rediscover.addEventListener("click", () => {
    fetch("/api/discover", { method: "POST", headers: sessionHeaders() });
  });

  el.changeBus.addEventListener("click", () => {
    didAutoRestore = true; // don't immediately re-open the last mix
    fetch("/api/clear-bus", { method: "POST", headers: sessionHeaders() });
  });

  // —— PWA: stay awake ——
  // Modern iPhone: Screen Wake Lock needs HTTPS. HTTP: muted video often ignored.
  const AWAKE_KEY = "x32mix_awake";
  let stayAwakeOn = localStorage.getItem(AWAKE_KEY) === "1";
  let wakeLock = null;
  let wakeTimer = null;
  const wakeVideo = document.getElementById("wake-video");
  const isIOS = /iPhone|iPad|iPod/i.test(navigator.userAgent);
  const isSecure = window.isSecureContext;

  function syncAwakeButtons() {
    for (const btn of [el.stayAwake, el.stayAwakeBoot]) {
      if (!btn) continue;
      btn.classList.toggle("on", stayAwakeOn);
      btn.setAttribute("aria-pressed", stayAwakeOn ? "true" : "false");
      btn.textContent = stayAwakeOn ? "Awake on" : "Awake off";
      btn.hidden = false;
    }
  }

  async function requestWakeLock() {
    if (!stayAwakeOn || !("wakeLock" in navigator) || !isSecure) return false;
    try {
      if (wakeLock) return true;
      wakeLock = await navigator.wakeLock.request("screen");
      wakeLock.addEventListener("release", () => {
        wakeLock = null;
      });
      return true;
    } catch (_) {
      return false;
    }
  }

  function ensureWakeVideo() {
    if (!wakeVideo) return;
    if (!wakeVideo.querySelector("source")) {
      const s1 = document.createElement("source");
      s1.src = "/wake-ios.mp4";
      s1.type = "video/mp4";
      const s2 = document.createElement("source");
      s2.src = "/wake-ios.webm";
      s2.type = "video/webm";
      wakeVideo.append(s1, s2);
      wakeVideo.load();
    }
    wakeVideo.setAttribute("playsinline", "");
    wakeVideo.setAttribute("webkit-playsinline", "");
    wakeVideo.muted = true;
    wakeVideo.loop = true;
    // NoSleep trick: nudge currentTime so iOS keeps treating it as active media.
    if (!wakeVideo._nudgeBound) {
      wakeVideo._nudgeBound = true;
      wakeVideo.addEventListener("timeupdate", () => {
        if (!stayAwakeOn) return;
        try {
          if (wakeVideo.currentTime > 0.5) {
            wakeVideo.currentTime = Math.random() * 0.4;
          }
        } catch (_) {
          /* ignore */
        }
      });
      wakeVideo.addEventListener("pause", () => {
        if (stayAwakeOn) {
          const p = wakeVideo.play();
          if (p && p.catch) p.catch(() => {});
        }
      });
    }
  }

  async function startWakeVideo() {
    if (!wakeVideo || !stayAwakeOn) return;
    ensureWakeVideo();
    try {
      await wakeVideo.play();
    } catch (_) {
      /* needs gesture */
    }
  }

  function stopWakeVideo() {
    if (!wakeVideo) return;
    try {
      wakeVideo.pause();
    } catch (_) {
      /* ignore */
    }
  }

  async function enableStayAwake() {
    if (!stayAwakeOn) return;
    const locked = await requestWakeLock();
    // Always start video on iOS; also as fallback when Wake Lock fails (HTTP).
    if (isIOS || !locked) await startWakeVideo();
  }

  function disableStayAwake() {
    if (wakeLock) {
      try {
        wakeLock.release();
      } catch (_) {
        /* ignore */
      }
      wakeLock = null;
    }
    stopWakeVideo();
    if (wakeTimer) {
      clearInterval(wakeTimer);
      wakeTimer = null;
    }
  }

  async function setStayAwake(on) {
    stayAwakeOn = !!on;
    localStorage.setItem(AWAKE_KEY, stayAwakeOn ? "1" : "0");
    syncAwakeButtons();
    if (stayAwakeOn) {
      await enableStayAwake();
      if (!wakeTimer) {
        wakeTimer = setInterval(() => {
          if (stayAwakeOn) enableStayAwake();
        }, 15000);
      }
      if (isIOS && !isSecure && !localStorage.getItem("x32mix_https_hint")) {
        localStorage.setItem("x32mix_https_hint", "1");
        window.alert(
          "On iPhone, only HTTPS keeps the screen awake reliably (Wake Lock).\n\n" +
            "Start the server with -https :8443, open https://<IP>:8443, and accept the certificate once.\n\n" +
            "Or use Settings → Display & Brightness → Auto-Lock → Never."
        );
      }
    } else {
      disableStayAwake();
    }
  }

  for (const btn of [el.stayAwake, el.stayAwakeBoot]) {
    if (!btn) continue;
    btn.addEventListener("click", (ev) => {
      ev.preventDefault();
      setStayAwake(!stayAwakeOn);
    });
  }
  syncAwakeButtons();

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && stayAwakeOn) enableStayAwake();
  });

  if ("serviceWorker" in navigator) {
    window.addEventListener("load", () => {
      navigator.serviceWorker.register("/sw.js").then((reg) => {
        reg.update().catch(() => {});
      }).catch(() => {});
    });
  }

  // —— Install / PWA help (iOS vs Android) ——
  const HINT_KEY = "x32mix_install_hint";
  const ua = navigator.userAgent || "";
  const platform = /iPhone|iPad|iPod/i.test(ua)
    ? "ios"
    : /Android/i.test(ua)
      ? "android"
      : "other";

  function isStandaloneApp() {
    return (
      window.matchMedia("(display-mode: standalone)").matches ||
      window.matchMedia("(display-mode: fullscreen)").matches ||
      !!navigator.standalone
    );
  }

  function helpContent() {
    if (platform === "ios") {
      return {
        title: "Install on iPhone",
        html: `
          <span class="help-platform">iOS · Safari</span>
          <p>Add X32 Personal Mix to the Home Screen without the address bar:</p>
          <ol>
            <li>Open the page over <strong>HTTPS</strong> (accept the certificate once).</li>
            <li>Tap <strong>Share</strong> at the bottom (square with arrow).</li>
            <li>Choose <strong>Add to Home Screen</strong> → Add.</li>
            <li>Launch the app from the Home Screen (fullscreen).</li>
            <li>In the mixer, tap <strong>Awake on</strong> to keep the screen on.</li>
          </ol>
          <p class="help-note">Tip: if needed, Settings → Display &amp; Brightness → Auto-Lock → Never.</p>`,
      };
    }
    if (platform === "android") {
      return {
        title: "Install on Android",
        html: `
          <span class="help-platform">Android · Chrome</span>
          <p>Install X32 Personal Mix as an app:</p>
          <ol>
            <li>Open the page over <strong>HTTPS</strong> (accept the certificate once).</li>
            <li>Open the <strong>⋮</strong> menu at the top right.</li>
            <li>Choose <strong>Install app</strong> or <strong>Add to Home screen</strong>.</li>
            <li>Launch from the home screen (no browser chrome).</li>
            <li>In the mixer, tap <strong>Awake on</strong> for long-running use.</li>
          </ol>
          <p class="help-note">If the menu item is missing: update Chrome or reload the page.</p>`,
      };
    }
    return {
      title: "Use as an app",
      html: `
        <span class="help-platform">Desktop / tablet</span>
        <p>Install on your phone — the matching guide appears there.</p>
        <ol>
          <li><strong>iPhone:</strong> Safari → Share → Add to Home Screen.</li>
          <li><strong>Android:</strong> Chrome → menu → Install app.</li>
          <li>Always open over <strong>HTTPS</strong>, then <strong>Awake on</strong> for long-running use.</li>
        </ol>`,
    };
  }

  function openHelp() {
    const c = helpContent();
    if (el.helpTitle) el.helpTitle.textContent = c.title;
    if (el.helpBody) el.helpBody.innerHTML = c.html;
    show(el.helpSheet, true);
  }

  function closeHelp() {
    show(el.helpSheet, false);
  }

  function updateInstallHint() {
    if (!el.installHint) return;
    if (isStandaloneApp() || localStorage.getItem(HINT_KEY) === "1") {
      el.installHint.hidden = true;
      return;
    }
    const label =
      platform === "ios"
        ? "As an app without address bar: Safari → Share → Add to Home Screen."
        : platform === "android"
          ? "As an app: Chrome menu → Install app / Add to Home screen."
          : "Install as an app on your phone — guide via “?”.";
    el.installHint.innerHTML = `
      <div>${label}</div>
      <button type="button" class="btn btn-ghost" data-help>Guide</button>
      <button type="button" class="btn btn-ghost" data-dismiss>Dismiss</button>`;
    el.installHint.hidden = false;
    el.installHint.querySelector("[data-help]")?.addEventListener("click", openHelp);
    el.installHint.querySelector("[data-dismiss]")?.addEventListener("click", () => {
      localStorage.setItem(HINT_KEY, "1");
      el.installHint.hidden = true;
    });
  }

  el.helpOpen?.addEventListener("click", openHelp);
  el.helpOpenBus?.addEventListener("click", openHelp);
  el.helpClose?.addEventListener("click", closeHelp);
  el.helpDismiss?.addEventListener("click", () => {
    localStorage.setItem(HINT_KEY, "1");
    if (el.installHint) el.installHint.hidden = true;
    closeHelp();
  });
  el.helpSheet?.addEventListener("click", (ev) => {
    if (ev.target === el.helpSheet) closeHelp();
  });

  // Hide help entry points when already running as installed PWA.
  if (isStandaloneApp()) {
    if (el.helpOpen) el.helpOpen.hidden = true;
    if (el.helpOpenBus) el.helpOpenBus.hidden = true;
  }

  connectWS();
})();
