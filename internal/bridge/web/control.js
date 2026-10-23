"use strict";
(() => {
  let token = "";
  const message = document.querySelector("#message");
  const fragment = new URLSearchParams(location.hash.slice(1));
  const capability = fragment.get("launch");
  history.replaceState(null, "", location.pathname);
  const call = async (path, body) => {
    const response = await fetch(path, {method: body === undefined ? "GET" : "POST", headers: {"Content-Type": "application/json", "X-Companion-Control": token}, ...(body === undefined ? {} : {body: JSON.stringify(body)})});
    if (!response.ok) throw new Error(response.status === 401 || response.status === 403 ? "Open the companion shortcut again to manage this connection." : "That action could not finish. Try again.");
    return response.json();
  };
  const action = (text, fn) => {
    const button = document.createElement("button"); button.textContent = text;
    button.onclick = async () => {button.disabled = true;try {await fn();await refresh();} catch (e) {message.textContent = e.message;} finally {button.disabled = false;}};
    return button;
  };
  async function refresh() {
    const state = await call("/control/state");
    document.querySelector("#controls").hidden = false;
    const accounts = document.querySelector("#accounts"); accounts.replaceChildren();
    for (const account of state.accounts) accounts.append(action(`${account.steam_id} · ${account.location}${account.selected ? " · selected" : ""}`, () => call("/control/select", {account_id: account.id})));
    if (!state.accounts.length) accounts.textContent = "Steam was not found in its usual locations. Choose its installation folder below.";
    document.querySelector("#freshness").textContent = state.source?.snapshot ? `Last read: ${new Date(state.source.snapshot.read_at).toLocaleString()}${state.source.state === "stale" ? " · keeping the last successful read" : ""}` : "Choose an account to read its collections.";
    const pending = document.querySelector("#pending"); pending.replaceChildren();
    for (const item of state.pending) {const row = document.createElement("p");row.textContent = item.origin + " ";row.append(action("Allow this site to read my collections", () => call("/control/approve", {request_id: item.request_id})));pending.append(row);}
    if (!state.pending.length) pending.textContent = "No pending requests. In Sage, choose Connect companion to request access.";
    const paired = document.querySelector("#paired"); paired.replaceChildren();
    for (const origin of new Set(state.paired_origins)) {const row = document.createElement("p");row.textContent = origin + " ";row.append(action("Disconnect", () => call("/control/revoke", {origin})));paired.append(row);}
    if (!state.paired_origins.length) paired.textContent = "No sites connected.";
    message.textContent = state.paired_origins.length ? "Sage can read your collections. Return to its browser tab for a recommendation." : "Local controls are ready. In Sage, choose Connect companion to request access.";
    document.querySelector("#startup").checked = state.startup;
  }
  document.querySelector("#startup").onchange = async e => {try {await call("/control/startup", {enabled: e.target.checked});await refresh();}catch(e){message.textContent = e.message;await refresh();}};
  document.querySelector("#choose").onclick = async () => {try {await call("/control/choose-root", {});await refresh();}catch(e){message.textContent = e.message;}};
  document.querySelector("#root-save").onclick = async () => {try {await call("/control/root", {path: document.querySelector("#root").value});await refresh();}catch(e){message.textContent = e.message;}};
  document.querySelector("#stop").onclick = async () => {try {await call("/control/stop", {});token = "";document.querySelector("#controls").hidden = true;message.textContent = "Companion stopped. Saved browser choices remain available.";}catch(e){message.textContent = e.message;}};
  if (capability) call("/control/session", {capability}).then(async value => {token = value.control_token;message.textContent = "Connected to your local companion controls.";await refresh();setInterval(() => {if (token && !document.hidden) refresh().catch(e => {message.textContent = e.message;});}, 3000);}).catch(e => {message.textContent = e.message;});
})();
