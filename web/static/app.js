// SHEYTAN-Local-Agent — frontend controller
// Handles: session management, chat rendering, WebSocket activity streaming,
// modals (model/preset/sysinfo/components), right panel tabs.

(function () {
  "use strict";

  // ---------- State ----------
  const state = {
    config: null,
    sessions: [],
    activeSessionId: null,
    activeSession: null,
    sysinfo: null,
    presets: [],
    models: { local: [], loaded: [], llamaRunning: false },
    ws: null,
    panelTab: "context",
    abortController: null,
  };

  // ---------- Helpers ----------
  const $ = (id) => document.getElementById(id);
  const el = (tag, cls, text) => {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  };
  const fmtTime = (ts) => {
    const d = new Date(ts);
    const now = new Date();
    if (d.toDateString() === now.toDateString()) {
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    }
    return d.toLocaleDateString([], { month: "short", day: "numeric" }) + " " +
           d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  };
  const escape = (s) =>
    String(s || "").replace(/[&<>"']/g, (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

  function toast(msg, kind = "") {
    const t = el("div", "toast " + kind, msg);
    $("toasts").appendChild(t);
    setTimeout(() => {
      t.style.opacity = "0";
      t.style.transition = "opacity 0.2s";
      setTimeout(() => t.remove(), 250);
    }, 3500);
  }

  async function api(path, opts = {}) {
    const r = await fetch(path, {
      method: opts.method || "GET",
      headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    });
    if (!r.ok) {
      const err = await r.json().catch(() => ({ error: r.statusText }));
      throw new Error(err.error || "API error");
    }
    return r.json();
  }

  // ---------- Sessions ----------
  async function loadSessions() {
    state.sessions = await api("/api/sessions");
    renderSessions();
  }

  function renderSessions() {
    const list = $("sessionList");
    list.innerHTML = "";
    const q = ($("sessionSearch").value || "").toLowerCase();
    const filtered = state.sessions.filter((s) =>
      (s.title || "New session").toLowerCase().includes(q) ||
      (s.messages && s.messages.some((m) => (m.content || "").toLowerCase().includes(q)))
    );
    if (filtered.length === 0) {
      list.appendChild(el("div", "session-empty", "No sessions yet"));
      return;
    }
    filtered.forEach((s) => {
      const item = el("div", "session-item");
      if (s.id === state.activeSessionId) item.classList.add("active");
      const title = el("span", "session-title", s.title || "New session");
      const meta = el("span", "session-meta", fmtTime(s.updatedAt || s.createdAt));
      item.appendChild(title);
      item.appendChild(meta);
      item.onclick = () => openSession(s.id);
      list.appendChild(item);
    });
  }

  async function newSession() {
    const s = await api("/api/sessions", { method: "POST" });
    state.sessions.unshift(s);
    state.activeSessionId = s.id;
    state.activeSession = s;
    renderSessions();
    renderActiveSession();
  }

  async function openSession(id) {
    state.activeSessionId = id;
    state.activeSession = await api("/api/sessions/" + id);
    renderSessions();
    renderActiveSession();
  }

  function renderActiveSession() {
    const s = state.activeSession;
    if (!s) {
      $("emptyState").style.display = "block";
      return;
    }
    $("emptyState").style.display = "none";
    $("sessionTitle").textContent = s.title || "New session";
    const msgCount = s.messages ? s.messages.length : 0;
    const last = s.messages && s.messages.length > 0 ? s.messages[s.messages.length - 1] : null;
    const lastKind = last ? last.role : "";
    $("chatMeta").textContent = `${msgCount} msg${msgCount === 1 ? "" : "s"} · ${s.model || state.config?.model || "default"}`;
    renderMessages(s.messages || []);
  }

  function renderMessages(messages) {
    const container = $("messages");
    // Remove existing msgs but keep empty state
    container.querySelectorAll(".msg").forEach((m) => m.remove());
    messages.forEach((m) => container.appendChild(renderMessage(m)));
    container.scrollTop = container.scrollHeight;
  }

  function renderMessage(m) {
    const wrap = el("div", "msg " + (m.role || "user"));
    const avatar = el("div", "msg-avatar", m.role === "user" ? "U" : "S");
    const body = el("div", "msg-body");
    const role = el("div", "msg-role", m.role === "user" ? "You" : "SHEYTAN");
    const content = el("div", "msg-content");
    content.innerHTML = formatContent(m.content || "");
    body.appendChild(role);
    body.appendChild(content);
    wrap.appendChild(avatar);
    wrap.appendChild(body);
    return wrap;
  }

  function formatContent(text) {
    // Very small markdown subset: code blocks, inline code, bold, italic
    let s = escape(text);
    s = s.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) =>
      `<pre><code>${code.replace(/&amp;/g, "&")}</code></pre>`);
    s = s.replace(/`([^`]+)`/g, "<code>$1</code>");
    s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    s = s.replace(/\*([^*]+)\*/g, "<em>$1</em>");
    return s;
  }

  // ---------- Send message + run agent ----------
  async function sendMessage() {
    const input = $("messageInput");
    const text = input.value.trim();
    if (!text) return;
    if (!state.activeSessionId) {
      await newSession();
    }

    // Optimistic: render user message immediately
    if (!state.activeSession.messages) state.activeSession.messages = [];
    state.activeSession.messages.push({ role: "user", content: text });
    renderMessages(state.activeSession.messages);
    input.value = "";
    updateCharCount();
    updateSendBtnState();

    // Show activity strip
    $("activityStrip").hidden = false;
    setActivityCaption("Starting…");
    addActivityLog("plan", "Sending request to model…");
    openWS(state.activeSessionId);

    try {
      await api("/api/run", {
        method: "POST",
        body: { sessionId: state.activeSessionId, message: text },
      });
    } catch (err) {
      toast("Send failed: " + err.message, "error");
      hideActivityStrip();
    }
  }

  function openWS(sessionId) {
    if (state.ws) {
      state.ws.close();
      state.ws = null;
    }
    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${location.host}/ws/activity?sessionId=${encodeURIComponent(sessionId)}`;
    const ws = new WebSocket(url);
    state.ws = ws;
    ws.onmessage = (ev) => {
      let a;
      try { a = JSON.parse(ev.data); } catch { return; }
      handleActivity(a);
    };
    ws.onclose = () => {
      if (state.ws === ws) state.ws = null;
    };
  }

  let liveAssistantText = "";

  function handleActivity(a) {
    addActivityLog(a.type, a.caption);
    setActivityCaption(a.caption);

    if (a.type === "response") {
      // Stream the assistant reply
      if (!liveAssistantText) {
        const wrap = el("div", "msg assistant");
        wrap.id = "live-assistant";
        wrap.appendChild(el("div", "msg-avatar", "S"));
        const body = el("div", "msg-body");
        body.appendChild(el("div", "msg-role", "SHEYTAN"));
        const content = el("div", "msg-content");
        body.appendChild(content);
        wrap.appendChild(body);
        $("messages").appendChild(wrap);
      }
      liveAssistantText = a.caption;
      const node = document.getElementById("live-assistant");
      if (node) {
        node.querySelector(".msg-content").innerHTML = formatContent(liveAssistantText);
        $("messages").scrollTop = $("messages").scrollHeight;
      }
    }

    if (a.type === "done" || a.type === "error") {
      // Finalize: persist the assistant message to local state
      if (liveAssistantText) {
        state.activeSession.messages.push({ role: "assistant", content: liveAssistantText });
        liveAssistantText = "";
        renderMessages(state.activeSession.messages);
      }
      hideActivityStrip();
      loadSessions(); // refresh sidebar timestamp
      openSession(state.activeSessionId);
    }
  }

  function addActivityLog(type, caption) {
    const ul = $("activityLog");
    const li = el("li", type);
    li.appendChild(el("span", "ts", new Date().toLocaleTimeString()));
    li.appendChild(el("span", "cap", caption));
    ul.appendChild(li);
    if (ul.children.length > 200) ul.removeChild(ul.firstChild);
    const det = ul.parentElement;
    if (det && !det.open) det.open = true;
  }

  function setActivityCaption(text) {
    $("activityCaption").textContent = text;
  }

  function hideActivityStrip() {
    $("activityStrip").hidden = true;
    $("activityLog").innerHTML = "";
  }

  // ---------- Abort ----------
  async function abortRun() {
    if (!state.activeSessionId) return;
    try {
      await api("/api/abort", {
        method: "POST",
        body: { sessionId: state.activeSessionId },
      });
      toast("Aborted", "warning");
    } catch (err) {
      toast("Abort failed: " + err.message, "error");
    }
  }

  // ---------- Right panel ----------
  function openPanel(tab) {
    state.panelTab = tab;
    document.querySelectorAll(".panel-tab").forEach((t) => {
      t.classList.toggle("active", t.dataset.tab === tab);
    });
    renderPanel();
    $("rightPanel").hidden = false;
    $("app").classList.add("panel-open");
  }

  function closePanel() {
    $("rightPanel").hidden = true;
    $("app").classList.remove("panel-open");
  }

  async function renderPanel() {
    const c = $("panelContent");
    c.innerHTML = "";
    if (!state.config) state.config = await api("/api/config");
    switch (state.panelTab) {
      case "context": await panelContext(c); break;
      case "params": panelParams(c); break;
      case "sysinfo": await panelSysinfo(c); break;
      case "tools": await panelTools(c); break;
    }
  }

  async function panelContext(c) {
    const sess = state.activeSession || {};
    const ctx = sess.context || {};
    c.appendChild(panelSection("System prompt", () => {
      const ta = el("textarea");
      ta.value = ctx.systemPrompt || "";
      ta.id = "ctxSystemPrompt";
      ta.placeholder = "You are SHEYTAN, an autonomous agent…";
      return ta;
    }));

    c.appendChild(panelSection("Attached files", () => {
      const wrap = el("div");
      const list = ctx.attachedFiles || [];
      list.forEach((f, i) => {
        const row = el("div", "info-row");
        row.appendChild(el("span", "label", f));
        const btn = el("button", "btn-secondary", "✕");
        btn.style.padding = "2px 8px";
        btn.style.fontSize = "10px";
        btn.onclick = async () => {
          const next = list.slice();
          next.splice(i, 1);
          await saveContext({ attachedFiles: next });
          openPanel("context");
        };
        row.appendChild(btn);
        wrap.appendChild(row);
      });
      const addBtn = el("button", "btn-secondary", "+ Add file path");
      addBtn.style.marginTop = "8px";
      addBtn.onclick = async () => {
        const path = prompt("File path to attach:");
        if (!path) return;
        await saveContext({ attachedFiles: [...list, path] });
        openPanel("context");
      };
      wrap.appendChild(addBtn);
      return wrap;
    }));

    c.appendChild(panelSection("Max iterations", () => {
      const input = el("input");
      input.type = "number";
      input.value = ctx.maxIterations || state.config.maxIterations || 25;
      input.id = "ctxMaxIter";
      return input;
    }));

    c.appendChild(panelSection("", () => {
      const btn = el("button", "btn-primary", "Save context");
      btn.onclick = saveContextFromForm;
      return btn;
    }));
  }

  function panelSection(title, render) {
    const s = el("div", "panel-section");
    if (title) s.appendChild(el("h3", null, title));
    s.appendChild(render());
    return s;
  }

  async function saveContext(patch) {
    if (!state.activeSession) return;
    const newCtx = Object.assign({}, state.activeSession.context || {}, patch);
    state.activeSession.context = newCtx;
    await api("/api/sessions/" + state.activeSession.id, {
      method: "PUT",
      body: { context: newCtx },
    });
    toast("Context saved", "success");
  }

  async function saveContextFromForm() {
    const ta = $("ctxSystemPrompt");
    const maxIter = $("ctxMaxIter");
    await saveContext({
      systemPrompt: ta ? ta.value : "",
      maxIterations: maxIter ? parseInt(maxIter.value, 10) : 25,
    });
    openPanel("context");
  }

  function panelParams(c) {
    const llm = state.config.llm;
    const row = (label, val, min, max, step) => {
      const f = el("div", "panel-field");
      f.appendChild(el("label", null, label));
      const sr = el("div", "slider-row");
      const input = el("input");
      input.type = "range";
      input.min = String(min);
      input.max = String(max);
      input.step = String(step);
      input.value = String(val);
      input.oninput = () => v.textContent = input.value;
      const v = el("div", "value", String(val));
      sr.appendChild(input);
      sr.appendChild(v);
      f.appendChild(sr);
      return f;
    };

    c.appendChild(panelSection("Sampling", () => {
      const w = el("div");
      w.appendChild(row("Temperature", llm.temperature, 0, 2, 0.05));
      w.appendChild(row("Top-p", llm.topP, 0, 1, 0.01));
      w.appendChild(row("Top-k", llm.topK, 0, 200, 1));
      w.appendChild(row("Max tokens", llm.maxTokens, 64, 8192, 64));
      w.appendChild(row("Repeat penalty", llm.repeatPenalty, 0.5, 2.0, 0.01));
      return w;
    }));

    c.appendChild(panelSection("Runtime", () => {
      const w = el("div");
      w.appendChild(row("Num ctx", llm.numCtx, 512, 32768, 512));
      w.appendChild(row("Num batch", llm.numBatch, 128, 4096, 64));
      w.appendChild(row("Num thread", llm.numThread, 1, 128, 1));
      w.appendChild(row("Num GPU layers", llm.numGPU, 0, 999, 1));
      w.appendChild(row("Mirostat", llm.mirostat, 0, 2, 1));
      w.appendChild(row("Seed", llm.seed, -1, 999999, 1));
      return w;
    }));

    c.appendChild(panelSection("", () => {
      const b = el("button", "btn-primary", "Save params");
      b.onclick = async () => {
        const sliders = c.querySelectorAll("input[type=range]");
        const values = Array.from(sliders).map((s) => parseFloat(s.value));
        const newLlm = {
          temperature: values[0], topP: values[1], topK: values[2],
          maxTokens: values[3], repeatPenalty: values[4],
          numCtx: values[5], numBatch: values[6], numThread: values[7],
          numGPU: values[8], mirostat: values[9], seed: values[10],
        };
        state.config.llm = Object.assign({}, state.config.llm, newLlm);
        await api("/api/config", { method: "PUT", body: state.config });
        toast("Params saved", "success");
      };
      return b;
    }));
  }

  async function panelSysinfo(c) {
    if (!state.sysinfo) state.sysinfo = await api("/api/sysinfo");
    const info = state.sysinfo;
    const row = (label, val) => {
      const r = el("div", "info-row");
      r.appendChild(el("span", "label", label));
      r.appendChild(el("span", "value", val));
      return r;
    };

    c.appendChild(panelSection("Host", () => {
      const w = el("div");
      w.appendChild(row("OS", `${info.os}/${info.arch}`));
      w.appendChild(row("Hostname", info.hostname));
      w.appendChild(row("CPU", info.cpu.name || "Unknown"));
      w.appendChild(row("Cores", `${info.cpu.physicalCores} physical / ${info.cpu.logicalCores} logical`));
      w.appendChild(row("RAM total", formatBytes(info.ram.totalBytes)));
      w.appendChild(row("RAM free", formatBytes(info.ram.freeBytes)));
      w.appendChild(row("RAM available", formatBytes(info.ram.available)));
      w.appendChild(row("Disk total", formatBytes(info.disk.totalBytes)));
      w.appendChild(row("Disk free", formatBytes(info.disk.freeBytes)));
      return w;
    }));

    c.appendChild(panelSection("GPU(s)", () => {
      const w = el("div");
      if (!info.gpus || info.gpus.length === 0) {
        w.appendChild(el("div", null, "None detected"));
      } else {
        info.gpus.forEach((g) => {
          w.appendChild(row(g.vendor, g.name + (g.vramBytes ? ` (VRAM ${formatBytes(g.vramBytes)})` : "")));
        });
      }
      return w;
    }));

    c.appendChild(panelSection("Flags", () => {
      const w = el("div");
      w.appendChild(row("WSL2", String(info.wsl2)));
      w.appendChild(row("Docker", String(info.docker)));
      return w;
    }));

    c.appendChild(panelSection("Recommended llama.cpp knobs", () => {
      const w = el("div");
      const r = info.recommended;
      w.appendChild(row("num_thread", String(r.numThread)));
      w.appendChild(row("num_gpu", String(r.numGPU)));
      w.appendChild(row("num_ctx", String(r.numCtx)));
      w.appendChild(row("num_batch", String(r.numBatch)));
      w.appendChild(row("max_tokens", String(r.maxTokens)));
      w.appendChild(row("can_run_cpu", String(r.canRunCPU)));
      w.appendChild(row("can_run_gpu", String(r.canRunGPU)));
      if (r.warnings && r.warnings.length) {
        r.warnings.forEach((ww) => {
          const d = el("div", "toast warning");
          d.style.position = "static";
          d.style.margin = "4px 0";
          d.textContent = "⚠ " + ww;
          w.appendChild(d);
        });
      }
      const applyBtn = el("button", "btn-primary", "Apply recommended");
      applyBtn.style.marginTop = "10px";
      applyBtn.onclick = async () => {
        state.config.llm.numThread = r.numThread;
        state.config.llm.numGPU = r.numGPU;
        state.config.llm.numCtx = r.numCtx;
        state.config.llm.numBatch = r.numBatch;
        state.config.llm.maxTokens = r.maxTokens;
        await api("/api/config", { method: "PUT", body: state.config });
        toast("Recommended knobs applied", "success");
        openPanel("params");
      };
      w.appendChild(applyBtn);
      return w;
    }));
  }

  async function panelTools(c) {
    let tools = [];
    try { tools = await api("/api/tools"); } catch {}
    c.appendChild(panelSection("Registered tools", () => {
      const w = el("div");
      tools.forEach((t) => {
        const card = el("div", "tool-card");
        card.appendChild(el("div", "name", t.name));
        card.appendChild(el("div", "desc", t.description));
        w.appendChild(card);
      });
      if (!tools.length) w.appendChild(el("div", null, "No tools registered"));
      return w;
    }));
  }

  function formatBytes(b) {
    if (!b) return "0 B";
    const u = ["B", "KB", "MB", "GB", "TB"];
    let i = 0, n = b;
    while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
    return n.toFixed(1) + " " + u[i];
  }

  // ---------- Modals ----------
  function openModal(id) { $(id).hidden = false; }
  function closeModal(id) { $(id).hidden = true; }

  async function openModelModal() {
    openModal("modelModal");
    const body = $("modelModalBody");
    state.models = await api("/api/models");
    // Local models
    const local = $("localModels");
    local.innerHTML = "";
    if (!state.models.local.length) {
      local.appendChild(el("li", null, "No .gguf files — drop one in " + (state.config.modelsDir || "~/.sheytan/models")));
    }
    state.models.local.forEach((m) => {
      const li = el("li");
      li.textContent = m;
      if (m === state.config.model) li.classList.add("active");
      li.onclick = async () => {
        state.config.model = m;
        await api("/api/config", { method: "PUT", body: state.config });
        $("modelLabel").textContent = m;
        toast("Model set: " + m, "success");
        closeModal("modelModal");
      };
      local.appendChild(li);
    });
    // Loaded models (from running llama.cpp)
    const loaded = $("loadedModels");
    loaded.innerHTML = "";
    if (state.models.loaded && state.models.loaded.length) {
      state.models.loaded.forEach((m) => {
        const li = el("li");
        li.textContent = m;
        loaded.appendChild(li);
      });
    } else {
      loaded.appendChild(el("li", null, "No models loaded"));
    }
    // Llama state
    updateLlamaState(state.models.llamaRunning ? "running" : "stopped");
  }

  function updateLlamaState(st) {
    const el = $("llamaStatus");
    el.textContent = st;
    el.className = "llama-status " + st;
  }

  async function llamaStart() {
    updateLlamaState("starting");
    toast("Starting llama.cpp server…", "");
    try {
      const r = await api("/api/llama", { method: "POST", body: { action: "start" } });
      updateLlamaState(r.state);
      toast("llama.cpp running", "success");
    } catch (err) {
      updateLlamaState("error");
      toast("Start failed: " + err.message, "error");
    }
  }

  async function llamaStop() {
    try {
      const r = await api("/api/llama", { method: "POST", body: { action: "stop" } });
      updateLlamaState(r.state);
      toast("Stopped", "warning");
    } catch (err) {
      toast("Stop failed: " + err.message, "error");
    }
  }

  async function llamaShowLogs() {
    const r = await api("/api/llama");
    const logs = $("llamaLogs");
    logs.hidden = false;
    logs.textContent = (r.logs || []).join("\n");
  }

  async function openPresetModal() {
    openModal("presetModal");
    if (!state.presets.length) state.presets = await api("/api/presets");
    const body = $("presetModalBody");
    body.innerHTML = "";
    state.presets.forEach((p) => {
      const card = el("div", "preset-card");
      if (p.name === state.config.llm.preset) card.classList.add("active");
      card.appendChild(el("div", "name", p.label));
      card.appendChild(el("div", "desc", p.description));
      card.appendChild(el("div", "params",
        `temp=${p.temperature} top_p=${p.topP} top_k=${p.topK} max_tokens=${p.maxTokens} num_ctx=${p.numCtx}`));
      card.onclick = async () => {
        state.config.llm = Object.assign({}, state.config.llm, {
          temperature: p.temperature, topP: p.topP, topK: p.topK,
          maxTokens: p.maxTokens, repeatPenalty: p.repeatPenalty,
          mirostat: p.mirostat, numCtx: p.numCtx, preset: p.name,
        });
        await api("/api/config", { method: "PUT", body: state.config });
        $("presetLabel").textContent = p.name;
        toast("Preset: " + p.label, "success");
        closeModal("presetModal");
      };
      body.appendChild(card);
    });
  }

  async function openSysinfoModal() {
    openModal("sysinfoModal");
    state.sysinfo = await api("/api/sysinfo");
    const body = $("sysinfoModalBody");
    body.innerHTML = "";
    const info = state.sysinfo;
    const card = (title, rows) => {
      const s = el("div", "panel-section");
      s.appendChild(el("h3", null, title));
      const w = el("div");
      rows.forEach(([l, v]) => {
        const r = el("div", "info-row");
        r.appendChild(el("span", "label", l));
        r.appendChild(el("span", "value", v));
        w.appendChild(r);
      });
      s.appendChild(w);
      return s;
    };
    body.appendChild(card("Host", [
      ["OS", `${info.os}/${info.arch}`], ["Hostname", info.hostname],
      ["CPU", info.cpu.name], ["Cores", `${info.cpu.physicalCores}/${info.cpu.logicalCores}`],
      ["RAM total", formatBytes(info.ram.totalBytes)],
      ["RAM available", formatBytes(info.ram.available)],
    ]));
    body.appendChild(card("GPU", (info.gpus || []).map((g) => [g.vendor, `${g.name} (${formatBytes(g.vramBytes)} VRAM)`])));
    body.appendChild(card("Recommended", [
      ["num_thread", info.recommended.numThread],
      ["num_gpu", info.recommended.numGPU],
      ["num_ctx", info.recommended.numCtx],
      ["can_run_cpu", info.recommended.canRunCPU],
      ["can_run_gpu", info.recommended.canRunGPU],
    ]));
  }

  async function openInstallModal() {
    openModal("installModal");
    const body = $("installModalBody");
    body.innerHTML = "Loading…";
    const r = await api("/api/state");
    body.innerHTML = "";
    body.appendChild(el("div", "panel-section", null)).appendChild(el("h3", null, "Components"));
    const list = Object.entries(r.state?.components || {});
    list.forEach(([name, c]) => {
      const row = el("div", "info-row");
      row.appendChild(el("span", "label", name));
      const v = el("span", "value", `${c.status} ${c.version || ""}`.trim());
      if (c.status === "installed") v.style.color = "var(--success)";
      if (c.status === "missing") v.style.color = "var(--text-faint)";
      row.appendChild(v);
      body.appendChild(row);
    });
    body.appendChild(el("div", null, "")).appendChild(el("p", null,
      `Last run: ${new Date(r.state?.lastRunAt).toLocaleString()}`));
  }

  // ---------- Wire up ----------
  function wire() {
    $("newSessionBtn").onclick = newSession;
    $("sessionSearch").oninput = renderSessions;
    $("sendBtn").onclick = sendMessage;
    $("abortBtn").onclick = abortRun;
    $("toggleSidebarBtn").onclick = () => {
      const app = $("app");
      const sb = $("sidebar");
      if (sb.style.display === "none") sb.style.display = "";
      else sb.style.display = "none";
    };
    $("modelPickerBtn").onclick = openModelModal;
    $("presetPickerBtn").onclick = openPresetModal;
    $("contextBtn").onclick = () => openPanel("context");
    $("sysinfoBtn").onclick = openSysinfoModal;
    $("installBtn").onclick = openInstallModal;
    $("openSysInfoBtn").onclick = openSysinfoModal;
    $("startWithSampleBtn").onclick = async () => {
      if (!state.activeSession) await newSession();
      $("messageInput").value = "List the files in the current directory and explain what each one is for.";
      updateCharCount();
      sendMessage();
    };
    $("llamaStartBtn").onclick = llamaStart;
    $("llamaStopBtn").onclick = llamaStop;
    $("llamaLogsBtn").onclick = llamaShowLogs;
    document.querySelectorAll(".modal-close, [data-close]").forEach((b) => {
      b.onclick = () => closeModal(b.dataset.close);
    });
    document.querySelectorAll(".panel-tab").forEach((t) => {
      t.onclick = () => openPanel(t.dataset.tab);
    });
    $("panelCloseBtn").onclick = closePanel;

    const input = $("messageInput");
    input.oninput = () => { updateCharCount(); updateSendBtnState(); };
    input.onkeydown = (e) => {
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
      if (e.key === "Escape") abortRun();
    };
  }

  function updateCharCount() {
    $("charCount").textContent = $("messageInput").value.length;
  }
  function updateSendBtnState() {
    $("sendBtn").disabled = !$("messageInput").value.trim();
  }

  // ---------- Init ----------
  async function init() {
    wire();
    state.config = await api("/api/config");
    $("modelLabel").textContent = state.config.model;
    $("presetLabel").textContent = state.config.llm.preset;
    await loadSessions();
    if (state.sessions.length === 0) {
      const s = await api("/api/sessions", { method: "POST" });
      state.sessions.push(s);
    }
    state.activeSessionId = state.sessions[0].id;
    state.activeSession = state.sessions[0];
    renderSessions();
    renderActiveSession();
    updateSendBtnState();
  }

  document.addEventListener("DOMContentLoaded", init);
})();
