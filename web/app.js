const output = document.getElementById("output");
const statsEl = document.getElementById("stats");
const catalogEl = document.getElementById("catalog");
const botsEl = document.getElementById("bots");
const recordingsEl = document.getElementById("recordings");
const calendarEl = document.getElementById("calendar");
const webhooksEl = document.getElementById("webhooks");
const docsLink = document.getElementById("docsLink");
const selectedBotPanel = document.getElementById("selectedBotPanel");
const selectionHint = document.getElementById("selectionHint");
const quickSteps = document.getElementById("quickSteps");

let currentState = null;
let selectedBotID = null;

const mediaPath = {
  transcript: "transcripts",
  video_mixed: "video_mixed",
  audio_mixed: "audio_mixed",
  meeting_metadata: "meeting_metadata",
  participant_events: "participant_events",
};

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  const text = await response.text();
  let data = text;
  try {
    data = text ? JSON.parse(text) : {};
  } catch (_) {}

  output.textContent = `${response.status} ${response.statusText}\n${path}\n\n${typeof data === "string" ? data : JSON.stringify(data, null, 2)}`;
  if (!response.ok) {
    throw new Error(typeof data === "string" ? data : JSON.stringify(data));
  }
  return data;
}

function fmt(value) {
  if (!value) return "n/a";
  return new Date(value).toLocaleString("pt-BR");
}

