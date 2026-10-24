"use strict";
(() => {
  let token = "";
  let refreshGeneration = 0;
  const $ = selector => document.querySelector(selector);
  const message = $("#message");
  const fragment = new URLSearchParams(location.hash.slice(1));
  const capability = fragment.get("launch");
  const focusID = fragment.get("request_id");
  history.replaceState(null, "", location.pathname);
  $("#title").textContent = focusID ? "Connect your collections" : "Companion settings";
  $("#settings").open = !focusID;
  $("#pending-section").hidden = !!focusID;
  if (focusID) $("#approval").insertBefore($("#library"), $("#approval-actions"));
  const call = async (path, body) => {
    const response = await fetch(path, {method: body === undefined ? "GET" : "POST", headers: {"Content-Type": "application/json", "X-Companion-Control": token}, ...(body === undefined ? {} : {body: JSON.stringify(body)})});
    if (!response.ok) throw new Error(response.status === 401 || response.status === 403 ? "Open Companion Settings from the application menu to manage this connection." : response.status === 410 ? "This request has ended. Return to your original Sage tab to connect again." : "That action could not finish. Try again, or check Companion settings.");
    return response.json();
  };
  const bindAction = (button, fn) => {
    button.onclick = async () => {
      button.disabled = true;
      try {await fn(); message.textContent = "";} catch (e) {message.textContent = e.message;}
      finally {button.disabled = false; await refresh().catch(e => {message.textContent = e.message;});}
    };
  };
  const action = (text, fn) => {
    const button = document.createElement("button"); button.textContent = text;
    bindAction(button, fn);
    return button;
  };
  // Keep keyed rows (and their buttons) in place while polling, including the
  // active keyboard target. Only remove rows whose identity has disappeared.
  const reconcile = (container, items, key, create, update, empty) => {
    const existing = new Map(Array.from(container.children).map(el => [el.dataset.key, el]));
    for (const item of items) {
      const id = key(item);
      let row = existing.get(id);
      if (!row) {row = create(item); row.dataset.key = id; container.append(row);}
      update(row, item);
      existing.delete(id);
    }
    for (const row of existing.values()) row.remove();
    let note = container.querySelector("[data-empty]");
    if (!items.length && !note) {note = document.createElement("p");note.dataset.empty = "true";note.textContent = empty;container.append(note);}
    if (items.length && note) note.remove();
  };
  function renderApproval(state) {
    if (!focusID) return;
    $("#approval").hidden = false;
    const focused = state.focused_request?.request_id === focusID ? state.focused_request : null;
    const lifecycle = focused?.state || "expired";
    if (focused) $("#request-origin").textContent = focused.origin;
    const explanations = {
      pending: "Approve only if you chose Connect in this site's Sage tab.",
      approved: "Permission approved. Return to the original Sage tab to finish connecting and read your collections.",
      claimed: "Connected. Return to the original Sage tab; it checks that your collections can be read. You can close this page.",
      declined: "Request declined. Nothing else needs to change. Return to the original Sage tab if you want to connect again.",
      cancelled: "Request cancelled. Return to the original Sage tab if you want to connect again.",
      expired: "Request expired. Return to the original Sage tab and choose Connect again. Other requests have not been approved."
    };
    $("#request-state").textContent = explanations[lifecycle] || explanations.expired;
    $("#approval-actions").hidden = lifecycle !== "pending";
    $("#allow").disabled = !state.selected_steam_id;
    $("#approval-title").textContent = lifecycle === "pending" ? "Allow access to your collections?" : "Connection request";
  }
  async function refresh() {
    const generation = ++refreshGeneration;
    const state = await call("/control/state" + (focusID ? "?request_id=" + encodeURIComponent(focusID) : ""));
    if (generation !== refreshGeneration) return;
    $("#controls").hidden = false;
    reconcile($("#accounts"), state.accounts, a => a.id,
      account => action("", () => call("/control/select", {account_id: account.id})),
      (button, account) => {button.textContent = `${account.steam_id} · ${account.location}${account.selected ? " · selected" : ""}`;button.setAttribute("aria-pressed", String(account.selected));},
      "Steam was not found in its usual locations. Use Companion settings below to choose its installation folder.");
    const source = state.source;
    $("#freshness").textContent = !state.selected_steam_id ? "Choose an account to read its collections." : source?.snapshot ?
      `Last successful read: ${new Date(source.snapshot.read_at).toLocaleString()}. ${source.state === "stale" ? "Companion could not read the latest collections. Keeping the last successful read; check the Steam folder in Companion settings." : source.snapshot.collections.length === 0 ? "No supported static collections were found. You can still use Sage recommendations." : "Collections are available to connected sites."}` :
      "Companion could not read collections for the selected account. Check the Steam folder in Companion settings below and try again. Your saved choices are kept.";
    if (!focusID) reconcile($("#pending"), state.pending, item => item.request_id,
      item => {const row = document.createElement("p");const site = document.createElement("span");site.textContent = item.origin + " ";row.append(site, action("Allow collections", () => call("/control/approve", {request_id: item.request_id})), action("Not now", () => call("/control/decline", {request_id: item.request_id})));return row;},
      () => {}, "No pending requests. In your Sage tab, choose Connect to request access.");
    reconcile($("#paired"), [...new Set(state.paired_origins)], origin => origin,
      origin => {const row = document.createElement("p");const site = document.createElement("span");site.textContent = origin + " ";row.append(site, action("Disconnect", () => call("/control/revoke", {origin})));return row;},
      () => {}, "No sites connected.");
    $("#startup").checked = state.startup;
    renderApproval(state);
  }
  bindAction($("#allow"), () => call("/control/approve", {request_id: focusID}));
  bindAction($("#decline"), () => call("/control/decline", {request_id: focusID}));
  $("#startup").onchange = async e => {try {await call("/control/startup", {enabled: e.target.checked});}catch(e){message.textContent = e.message;}finally{await refresh().catch(e => {message.textContent = e.message;});}};
  bindAction($("#choose"), () => call("/control/choose-root", {}));
  bindAction($("#root-save"), () => call("/control/root", {path: $("#root").value}));
  $("#stop").onclick = async () => {try {await call("/control/stop", {});token = "";refreshGeneration++;$("#controls").hidden = true;message.textContent = "Companion stopped. Saved browser choices remain available.";}catch(e){message.textContent = e.message;}};
  if (capability) call("/control/session", {capability}).then(async value => {token = value.control_token;message.textContent = "";await refresh();setInterval(() => {if (token && !document.hidden) return refresh().catch(e => {message.textContent = e.message;});}, 3000);}).catch(e => {message.textContent = e.message;});
})();
