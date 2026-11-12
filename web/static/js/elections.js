(() => {
  "use strict";

  const GV = window.GV;
  const dialog = document.getElementById("election-dialog");
  const form = document.getElementById("election-form");
  const formTitle = document.getElementById("election-dialog-title");
  const formError = document.getElementById("election-form-error");
  const submit = document.getElementById("election-submit");
  const closeDialog = document.getElementById("close-dialog");
  const closeForm = document.getElementById("close-form");
  const closeError = document.getElementById("close-form-error");
  const closeSubmit = document.getElementById("close-submit");
  let editingId = null;
  let closingId = null;

  function toLocalInput(date) {
    const pad = (number) => String(number).padStart(2, "0");
    return (
      date.getFullYear() +
      "-" +
      pad(date.getMonth() + 1) +
      "-" +
      pad(date.getDate()) +
      "T" +
      pad(date.getHours()) +
      ":" +
      pad(date.getMinutes())
    );
  }

  function setField(id, value) {
    const field = document.getElementById(id);
    if (field) field.value = value || "";
  }

  function resetForm() {
    if (!form) return;
    form.reset();
    editingId = null;
    const now = new Date();
    const start = new Date(now.getTime() + 10 * 60 * 1000);
    const end = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000);
    setField("election-start", toLocalInput(start));
    setField("election-end", toLocalInput(end));
    setField("election-visibility", "LIVE");
    if (formTitle) formTitle.textContent = "Buat periode baru";
    GV.clearError(formError);
  }

  GV.qsa("[data-election-create]").forEach((button) => {
    button.addEventListener("click", () => {
      if (!dialog) return;
      resetForm();
      GV.openDialog(dialog);
      document.getElementById("election-name")?.focus({ preventScroll: true });
    });
  });

  GV.qsa("[data-election-edit]").forEach((button) => {
    button.addEventListener("click", () => {
      const card = button.closest("[data-election-id]");
      if (!card || !dialog) return;
      editingId = card.dataset.electionId || null;
      setField("election-name", card.dataset.name);
      setField("election-period", card.dataset.period);
      setField("election-description", card.dataset.description);
      setField("election-start", card.dataset.start);
      setField("election-end", card.dataset.end);
      setField("election-visibility", card.dataset.visibility || "LIVE");
      if (formTitle) formTitle.textContent = "Ubah periode pemilihan";
      GV.clearError(formError);
      GV.openDialog(dialog);
    });
  });

  if (form) {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      GV.clearError(formError);

      const payload = {
        name: document.getElementById("election-name")?.value.trim() || "",
        period: document.getElementById("election-period")?.value.trim() || "",
        description: document.getElementById("election-description")?.value.trim() || "",
        start_at: document.getElementById("election-start")?.value || "",
        end_at: document.getElementById("election-end")?.value || "",
        results_visibility: document.getElementById("election-visibility")?.value || "LIVE",
      };
      if (!payload.name || !payload.period || !payload.start_at || !payload.end_at) {
        GV.setError(formError, "Lengkapi nama, periode, waktu mulai, dan waktu selesai.");
        return;
      }
      if (new Date(payload.end_at) <= new Date(payload.start_at)) {
        GV.setError(formError, "Waktu selesai harus setelah waktu mulai.");
        return;
      }

      const editing = Boolean(editingId);
      GV.setBusy(submit, true);
      try {
        await GV.api("/api/v1/admin/elections" + (editing ? "/" + encodeURIComponent(editingId) : ""), {
          method: editing ? "PUT" : "POST",
          body: payload,
        });
        GV.toast(editing ? "Periode pemilihan diperbarui." : "Periode pemilihan dibuat.", "success");
        window.setTimeout(() => window.location.reload(), 420);
      } catch (error) {
        GV.setError(formError, error.message);
        GV.setBusy(submit, false);
      }
    });
  }

  const actionCopy = {
    schedule: {
      title: "Jadwalkan pemilihan?",
      message: "Pemilihan akan mengikuti waktu mulai dan selesai pada detail periode. Token dapat diterbitkan setelah periode terjadwal.",
      label: "Ya, jadwalkan",
    },
    start: {
      title: "Mulai pemilihan sekarang?",
      message: "Bilik suara langsung dibuka dan pasangan calon terkunci. Pastikan token sudah dibagikan kepada pemilih.",
      label: "Ya, mulai sekarang",
    },
    pause: {
      title: "Jeda pemilihan?",
      message: "Token baru tidak dapat divalidasi dan suara tidak dapat dikirim sampai pemilihan dilanjutkan. Pemilih yang sedang di bilik suara akan kehilangan sesinya.",
      label: "Ya, jeda",
    },
    archive: {
      title: "Arsipkan pemilihan?",
      message: "Periode yang sudah ditutup akan dipindahkan ke arsip. Data suara dan audit tetap tersimpan.",
      label: "Ya, arsipkan",
    },
  };

  GV.qsa("[data-election-action]").forEach((button) => {
    button.addEventListener("click", async () => {
      const card = button.closest("[data-election-id]");
      if (!card) return;
      const action = button.dataset.electionAction;
      const copy = actionCopy[action];
      if (!copy) return;
      const confirmed = await GV.confirmAction({
        title: copy.title,
        message: copy.message,
        confirmLabel: copy.label,
        danger: action === "pause" || action === "archive",
      });
      if (!confirmed) return;

      button.setAttribute("aria-busy", "true");
      button.disabled = true;
      try {
        await GV.api(
          "/api/v1/admin/elections/" + encodeURIComponent(card.dataset.electionId) + "/" + encodeURIComponent(action),
          { method: "POST" }
        );
        GV.toast("Status pemilihan diperbarui.", "success");
        window.setTimeout(() => window.location.reload(), 420);
      } catch (error) {
        GV.toast(error.message, "error");
        button.removeAttribute("aria-busy");
        button.disabled = false;
      }
    });
  });

  GV.qsa("[data-election-close]").forEach((button) => {
    button.addEventListener("click", () => {
      const card = button.closest("[data-election-id]");
      if (!card || !closeDialog) return;
      closingId = card.dataset.electionId;
      if (closeForm) closeForm.reset();
      GV.clearError(closeError);
      GV.openDialog(closeDialog);
      document.getElementById("close-password")?.focus({ preventScroll: true });
    });
  });

  if (closeForm) {
    closeForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      GV.clearError(closeError);
      const password = document.getElementById("close-password")?.value || "";
      if (!password) {
        GV.setError(closeError, "Masukkan password administrator.");
        return;
      }
      if (!closingId) return;

      GV.setBusy(closeSubmit, true);
      try {
        await GV.api("/api/v1/admin/elections/" + encodeURIComponent(closingId) + "/close", {
          method: "POST",
          body: { password },
        });
        GV.toast("Pemilihan ditutup. Hasil akhir dapat diunduh.", "success");
        window.setTimeout(() => window.location.reload(), 420);
      } catch (error) {
        GV.setError(closeError, error.message);
        GV.setBusy(closeSubmit, false);
      }
    });
  }
})();