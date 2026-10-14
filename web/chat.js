// The browser sends A2A messages directly to the agent. The website and the API never relay them.

const timeoutMs = 60000;

// endpoint returns the JSON-RPC URL of the card and whether it uses v0.3, or null.
function endpoint(card) {
  for (const i of card.supportedInterfaces || []) {
    if (i.protocolBinding === "JSONRPC" && i.url?.startsWith("https://")) {
      return { url: i.url, v03: String(i.protocolVersion || "").startsWith("0.") };
    }
  }
  if (card.url?.startsWith("https://") && (card.preferredTransport || "JSONRPC") === "JSONRPC") {
    return { url: card.url, v03: true };
  }
  return null;
}

function request(ep, text, contextId) {
  const id = crypto.randomUUID();
  const ctx = contextId ? { contextId } : {};
  if (ep.v03) {
    const message = { kind: "message", messageId: id, role: "user", parts: [{ kind: "text", text }], ...ctx };
    return { headers: {}, body: { jsonrpc: "2.0", id, method: "message/send", params: { message } } };
  }
  const message = { messageId: id, role: "ROLE_USER", parts: [{ text }], ...ctx };
  return { headers: { "A2A-Version": "1.0" }, body: { jsonrpc: "2.0", id, method: "SendMessage", params: { message } } };
}

function partText(p) {
  if (typeof p.text === "string") return p.text;
  if (p.data !== undefined) return "\n" + JSON.stringify(p.data, null, 2) + "\n";
  const file = p.file || p;
  const name = file.name || file.filename;
  return name ? `\n[file ${name}]\n` : "\n[file]\n";
}

// Text parts join without a separator: agents often send one text in chunks.
function partsText(parts) {
  return (parts || []).map(partText).join("").trim();
}

// reply converts a v1.0 or v0.3 SendMessage result to text.
function reply(result) {
  const message = result.message || (result.kind === "message" ? result : null);
  if (message) return { text: partsText(message.parts), contextId: message.contextId };
  const task = result.task || result;
  const state = String(task.status?.state || "unknown").replace(/^TASK_STATE_/, "").toLowerCase().replaceAll("_", " ");
  const lines = [`[task ${state}]`, partsText(task.status?.message?.parts)];
  for (const a of task.artifacts || []) lines.push(partsText(a.parts));
  return { text: lines.filter(Boolean).join("\n"), contextId: task.contextId };
}

async function send(ep, text, contextId) {
  const req = request(ep, text, contextId);
  let resp;
  try {
    resp = await fetch(ep.url, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...req.headers },
      body: JSON.stringify(req.body),
      signal: AbortSignal.timeout(timeoutMs),
    });
  } catch (err) {
    if (err.name === "TimeoutError") throw new Error("The agent did not reply in 60 s.");
    throw new Error("The agent does not respond, or it does not accept requests from this website (CORS).");
  }
  const body = await resp.json().catch(() => null);
  if (!body) throw new Error(`The agent gave status ${resp.status} without a JSON-RPC response.`);
  if (body.error) throw new Error(`The agent gave error ${body.error.code}: ${body.error.message}`);
  return reply(body.result || {});
}

export function mountChat(card, section, el) {
  const ep = card && endpoint(card);
  if (!ep) {
    section.querySelector("#chat-note").textContent = card
      ? "This agent has no HTTPS JSON-RPC endpoint. The chat is not available."
      : "This host has no valid agent card. The chat is not available.";
    section.querySelector("#chat-form").hidden = true;
    return;
  }
  const log = section.querySelector("#chat-log");
  const form = section.querySelector("#chat-form");
  let contextId;
  const add = (who, text) => {
    log.append(el("div", text, "msg " + who));
    log.scrollTop = log.scrollHeight;
  };
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const text = form.text.value.trim();
    if (!text) return;
    form.text.value = "";
    form.send.disabled = true;
    add("user", text);
    try {
      const r = await send(ep, text, contextId);
      contextId = r.contextId || contextId;
      add("agent", r.text || "(empty reply)");
    } catch (err) {
      add("error", err.message);
    } finally {
      form.send.disabled = false;
      form.text.focus();
    }
  });
}
