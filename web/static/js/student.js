(() => {
  "use strict";

  const body = document.body;
  const meta = document.querySelector('meta[name="csrf-token"]');
  const csrf = (meta && meta.content) || body.dataset.csrf || "";
  const page = body.dataset.page || "";

  function qs(selector, root = document) {
    return root.querySelector(selector);
  }

  function qsa(selector, root = document) {
    return Array.from(root.querySelectorAll(selector));
  }

  function showError(element, message) {
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
    button.disabled = busy;
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

  async function postJSON(path, payload) {
    let response;
    try {
      response = await fetch(path, {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
        body: JSON.stringify(payload),
      });
    } catch (error) {
      throw new Error("Tidak dapat terhubung ke server. Periksa koneksi Anda lalu coba lagi.");
    }

    let data = null;
    try {
      data = await response.json();
    } catch (error) {
      data = null;
    }
    if (!response.ok || !data || data.ok !== true) {
      const err = new Error(
        (data && data.error && data.error.message) ||
          (response.status === 429
            ? "Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi."
            : "Permintaan tidak dapat diproses.")
      );
      err.code = data && data.error ? data.error.code : "";
      err.status = response.status;
      throw err;
    }
    return data.data;
  }

  function redirectFor(error) {
    if (!error) return false;
    if (error.code === "SESSION_INVALID") {
      window.location.assign("/token?status=expired");
      return true;
    }
    if (error.code === "TOKEN_INVALID") {
      window.location.assign("/token");
      return true;
    }
    if (error.code === "ELECTION_CLOSED") {
      window.location.assign("/token?status=closed");
      return true;
    }
    return false;
  }

  function initTokenForm() {
    const form = qs("#token-form");
    if (!form) return;
    const input = qs("#token-input");
    const submit = qs("#token-submit");
    const errorBox = qs("#token-error");

    if (input) {
      input.addEventListener("input", () => {
        const clean = input.value.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 5);
        if (input.value !== clean) input.value = clean;
        clearError(errorBox);
      });
      input.addEventListener("paste", (event) => {
        const text = (event.clipboardData || window.clipboardData).getData("text");
        if (!text) return;
        event.preventDefault();
        input.value = text.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 5);
        clearError(errorBox);
      });
    }

    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      clearError(errorBox);
      const token = String(input ? input.value : "").trim().toUpperCase();
      if (!/^[A-Z0-9]{5}$/.test(token)) {
        showError(errorBox, "Token terdiri dari tepat 5 huruf atau angka.");
        if (input) input.focus();
        return;
      }

      setBusy(submit, true);
      try {
        const data = await postJSON("/api/v1/voting/validate-token", { token });
        window.location.assign((data && data.redirect) || "/vote/ballot");
      } catch (error) {
        showError(errorBox, error.message);
        setBusy(submit, false);
        if (input) {
          input.focus();
          input.select();
        }
      }
    });
  }

  function initHomeResults() {
    const panel = qs("[data-public-results]");
    if (!panel || panel.dataset.live !== "true") return;
    const electionId = panel.dataset.electionId;
    const list = qs("[data-public-list]", panel);
    const numberFormat = new Intl.NumberFormat("id-ID");
    const percentFormat = new Intl.NumberFormat("id-ID", {
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    });

    function setLiveState(state, text) {
      qsa("[data-public-live]").forEach((node) => {
        node.dataset.state = state;
      });
      qsa("[data-public-live-text]").forEach((node) => {
        node.textContent = text;
      });
    }

    function render(payload) {
      if (!payload || !Array.isArray(payload.candidates)) return;
      qsa("[data-public-total]").forEach((node) => {
        node.textContent = numberFormat.format(payload.total_votes || 0);
      });

      const ordered = [];
      payload.candidates.forEach((candidate) => {
        if (!list) return;
        const item = list.querySelector('[data-candidate-id="' + candidate.candidate_id + '"]');
        if (!item) return;
        const rank = item.querySelector("[data-public-rank]");
        const votes = item.querySelector("[data-public-votes]");
        const percent = item.querySelector("[data-public-pct]");
        const bar = item.querySelector("progress");
        const formattedPercent = percentFormat.format(candidate.percentage || 0) + "%";
        if (rank) rank.textContent = "#" + candidate.rank;
        if (votes) votes.textContent = numberFormat.format(candidate.votes || 0) + " suara";
        if (percent) percent.textContent = formattedPercent;
        if (bar) {
          bar.value = candidate.percentage || 0;
          bar.textContent = formattedPercent;
        }
        item.dataset.votes = String(candidate.votes || 0);
        ordered.push(item);
      });
      ordered.forEach((item) => list.appendChild(item));

      const leader = payload.candidates[0];
      if (leader) {
        const fields = {
          "[data-public-leader-number]": String(leader.candidate_number).padStart(2, "0"),
          "[data-public-leader-chairman]": leader.chairman_name,
          "[data-public-leader-vice]": leader.vice_chairman_name,
          "[data-public-leader-votes]": numberFormat.format(leader.votes || 0),
          "[data-public-leader-pct]": percentFormat.format(leader.percentage || 0) + "%",
        };
        Object.keys(fields).forEach((selector) => {
          const node = qs(selector);
          if (node) node.textContent = fields[selector];
        });
      }
    }

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
      return protocol + "//" + window.location.host + "/ws/public/results";
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
          render(message.data);
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
  }

  function initBallot() {
    if (page !== "ballot") return;

    const voteDialog = qs("#vote-dialog");
    const cancelDialog = qs("#cancel-dialog");
    let selectedCandidate = null;

    qsa("[data-choose]").forEach((button) => {
      button.addEventListener("click", () => {
        selectedCandidate = button.dataset.candidateId || null;
        if (!selectedCandidate) return;
        const number = qs("#confirm-number");
        const chairman = qs("#confirm-chairman");
        const vice = qs("#confirm-vice");
        if (number) number.textContent = button.dataset.number || "-";
        if (chairman) chairman.textContent = button.dataset.chairman || "-";
        if (vice) vice.textContent = button.dataset.vice || "-";
        clearError("#vote-error");
        openDialog(voteDialog);
      });
    });

    qsa("[data-close-dialog]").forEach((button) => {
      button.addEventListener("click", () => {
        const dialog = button.closest("dialog");
        closeDialog(dialog);
      });
    });

    [voteDialog, cancelDialog].forEach((dialog) => {
      if (!dialog) return;
      dialog.addEventListener("click", (event) => {
        if (event.target === dialog) closeDialog(dialog);
      });
    });

    const confirmVote = qs("#confirm-vote");
    if (confirmVote) {
      confirmVote.addEventListener("click", async () => {
        if (!selectedCandidate) {
          showError("#vote-error", "Pilih pasangan calon terlebih dahulu.");
          return;
        }
        setBusy(confirmVote, true);
        try {
          const data = await postJSON("/api/v1/voting/submit", {
            candidate_id: Number(selectedCandidate),
          });
          window.location.assign((data && data.redirect) || "/?status=success");
        } catch (error) {
          if (redirectFor(error)) return;
          showError("#vote-error", error.message);
          setBusy(confirmVote, false);
        }
      });
    }

    const cancelSession = qs("#cancel-session");
    if (cancelSession) {
      cancelSession.addEventListener("click", () => openDialog(cancelDialog));
    }

    const confirmCancel = qs("#confirm-cancel");
    if (confirmCancel) {
      confirmCancel.addEventListener("click", async () => {
        clearError("#cancel-error");
        setBusy(confirmCancel, true);
        try {
          const data = await postJSON("/api/v1/voting/cancel", {});
          window.location.assign((data && data.redirect) || "/token");
        } catch (error) {
          if (redirectFor(error)) return;
          showError("#cancel-error", error.message);
          setBusy(confirmCancel, false);
        }
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener(
      "DOMContentLoaded",
      () => {
        initTokenForm();
        initHomeResults();
        initBallot();
      },
      { once: true }
    );
  } else {
    initTokenForm();
    initHomeResults();
    initBallot();
  }
})();