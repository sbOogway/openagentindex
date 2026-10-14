const base = import.meta.env.VITE_API_URL;

// get calls the API and returns the JSON body. It throws an Error with a message for the user.
export async function get(path) {
  let resp;
  try {
    resp = await fetch(base + path);
  } catch {
    throw new Error("Search is not available. The API does not respond.");
  }
  const body = await resp.json().catch(() => ({ error: `The API gave status ${resp.status}.` }));
  if (!resp.ok) throw new Error(body.error);
  return body;
}

export function time(sec) {
  return new Date(sec * 1000).toLocaleString();
}

export function agentURL(host) {
  return "agent.html?host=" + encodeURIComponent(host);
}

export function searchURL(params) {
  return "index.html?" + new URLSearchParams(params);
}
