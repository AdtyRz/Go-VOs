(() => {
  "use strict";

  const meta = document.querySelector('meta[name="csrf-token"]');
  const csrf = (meta && meta.content) || document.body.dataset.csrf || "";
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  const numberFormat = new Intl.NumberFormat("id-ID");
  const dateTimeFormat = new Intl.DateTimeFormat("id-ID", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

  function qs(selector, root = document) {
    return root.querySelector(selector);
  }

  function qsa(selector, root = document) {
    return Array.from(root.querySelectorAll(selector));
  }

  function setError(element, message) {
    const target = typeof element === "string" ? qs(element) : element;
    if (!target) return;
    target.textContent = message;
    target.hidden = false;
  }

  function clearError(element) {
    const target = typeof element === "string" ? qs(element) : element;
    if (!target) return;
    target.textContent = "";
    target.hidden = true;
  }

  function setBusy(button, busy) {
    if (!button) return;
    button.setAttribute("aria-busy", busy ? "true" : "false");
    button.disabled = Boolean(busy);
  }

  function openDialog(dialog) {
    const target = typeof dialog === "string" ? qs(dialog) : dialog;
    if (!target || typeof target.showModal !== "function") return;
    if (!target.open) target.showModal();
  }

  function closeDialog(dialog) {
    const target = typeof dialog === "string" ? qs(dialog) : dialog;
    if (target && target.open) target.close();
  }

  function toast(message, kind = "info", timeout = 4800) {
    const stack = qs("#toasts");
    if (!stack) return;
    const item = document.createElement("div");
    item.className = "toast" + (kind === "error" ? " is-error" : kind === "success" ? " is-success" : "");
    item.textContent = message;
    stack.appendChild(item);
    window.setTimeout(() => {
      if (reducedMotion) {
        item.remove();
        return;
      }
      item.style.opacity = "0";
      item.style.transform = "translateY(6px)";
      window.setTimeout(() => item.remove(), 220);
    }, timeout);
  }

  async function api(path, options = {}) {
    const method = (options.method || "GET").toUpperCase();
    const headers = new Headers(options.headers || {});
    headers.set("Accept", "application/json");
    headers.set("X-CSRF-Token", csrf);
    let body = options.body;
    if (body && !(body instanceof FormData) && typeof body !== "string") {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }

    let response;
    try {
      response = await fetch(path, {
        method,
        headers,
        body,
        credentials: "same-origin",
        redirect: "follow",
      });
    } catch (error) {
      throw new Error("Tidak dapat terhubung ke server. Periksa koneksi Anda.");
    }

    let payload = null;
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      payload = await response.json().catch(() => null);
    }

    if (response.status === 401 && !path.includes("/login")) {
      window.location.assign("/admin/login");
      throw new Error("Sesi berakhir. Silakan login kembali.");
    }

    if (!response.ok || !payload || payload.ok !== true) {
      const message =
        (payload && payload.error && payload.error.message) ||
        (response.status === 429
          ? "Terlalu banyak permintaan. Tunggu sebentar lalu coba lagi."
          : "Permintaan tidak dapat diproses.");
      const error = new Error(message);
      error.code = payload && payload.error ? payload.error.code : "";
      error.status = response.status;
      throw error;
    }
    return payload.data;
  }

  
  function confirmAction(options = {}) {
    const dialog = qs("#confirm-dialog");
    const okButton = qs("#confirm-ok");
    const title = qs("#confirm-title");
    const message = qs("#confirm-message");
    if (!dialog || !okButton || !title || !message) {
      return Promise.resolve(window.confirm(options.message || "Lanjutkan?"));
    }

    title.textContent = options.title || "Lanjutkan tindakan?";
    message.textContent = options.message || "";
    okButton.textContent = options.confirmLabel || "Ya, lanjutkan";
    okButton.classList.toggle("btn-danger-solid", options.danger === true);
    okButton.classList.toggle("btn-primary", options.danger !== true);

    return new Promise((resolve) => {
      let settled = false;
      const finish = (value) => {
        if (settled) return;
        settled = true;
        okButton.removeEventListener("click", onConfirm);
        dialog.removeEventListener("close", onClose);
        if (dialog.open) dialog.close();
        resolve(value);
      };
      const onConfirm = () => finish(true);
      const onClose = () => finish(false);
      okButton.addEventListener("click", onConfirm);
      dialog.addEventListener("close", onClose);
      dialog.showModal();
    });
  }

  function formatDateTime(value) {
    if (!value) return "-";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "-";
    return dateTimeFormat.format(date);
  }

  function formatNumber(value) {
    return numberFormat.format(Number(value) || 0);
  }

  function formatPercent(value) {
    const number = Number(value);
    if (!Number.isFinite(number)) return "0,0%";
    return number.toLocaleString("id-ID", { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + "%";
  }

  function fallbackCopy(text) {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    let copied = false;
    try {
      copied = document.execCommand("copy");
    } catch (error) {
      copied = false;
    }
    area.remove();
    return copied;
  }

  async function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch (error) {
        return fallbackCopy(text);
      }
    }
    return fallbackCopy(text);
  }

  function saveBlob(filename, blob) {
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = filename;
    link.rel = "noopener";
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1500);
  }

  function downloadFile(filename, content, mime = "text/csv;charset=utf-8") {
    saveBlob(filename, new Blob([content], { type: mime }));
  }

  async function fetchFile(path, options = {}, fallbackMessage = "Berkas tidak dapat diunduh.") {
    const method = (options.method || "GET").toUpperCase();
    const headers = new Headers(options.headers || {});
    headers.set("X-CSRF-Token", csrf);
    headers.set("Accept", "application/pdf, text/csv, */*");
    let body = options.body;
    if (body && !(body instanceof FormData) && typeof body !== "string") {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }

    let response;
    try {
      response = await fetch(path, {
        method,
        headers,
        body,
        credentials: "same-origin",
        redirect: "follow",
      });
    } catch (error) {
      throw new Error("Tidak dapat terhubung ke server. Periksa koneksi Anda.");
    }

    if (response.status === 401) {
      window.location.assign("/admin/login");
      throw new Error("Sesi berakhir. Silakan login kembali.");
    }

    if (!response.ok) {
      let message = fallbackMessage;
      const contentType = response.headers.get("content-type") || "";
      if (contentType.includes("application/json")) {
        const payload = await response.json().catch(() => null);
        if (payload && payload.error && payload.error.message) message = payload.error.message;
      } else if (response.status === 429) {
        message = "Terlalu banyak permintaan. Tunggu sebentar lalu coba lagi.";
      }
      throw new Error(message);
    }
    return response.blob();
  }

  async function downloadFromServer(path, filename, options = {}, fallbackMessage = "Berkas tidak dapat diunduh.") {
    const blob = await fetchFile(path, options, fallbackMessage);
    saveBlob(filename, blob);
    return blob;
  }

  function makeElement(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = String(text);
    return node;
  }

  function setStat(name, value) {
    qsa('[data-stat="' + name + '"]').forEach((node) => {
      node.textContent = typeof value === "number" ? formatNumber(value) : value;
    });
  }

  function resultRow(candidate) {
    const item = makeElement("li", "result-item");
    item.dataset.candidateId = String(candidate.candidate_id);

    item.appendChild(makeElement("span", "result-rank", Number(candidate.rank) > 0 ? "#" + candidate.rank : "-"));
    const main = makeElement("div", "result-main");
    const line = makeElement("div", "result-line");
    line.appendChild(makeElement("span", "result-number", String(candidate.candidate_number).padStart(2, "0")));
    const names = makeElement("span", "result-names");
    names.appendChild(makeElement("strong", "", candidate.chairman_name));
    names.appendChild(makeElement("span", "", candidate.vice_chairman_name));
    line.appendChild(names);
    main.appendChild(line);

    const bar = makeElement("div", "result-bar");
    const progress = document.createElement("progress");
    progress.max = 100;
    progress.value = Number(candidate.percentage) || 0;
    progress.setAttribute("aria-label", "Persentase suara " + candidate.chairman_name);
    progress.textContent = formatPercent(candidate.percentage);
    bar.appendChild(progress);
    main.appendChild(bar);

    const metaRow = makeElement("div", "result-meta");
    const votes = makeElement("span", "", formatNumber(candidate.votes) + " suara");
    votes.dataset.votes = "";
    const percentage = makeElement("span", "", formatPercent(candidate.percentage));
    percentage.dataset.pct = "";
    metaRow.appendChild(votes);
    metaRow.appendChild(percentage);
    main.appendChild(metaRow);
    item.appendChild(main);
    return item;
  }

  function renderResults(payload) {
    if (!payload) return;
    const stats = payload.token_stats || {};
    setStat("votes", formatNumber(payload.total_votes || 0));
    setStat("tokens-used", formatNumber(stats.used || 0));
    setStat("tokens-total", formatNumber(stats.total || 0));
    setStat("tokens-available", formatNumber(stats.available || 0));
    setStat("tokens-revoked", formatNumber(stats.revoked || 0));
    setStat("tokens-expired", formatNumber(stats.expired || 0));
    setStat("participation", formatPercent(payload.participation || 0));

    const list = qs("[data-live-results] .result-list");
    if (list) {
      const fragment = document.createDocumentFragment();
      (payload.candidates || []).forEach((candidate) => fragment.appendChild(resultRow(candidate)));
      list.replaceChildren(fragment);
      list.hidden = (payload.candidates || []).length === 0;
    }

    const candidates = payload.candidates || [];
    const winner = Number(payload.total_votes || 0) > 0 ? candidates[0] : null;
    const winnerNumber = qs("[data-winner-number]");
    if (winnerNumber) {
      winnerNumber.classList.toggle("is-empty", !winner);
      winnerNumber.textContent = winner ? String(winner.candidate_number).padStart(2, "0") : "-";
      qs("[data-winner-chairman]").textContent = winner ? winner.chairman_name : (candidates.length ? "Belum ada suara" : "Belum ada calon");
      qs("[data-winner-vice]").textContent = winner ? winner.vice_chairman_name : (candidates.length ? "Menunggu suara pertama" : "-");
      qs("[data-winner-votes]").textContent = winner ? formatNumber(winner.votes) : "0";
      qs("[data-winner-pct]").textContent = winner ? formatPercent(winner.percentage) : "0,0%";
    }

    qsa("[data-live-updated]").forEach((node) => {
      node.textContent = formatDateTime(payload.updated_at);
    });
  }

  function setLiveState(state, text) {
    qsa("[data-live-indicator]").forEach((node) => {
      node.dataset.state = state;
    });
    qsa("[data-live-text]").forEach((node) => {
      node.textContent = text;
    });
  }

  function connectLiveResults(electionId) {
    if (!electionId || !("WebSocket" in window)) {
      setLiveState("offline", "Live tidak tersedia");
      return;
    }

    let socket = null;
    let attempt = 0;
    let stopped = false;
    let retryTimer = 0;

    const endpoint = () => {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      return protocol + "//" + window.location.host + "/ws/admin/results?election_id=" + encodeURIComponent(electionId);
    };

    const scheduleRetry = () => {
      if (stopped) return;
      attempt += 1;
      const delay = Math.min(1000 * Math.pow(1.7, attempt), 20000);
      setLiveState("offline", "Menyambung ulang...");
      window.clearTimeout(retryTimer);
      retryTimer = window.setTimeout(connect, delay);
    };

    const connect = () => {
      if (stopped) return;
      setLiveState("connecting", "Menghubungkan live...");
      try {
        socket = new WebSocket(endpoint());
      } catch (error) {
        scheduleRetry();
        return;
      }

      socket.addEventListener("open", () => {
        attempt = 0;
        setLiveState("live", "Live tersambung");
      });

      socket.addEventListener("message", (event) => {
        let message;
        try {
          message = JSON.parse(event.data);
        } catch (error) {
          return;
        }
        if (message && (message.event === "snapshot" || message.event === "vote_update")) {
          renderResults(message.data);
        }
      });

      socket.addEventListener("close", () => {
        if (stopped) return;
        socket = null;
        scheduleRetry();
      });

      socket.addEventListener("error", () => {
        if (socket) socket.close();
      });
    };

    connect();

    window.addEventListener("pagehide", () => {
      stopped = true;
      window.clearTimeout(retryTimer);
      if (socket) socket.close();
    });

    document.addEventListener("visibilitychange", () => {
      if (!document.hidden && !socket) {
        window.clearTimeout(retryTimer);
        connect();
      }
    });
  }

  function initVisibilityControls() {
    qsa("[data-visibility-control]").forEach((control) => {
      const toggle = qs("[data-visibility-toggle]", control);
      if (!toggle) return;
      toggle.addEventListener("click", async () => {
        const electionId = Number(control.dataset.electionId || 0);
        if (!electionId) return;
        const next = toggle.getAttribute("aria-checked") === "true" ? "FINAL" : "LIVE";
        const pageWide = control.classList.contains("visibility-control-page");
        setBusy(toggle, true);
        try {
          await api("/api/v1/admin/elections/" + electionId + "/visibility", {
            method: "POST",
            body: { results_visibility: next },
          });
          toggle.setAttribute("aria-checked", next === "LIVE" ? "true" : "false");
          control.dataset.visibility = next;
          const label = qs("[data-visibility-label]", control);
          if (label) {
            label.textContent =
              next === "LIVE"
                ? pageWide
                  ? "Aktif - hasil tampil di beranda siswa dan diperbarui otomatis"
                  : "Aktif - hasil tampil di beranda siswa"
                : pageWide
                  ? "Nonaktif - hasil disembunyikan dari beranda sampai pemilihan ditutup"
                  : "Nonaktif - hasil disembunyikan dari beranda";
          }
          const scope = control.closest(".election-card") || control;
          qsa("[data-visibility-badge]", scope).forEach((badge) => {
            badge.textContent = next === "LIVE" ? "Hasil publik" : "Hasil privat";
          });
          qsa("[data-visibility-meta]", scope).forEach((meta) => {
            meta.textContent = next === "LIVE" ? "Publik (live)" : "Privat sampai ditutup";
          });
          toast(
            next === "LIVE"
              ? "Hasil publik diaktifkan. Beranda siswa menampilkan hasil secara realtime."
              : "Hasil publik dinonaktifkan. Beranda siswa menyembunyikan hasil sampai pemilihan ditutup.",
            "success"
          );
        } catch (error) {
          toast(error.message, "error");
        } finally {
          setBusy(toggle, false);
        }
      });
    });
  }

  function initShell() {
    const body = document.body;
    const navToggle = qs("[data-nav-toggle]");
    const scrim = qs("[data-nav-close]");
    const sidebar = qs("#admin-sidebar");

    const closeNav = () => {
      body.classList.remove("nav-open");
      if (navToggle) navToggle.setAttribute("aria-expanded", "false");
    };

    if (navToggle) {
      navToggle.addEventListener("click", () => {
        const open = !body.classList.contains("nav-open");
        body.classList.toggle("nav-open", open);
        navToggle.setAttribute("aria-expanded", open ? "true" : "false");
        if (open && sidebar) sidebar.querySelector("a")?.focus({ preventScroll: true });
      });
    }
    if (scrim) scrim.addEventListener("click", closeNav);
    qsa(".admin-nav a").forEach((link) => link.addEventListener("click", closeNav));
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && body.classList.contains("nav-open")) closeNav();
    });

    const logout = qs("[data-logout]");
    if (logout) {
      logout.addEventListener("click", async () => {
        setBusy(logout, true);
        try {
          const result = await api("/api/v1/admin/logout", { method: "POST" });
          window.location.assign(result.redirect || "/admin/login");
        } catch (error) {
          toast(error.message, "error");
          setBusy(logout, false);
        }
      });
    }

    const electionSelect = qs("[data-election-select]");
    const electionForm = qs("[data-election-form]");
    if (electionSelect && electionForm) {
      electionSelect.addEventListener("change", () => {
        if (typeof electionForm.requestSubmit === "function") {
          electionForm.requestSubmit();
        } else {
          electionForm.submit();
        }
      });
    }

    qsa("dialog").forEach((dialog) => {
      dialog.addEventListener("click", (event) => {
        if (event.target !== dialog) return;
        if (dialog.id === "generated-dialog") return;
        closeDialog(dialog);
      });
    });

    initVisibilityControls();
  }

  window.GV = {
    api,
    clearError,
    closeDialog,
    confirmAction,
    connectLiveResults,
    copyText,
    csrf,
    downloadFile,
    downloadFromServer,
    formatDateTime,
    formatNumber,
    formatPercent,
    makeElement,
    openDialog,
    qs,
    qsa,
    renderResults,
    setBusy,
    setError,
    setLiveState,
    setStat,
    toast,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initShell, { once: true });
  } else {
    initShell();
  }
})();