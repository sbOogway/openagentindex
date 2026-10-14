import Alpine from "alpinejs";
import "./main.css";
import { agentURL, get, searchURL, time } from "./api.js";

const pageSize = 20;
const popularCount = 12;
const maxTags = 10;

Alpine.data("search", () => ({
  q: "",
  status: "ok",
  binding: "",
  version: "",
  tags: [],
  offset: 0,
  results: [],
  total: 0,
  error: "",
  loading: true,
  tagInput: "",
  suggestions: [],
  popular: [],
  agentURL,
  time,

  init() {
    this.readURL();
    this.load();
    this.loadPopular();
    window.addEventListener("popstate", () => {
      this.readURL();
      this.load();
    });
  },

  readURL() {
    const p = new URLSearchParams(location.search);
    this.q = p.get("q") || "";
    this.status = p.get("status") || "ok";
    this.binding = p.get("binding") || "";
    this.version = p.get("version") || "";
    this.tags = p.getAll("tag");
    this.offset = Number(p.get("offset")) || 0;
  },

  params() {
    const p = new URLSearchParams({ q: this.q, status: this.status });
    if (this.binding) p.set("binding", this.binding);
    if (this.version) p.set("version", this.version);
    for (const t of this.tags) p.append("tag", t);
    return p;
  },

  async load() {
    this.loading = true;
    this.error = "";
    const p = this.params();
    p.set("limit", pageSize);
    p.set("offset", this.offset);
    try {
      const res = await get("/v1/search?" + p);
      this.results = res.results;
      this.total = res.total;
    } catch (err) {
      this.error = err.message;
      this.results = [];
      this.total = 0;
    } finally {
      this.loading = false;
    }
  },

  async loadPopular() {
    try {
      this.popular = (await get(`/v1/tags?limit=${popularCount}`)).tags || [];
    } catch {
      this.popular = [];
    }
  },

  async suggest() {
    const prefix = this.tagInput.trim();
    if (!prefix) {
      this.suggestions = [];
      return;
    }
    try {
      const res = await get("/v1/tags?limit=10&q=" + encodeURIComponent(prefix));
      if (prefix === this.tagInput.trim()) this.suggestions = res.tags || [];
    } catch {
      this.suggestions = [];
    }
  },

  go(offset) {
    this.offset = Math.max(0, offset);
    const p = this.params();
    if (this.offset) p.set("offset", this.offset);
    history.pushState(null, "", "?" + p);
    this.load();
  },

  addTag(tag) {
    const t = String(tag || this.tagInput).trim().toLowerCase();
    this.tagInput = "";
    this.suggestions = [];
    if (!t || this.tags.includes(t) || this.tags.length >= maxTags) return;
    this.tags.push(t);
    this.go(0);
  },

  removeTag(tag) {
    this.tags = this.tags.filter((t) => t !== tag);
    this.go(0);
  },

  clearFilters() {
    Object.assign(this, { q: "", status: "ok", binding: "", version: "", tags: [], tagInput: "", suggestions: [] });
    this.go(0);
  },

  get filtered() {
    return this.q || this.status !== "ok" || this.binding || this.version || this.tags.length > 0;
  },

  get last() {
    return Math.min(this.offset + this.results.length, this.total);
  },

  get summary() {
    if (this.loading) return "Searching…";
    return this.total === 0 ? "No agents found." : `${this.offset + 1}–${this.last} of ${this.total}`;
  },

  tagURL(tag) {
    return searchURL({ tag });
  },

  prev() {
    this.go(this.offset - pageSize);
  },

  next() {
    this.go(this.offset + pageSize);
  },
}));

Alpine.start();
