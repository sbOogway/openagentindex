import Alpine from "alpinejs";
import "./main.css";
import { agentURL, get, time } from "./api.js";

const pageSize = 20;

Alpine.data("search", () => ({
  q: "",
  status: "ok",
  offset: 0,
  results: [],
  total: 0,
  error: "",
  loading: true,
  agentURL,
  time,

  init() {
    this.readURL();
    this.load();
    window.addEventListener("popstate", () => {
      this.readURL();
      this.load();
    });
  },

  readURL() {
    const p = new URLSearchParams(location.search);
    this.q = p.get("q") || "";
    this.status = p.get("status") || "ok";
    this.offset = Number(p.get("offset")) || 0;
  },

  async load() {
    this.loading = true;
    this.error = "";
    const query = new URLSearchParams({ q: this.q, status: this.status, limit: pageSize, offset: this.offset });
    try {
      const res = await get("/v1/search?" + query);
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

  go(offset) {
    this.offset = Math.max(0, offset);
    const p = new URLSearchParams({ q: this.q, status: this.status, offset: this.offset });
    history.pushState(null, "", "?" + p);
    this.load();
  },

  get last() {
    return Math.min(this.offset + this.results.length, this.total);
  },

  get summary() {
    if (this.loading) return "Searching…";
    return this.total === 0 ? "No agents found." : `${this.offset + 1}–${this.last} of ${this.total}`;
  },

  prev() {
    this.go(this.offset - pageSize);
  },

  next() {
    this.go(this.offset + pageSize);
  },
}));

Alpine.start();
