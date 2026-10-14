import Alpine from "alpinejs";
import "./main.css";
import { endpoint, send } from "./a2a.js";
import { get, searchURL, time } from "./api.js";

Alpine.data("agent", () => ({
  agent: null,
  error: "",
  endpoint: null,
  contextId: undefined,
  messages: [],
  text: "",
  sending: false,
  time,
  searchURL,

  async init() {
    const host = new URLSearchParams(location.search).get("host") || "";
    try {
      this.agent = await get("/v1/agents/" + encodeURIComponent(host));
      this.endpoint = this.agent.card && endpoint(this.agent.card);
      document.title = (this.agent.name || this.agent.host) + " · Open Agent Index";
    } catch (err) {
      this.error = err.message;
    }
  },

  get cardURL() {
    return new URL("https://" + this.agent.host + "/.well-known/agent-card.json").href;
  },

  get tags() {
    return this.agent.tags || [];
  },

  get lastCheck() {
    const o = this.agent.observations[0];
    if (!o) return "";
    return `${o.ok ? "ok" : "failed"} · HTTP ${o.http_status || "-"} · ${o.latency_ms} ms · ${time(o.at)}`;
  },

  get chatUnavailable() {
    if (!this.agent.card) return "This host has no valid agent card. The chat is not available.";
    if (!this.endpoint) return "This agent has no HTTPS JSON-RPC endpoint. The chat is not available.";
    return "";
  },

  async sendMessage() {
    const text = this.text.trim();
    if (!text || this.sending) return;
    this.text = "";
    this.sending = true;
    this.messages.push({ who: "user", text });
    try {
      const r = await send(this.endpoint, text, this.contextId);
      this.contextId = r.contextId || this.contextId;
      this.messages.push({ who: "agent", text: r.text || "(empty reply)" });
    } catch (err) {
      this.messages.push({ who: "error", text: err.message });
    } finally {
      this.sending = false;
      this.$nextTick(() => {
        this.$refs.log.scrollTop = this.$refs.log.scrollHeight;
        this.$refs.input.focus();
      });
    }
  },
}));

Alpine.start();
