// Agent cards are untrusted input: set text with textContent only, never as HTML.

import { mountChat } from "./chat.js";

const api = document.querySelector('meta[name="oai-api"]').content;
const pageSize = 20;
const $ = (id) => document.getElementById(id);

function el(tag, text, className) {
  const e = document.createElement(tag);
  if (text !== undefined) e.textContent = text;
  if (className) e.className = className;
  return e;
}

async function get(path) {
  let resp;
  try {
    resp = await fetch(api + path);
  } catch {
    throw new Error("Search is not available. The API does not respond.");
  }
  const body = await resp.json().catch(() => ({ error: `The API gave status ${resp.status}.` }));
  if (!resp.ok) throw new Error(body.error);
  return body;
}

function time(sec) {
  return new Date(sec * 1000).toLocaleString();
}

function agentLink(host) {
  const a = el("a", host);
  a.href = "agent.html?host=" + encodeURIComponent(host);
  return a;
}

function resultItem(r) {
  const li = el("li");
  const title = el("h3");
  title.append(agentLink(r.host));
  if (r.name) title.prepend(r.name + " ");
  li.append(title);
  li.append(el("p", r.description || r.error || "", "description"));
  const meta = el("p", "", "meta");
  const version = r.protocol_version ? ` v${r.protocol_version} ·` : "";
  meta.append(el("span", r.status, "status " + r.status), `${version} checked ${time(r.last_checked)}`);
  li.append(meta);
  return li;
}

async function searchPage() {
  const params = new URLSearchParams(location.search);
  const form = $("search");
  form.q.value = params.get("q") || "";
  form.status.value = params.get("status") || "ok";
  const offset = Number(params.get("offset")) || 0;

  const go = (newOffset) => {
    const p = new URLSearchParams({ q: form.q.value, status: form.status.value, offset: newOffset });
    location.search = p.toString();
  };
  form.addEventListener("submit", (e) => { e.preventDefault(); go(0); });
  $("prev").addEventListener("click", () => go(Math.max(0, offset - pageSize)));
  $("next").addEventListener("click", () => go(offset + pageSize));

  const query = new URLSearchParams({ q: form.q.value, status: form.status.value, limit: pageSize, offset });
  try {
    const res = await get("/v1/search?" + query);
    const last = Math.min(offset + res.results.length, res.total);
    $("message").textContent = res.total === 0 ? "No agents found." : `${offset + 1}–${last} of ${res.total}`;
    $("results").replaceChildren(...res.results.map(resultItem));
    $("prev").disabled = offset === 0;
    $("next").disabled = last >= res.total;
  } catch (err) {
    $("message").textContent = err.message;
    $("pages").hidden = true;
  }
}

function skillItem(s) {
  const li = el("li");
  li.append(el("strong", s.name || s.id || ""), " ", el("span", s.description || ""));
  if (Array.isArray(s.tags) && s.tags.length) li.append(el("p", s.tags.join(", "), "meta"));
  return li;
}

function observationRow(o) {
  const tr = el("tr");
  tr.append(
    el("td", time(o.at)), el("td", o.kind), el("td", o.ok ? "ok" : "failed", "status " + (o.ok ? "ok" : "failed")),
    el("td", String(o.http_status || "")), el("td", o.latency_ms + " ms"), el("td", o.error || ""),
  );
  return tr;
}

async function agentPage() {
  const host = new URLSearchParams(location.search).get("host") || "";
  try {
    const a = await get("/v1/agents/" + encodeURIComponent(host));
    document.title = (a.name || a.host) + " · Open Agent Index";
    $("name").textContent = a.name || a.host;
    const cardURL = new URL("https://" + a.host + "/.well-known/agent-card.json");
    $("card-url").href = cardURL.href;
    $("card-url").textContent = a.host;
    $("status").textContent = a.status;
    $("status").classList.add(a.status);
    $("version").textContent = a.protocol_version ? "v" + a.protocol_version : "";
    $("description").textContent = a.description;
    const skills = (a.card && Array.isArray(a.card.skills)) ? a.card.skills : [];
    $("skills").replaceChildren(...(skills.length ? skills.map(skillItem) : [el("li", "No skills.")]));
    $("observations").replaceChildren(...a.observations.map(observationRow));
    $("card").textContent = a.card ? JSON.stringify(a.card, null, 2) : "No valid card.";
    mountChat(a.card, $("chat"), el);
    $("agent").hidden = false;
  } catch (err) {
    $("message").textContent = err.message;
  }
}

if (document.body.dataset.page === "search") searchPage();
else agentPage();