function toDateTimeLocal(date) {
  const pad = (value) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function chip(text, kind = "") {
  return `<span class="chip ${kind}">${text}</span>`;
}

function parseCSV(value) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseJSONField(value, fallback = {}) {
  if (!value.trim()) return fallback;
  return JSON.parse(value);
}

function selectedBot() {
  return currentState?.bots?.find((bot) => bot.id === selectedBotID) || null;
}

function selectedRecordings() {
  if (!selectedBotID) return [];
  return (currentState?.recordings || []).filter((recording) => recording.bot_id === selectedBotID);
}

function renderStats(summary) {
  statsEl.innerHTML = `
    <article class="stat"><span class="tag">Bots</span><strong>${summary.bots}</strong><span>bots carregados para teste</span></article>
    <article class="stat"><span class="tag">Recordings</span><strong>${summary.recordings}</strong><span>gravacoes simuladas</span></article>
    <article class="stat"><span class="tag">Completed</span><strong>${summary.completed_recordings}</strong><span>com media shortcuts gerados</span></article>
    <article class="stat"><span class="tag">Calendar</span><strong>${summary.calendar_meetings}</strong><span>itens de agenda para auto-join</span></article>
  `;
}

function renderQuickSteps() {
  const steps = [
    "1. Rode Bootstrap demo para carregar bots, recordings e agenda inicial.",
    "2. Selecione um bot na coluna da esquerda para habilitar as ações dele.",
    "3. Use Start, Pause, Resume, Stop e Leave para validar o ciclo completo de gravação.",
    "4. Abra os media shortcuts do recording para inspecionar transcript, vídeo, áudio, metadata e participant events.",
    "5. Crie um bot novo ou um calendar meeting pelo formulário para testar criação manual.",
    "6. Se quiser revisar o contrato, use o catálogo prático com endpoint oficial e espelho local."
  ];

  quickSteps.innerHTML = steps.map((step) => `<div class="step">${step}</div>`).join("");
}

function renderCatalog(endpoints) {
  catalogEl.innerHTML = `<div class="stack compact">${
    endpoints.map((endpoint) => `
      <div class="item endpoint">
        <div class="meta">
          ${chip(endpoint.category)}
          ${chip(endpoint.resource, "warn")}
        </div>
        <h3>${endpoint.name}</h3>
        <p>${endpoint.description}</p>
        <div class="small">Official: ${endpoint.official}</div>
        <div class="small">Local: ${endpoint.local}</div>
        <div class="actions">
          <button class="ghost small-btn" data-local-endpoint="${endpoint.local}">Testar local</button>
          <a class="action-link ghost small-btn" href="${endpoint.docs_url}" target="_blank" rel="noreferrer">Doc</a>
        </div>
      </div>
    `).join("")
  }</div>`;

  catalogEl.querySelectorAll("[data-local-endpoint]").forEach((button) => {
    button.addEventListener("click", async () => {
      const local = button.dataset.localEndpoint;
      if (!local.startsWith("GET ")) {
        output.textContent = `Use os formulários e ações da tela para testar ${local}.`;
        return;
      }
      const path = local.replace("GET ", "");
      await api(path);
    });
  });
}

function renderBots(bots) {
  botsEl.innerHTML = `<div class="stack">${
    bots.map((bot) => `
      <button class="item bot-card ${bot.id === selectedBotID ? "selected" : ""}" data-select-bot="${bot.id}">
        <div class="meta">
          ${chip(bot.status)}
          ${chip(bot.recording_mode)}
        </div>
        <h3>${bot.bot_name}</h3>
        <p>${bot.meeting_url || "Sem meeting_url"}</p>
        <div class="small">ID: ${bot.id}</div>
        <div class="list-inline">
          ${(bot.real_time_features || []).map((feature) => chip(feature, "soft")).join("")}
        </div>
      </button>
    `).join("")
  }</div>`;

  botsEl.querySelectorAll("[data-select-bot]").forEach((button) => {
    button.addEventListener("click", () => {
      selectedBotID = button.dataset.selectBot;
      renderBots(currentState.bots || []);
      renderSelectedBotPanel();
    });
  });
}

function renderSelectedBotPanel() {
  const bot = selectedBot();
  if (!bot) {
    selectionHint.textContent = "Nenhum bot selecionado.";
    selectedBotPanel.className = "empty-state";
    selectedBotPanel.textContent = "Selecione um bot na coluna da esquerda para habilitar ações.";
    return;
  }

  selectionHint.textContent = `${bot.bot_name} (${bot.id})`;
  const recordings = selectedRecordings();
  const outputMediaValue = bot.output_media ? JSON.stringify(bot.output_media, null, 2) : "";

  selectedBotPanel.className = "selected-panel";
  selectedBotPanel.innerHTML = `
    <div class="selected-summary">
      <div>
        <div class="meta">
          ${chip(bot.status)}
          ${chip(bot.transcription, "warn")}
          ${chip(bot.recording_mode)}
        </div>
        <h3>${bot.bot_name}</h3>
        <p>${bot.meeting_url || "Sem meeting_url"}</p>
        <div class="small">Join at: ${fmt(bot.join_at)}</div>
        <div class="small">Created: ${fmt(bot.created_at)}</div>
      </div>
      <div class="actions">
        <button data-bot-action="start">Start recording</button>
        <button data-bot-action="pause" class="ghost">Pause</button>
        <button data-bot-action="resume" class="ghost">Resume</button>
        <button data-bot-action="stop" class="ghost">Stop</button>
        <button data-bot-action="leave" class="ghost">Leave call</button>
        <button data-bot-action="inspect" class="ghost">Buscar bot</button>
        <button data-bot-action="delete" class="ghost danger">Excluir bot</button>
      </div>
    </div>

    <div class="workspace-grid">
      <form id="updateBotForm" class="form-grid">
        <h4>Editar bot</h4>
        <label>
          <span>Bot name</span>
          <input name="bot_name" value="${bot.bot_name}">
        </label>
        <label>
          <span>Meeting URL</span>
          <input name="meeting_url" value="${bot.meeting_url || ""}">
        </label>
        <label>
          <span>Status</span>
          <input name="status" value="${bot.status}">
        </label>
        <label>
          <span>Join at</span>
          <input type="datetime-local" name="join_at" value="${bot.join_at ? toDateTimeLocal(new Date(bot.join_at)) : ""}">
        </label>
        <label class="full">
          <span>Metadata JSON</span>
          <textarea name="metadata_json" rows="4">${JSON.stringify(bot.metadata || {}, null, 2)}</textarea>
        </label>
        <div class="form-actions full">
          <button type="submit">Salvar bot</button>
        </div>
      </form>

      <form id="outputMediaForm" class="form-grid">
        <h4>Output media</h4>
        <label class="full">
          <span>JSON de output_media</span>
          <textarea name="output_media_json" rows="8" placeholder='{"camera":{"kind":"webpage","url":"https://example.com/avatar"}}'>${outputMediaValue}</textarea>
        </label>
        <div class="form-actions full">
          <button type="submit">Salvar output media</button>
          <button type="button" id="clearOutputMediaBtn" class="ghost">Remover output media</button>
        </div>
      </form>
    </div>

    <div class="item embedded">
      <h4>Recordings do bot</h4>
      <div class="stack compact">
        ${recordings.length ? recordings.map((recording) => `
          <div class="item inline-card">
            <div>
              <div class="meta">
                ${chip(recording.status)}
                ${chip(`${recording.duration_sec}s`, "warn")}
              </div>
              <div class="small">${recording.id}</div>
            </div>
            <div class="actions">
              <button class="ghost small-btn" data-open-recording="${recording.id}">Ver recording</button>
              ${Object.values(recording.media_shortcuts || {}).filter((item) => item.id).map((item) => `
                <button class="ghost small-btn" data-open-media="${item.kind}:${item.id}">${item.kind}</button>
              `).join("")}
            </div>
          </div>
        `).join("") : `<div class="empty-inline">Nenhuma recording ligada ao bot selecionado.</div>`}
      </div>
    </div>
  `;

  selectedBotPanel.querySelectorAll("[data-bot-action]").forEach((button) => {
    button.addEventListener("click", async () => {
      const action = button.dataset.botAction;
      if (action === "inspect") {
        await api(`/api/review/recall/bots/${bot.id}`);
        return;
      }
      if (action === "delete") {
        await api(`/api/review/recall/bots/${bot.id}`, { method: "DELETE" });
        selectedBotID = null;
        await loadState();
        return;
      }
      const map = {
        start: "start_recording",
        pause: "pause_recording",
        resume: "resume_recording",
        stop: "stop_recording",
        leave: "leave",
      };
      await api(`/api/review/recall/bots/${bot.id}/${map[action]}`, { method: "POST" });
      await loadState();
    });
  });

  selectedBotPanel.querySelector("#updateBotForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const joinAt = form.get("join_at");
    await api(`/api/review/recall/bots/${bot.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        bot_name: form.get("bot_name"),
        meeting_url: form.get("meeting_url"),
        status: form.get("status"),
        join_at: joinAt ? new Date(joinAt).toISOString() : "",
        metadata: parseJSONField(form.get("metadata_json"), {}),
      }),
    });
    await loadState();
  });

  selectedBotPanel.querySelector("#outputMediaForm").addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await api(`/api/review/recall/bots/${bot.id}/output_media`, {
      method: "POST",
      body: JSON.stringify(parseJSONField(form.get("output_media_json"), {})),
    });
    await loadState();
  });

  selectedBotPanel.querySelector("#clearOutputMediaBtn").addEventListener("click", async () => {
    await api(`/api/review/recall/bots/${bot.id}/output_media`, { method: "DELETE" });
    await loadState();
  });

  selectedBotPanel.querySelectorAll("[data-open-recording]").forEach((button) => {
    button.addEventListener("click", async () => {
      await api(`/api/review/recall/recordings/${button.dataset.openRecording}`);
    });
  });

  selectedBotPanel.querySelectorAll("[data-open-media]").forEach((button) => {
    button.addEventListener("click", async () => {
      const [kind, id] = button.dataset.openMedia.split(":");
      await api(`/api/review/recall/${mediaPath[kind]}/${id}`);
    });
  });
}

function renderRecordings(recordings) {
  recordingsEl.innerHTML = `<div class="stack compact">${
    recordings.map((recording) => `
      <div class="item">
        <div class="meta">
          ${chip(recording.status)}
          ${chip(`${recording.duration_sec}s`, "warn")}
        </div>
        <h3>${recording.id}</h3>
        <div class="small">Bot: ${recording.bot_id}</div>
        <div class="actions">
          <button class="ghost small-btn" data-recording-id="${recording.id}">Abrir</button>
          ${Object.values(recording.media_shortcuts || {}).filter((item) => item.id).slice(0, 2).map((item) => `
            <button class="ghost small-btn" data-recording-media="${item.kind}:${item.id}">${item.kind}</button>
          `).join("")}
        </div>
      </div>
    `).join("")
  }</div>`;

  recordingsEl.querySelectorAll("[data-recording-id]").forEach((button) => {
    button.addEventListener("click", async () => {
      await api(`/api/review/recall/recordings/${button.dataset.recordingId}`);
    });
  });

  recordingsEl.querySelectorAll("[data-recording-media]").forEach((button) => {
    button.addEventListener("click", async () => {
      const [kind, id] = button.dataset.recordingMedia.split(":");
      await api(`/api/review/recall/${mediaPath[kind]}/${id}`);
    });
  });
}

function renderCalendar(meetings) {
  calendarEl.innerHTML = `<div class="stack compact">${
    meetings.map((meeting) => `
      <div class="item">
        <div class="meta">
          ${chip(meeting.platform)}
          ${chip(meeting.recording_state, "warn")}
        </div>
        <h3>${meeting.id}</h3>
        <p>${meeting.meeting_url}</p>
        <div class="small">Starts at: ${fmt(meeting.starts_at)}</div>
        <div class="actions">
          <button class="ghost small-btn" data-calendar-id="${meeting.id}">Abrir</button>
        </div>
      </div>
    `).join("")
  }</div>`;

  calendarEl.querySelectorAll("[data-calendar-id]").forEach((button) => {
    button.addEventListener("click", async () => {
      await api(`/api/review/recall/calendar/meetings/${button.dataset.calendarId}`);
    });
  });
}

function renderWebhooks(samples) {
  webhooksEl.innerHTML = `<div class="stack compact">${
    samples.map((sample, index) => `
      <div class="item">
        <div class="meta">${chip(sample.event)}</div>
        <h3>${sample.description}</h3>
        <div class="actions">
          <button class="ghost small-btn" data-webhook-index="${index}">Ver payload</button>
        </div>
      </div>
    `).join("")
  }</div>`;

  webhooksEl.querySelectorAll("[data-webhook-index]").forEach((button) => {
    button.addEventListener("click", () => {
      const sample = currentState.webhook_samples[Number(button.dataset.webhookIndex)];
      output.textContent = JSON.stringify(sample, null, 2);
    });
  });
}

async function loadCatalog() {
  const data = await api("/api/review/recall/catalog");
  renderCatalog(data.endpoints || []);
}

async function loadState() {
  currentState = await api("/api/review/recall/state");
  docsLink.href = currentState.official_docs_entry;
  renderStats(currentState.summary);
  renderQuickSteps();
  renderBots(currentState.bots || []);
  renderRecordings(currentState.recordings || []);
  renderCalendar(currentState.calendar_meetings || []);
  renderWebhooks(currentState.webhook_samples || []);

  if (!selectedBotID && currentState.bots?.length) {
    selectedBotID = currentState.bots[0].id;
    renderBots(currentState.bots || []);
  }
  if (selectedBotID && !currentState.bots.some((bot) => bot.id === selectedBotID)) {
    selectedBotID = currentState.bots[0]?.id || null;
    renderBots(currentState.bots || []);
  }
  renderSelectedBotPanel();
}

document.getElementById("bootstrapBtn").addEventListener("click", async () => {
  await api("/api/review/recall/bootstrap", { method: "POST" });
  await loadState();
});

document.getElementById("refreshBtn").addEventListener("click", async () => {
  await loadState();
});

document.getElementById("resetBtn").addEventListener("click", async () => {
  await api("/api/review/recall/reset", { method: "POST" });
  await loadState();
});

document.getElementById("createBotForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const joinAt = form.get("join_at");
  const bot = await api("/api/review/recall/bots", {
    method: "POST",
    body: JSON.stringify({
      bot_name: form.get("bot_name"),
      meeting_url: form.get("meeting_url"),
      recording_mode: form.get("recording_mode"),
      transcription: form.get("transcription"),
      join_at: joinAt ? new Date(joinAt).toISOString() : "",
      metadata: { meeting_id: form.get("meeting_id") },
      real_time_features: parseCSV(String(form.get("real_time_features"))),
      output_media: parseJSONField(String(form.get("output_media_json")), null),
    }),
  });
  selectedBotID = bot.id;
  await loadState();
});

document.getElementById("calendarForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const startsAt = form.get("starts_at");
  await api("/api/review/recall/calendar/meetings", {
    method: "POST",
    body: JSON.stringify({
      platform: form.get("platform"),
      meeting_url: form.get("meeting_url"),
      starts_at: startsAt ? new Date(startsAt).toISOString() : "",
      calendar_user: form.get("calendar_user"),
      metadata: { source: form.get("source") },
    }),
  });
  await loadState();
});

document.getElementById("refreshCalendarBtn").addEventListener("click", async () => {
  await api("/api/review/recall/calendar/meetings/refresh", { method: "POST" });
  await loadState();
});

const calendarDateInput = document.querySelector('#calendarForm [name="starts_at"]');
calendarDateInput.value = toDateTimeLocal(new Date(Date.now() + 90 * 60 * 1000));

Promise.all([loadCatalog(), loadState()]).catch((error) => {
  output.textContent = error.message;
});
