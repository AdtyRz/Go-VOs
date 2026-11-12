(() => {
  "use strict";

  const GV = window.GV;
  const electionId = document.body.dataset.electionId;
  const generateDialog = document.getElementById("token-dialog");
  const generateForm = document.getElementById("token-form");
  const generatedDialog = document.getElementById("generated-dialog");
  const generatedBox = document.getElementById("generated-tokens");
  const generatedCount = document.getElementById("generated-count");
  const generatedExpiry = document.getElementById("generated-expiry");
  const formError = document.getElementById("token-form-error");
  const submit = document.getElementById("token-submit");
  const checkAll = document.querySelector("[data-check-all]");
  const revokeSelected = document.querySelector("[data-revoke-selected]");
  const revokeAll = document.querySelector("[data-revoke-all]");
  const metadataButton = document.querySelector("[data-download-token-meta]");
  const printButton = document.querySelector("[data-print-tokens]");
  let lastTokens = [];
  let lastExpiry = "";

  function tokenChecks() {
    return GV.qsa("[data-token-check]");
  }

  function selectedIDs() {
    return tokenChecks()
      .filter((checkbox) => checkbox.checked && !checkbox.disabled)
      .map((checkbox) => Number(checkbox.value));
  }

  function syncSelection() {
    const count = selectedIDs().length;
    if (revokeSelected) {
      revokeSelected.disabled = count === 0;
      revokeSelected.textContent = count ? "Cabut terpilih (" + count + ")" : "Cabut terpilih";
    }
    if (checkAll) {
      const enabled = tokenChecks().filter((checkbox) => !checkbox.disabled);
      checkAll.checked = enabled.length > 0 && enabled.every((checkbox) => checkbox.checked);
      checkAll.indeterminate = enabled.some((checkbox) => checkbox.checked) && !checkAll.checked;
    }
  }

  function csvEscape(value) {
    const text = String(value == null ? "" : value);
    if (/[",\n\r]/.test(text)) return '"' + text.replace(/"/g, '""') + '"';
    return text;
  }

  function openGenerate() {
    if (!generateDialog || !generateForm) return;
    generateForm.reset();
    const count = document.getElementById("token-count");
    if (count) count.value = "25";
    GV.clearError(formError);
    GV.openDialog(generateDialog);
  }

  GV.qsa("[data-token-generate]").forEach((button) => {
    button.addEventListener("click", openGenerate);
  });

  if (generateForm) {
    generateForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      GV.clearError(formError);

      const count = Number(document.getElementById("token-count")?.value || 0);
      const expires = document.getElementById("token-expires")?.value || "";
      if (!Number.isInteger(count) || count < 1 || count > 1000) {
        GV.setError(formError, "Jumlah token harus antara 1 dan 1000.");
        return;
      }

      GV.setBusy(submit, true);
      try {
        const data = await GV.api("/api/v1/admin/tokens/generate", {
          method: "POST",
          body: {
            election_id: Number(electionId || 0),
            count,
            expires_at: expires,
          },
        });

        lastTokens = Array.isArray(data.tokens) ? data.tokens : [];
        lastExpiry = data.expires_at || "";
        GV.closeDialog(generateDialog);
        if (generatedCount) generatedCount.textContent = String(lastTokens.length);
        if (generatedExpiry) generatedExpiry.textContent = GV.formatDateTime(lastExpiry);
        if (generatedBox) {
          const fragment = document.createDocumentFragment();
          lastTokens.forEach((token) => {
            const chip = document.createElement("span");
            chip.className = "token-chip";
            chip.textContent = token;
            fragment.appendChild(chip);
          });
          generatedBox.replaceChildren(fragment);
        }
        GV.openDialog(generatedDialog);
      } catch (error) {
        GV.setError(formError, error.message);
      } finally {
        GV.setBusy(submit, false);
      }
    });
  }

  if (generatedDialog) {
    generatedDialog.addEventListener("cancel", (event) => {
      if (!lastTokens.length) return;
      event.preventDefault();
      GV.toast("Simpan token terlebih dahulu, lalu tekan Selesai & muat ulang.", "info");
    });
  }

  function fileSuffix() {
    return new Date().toISOString().slice(0, 10);
  }

  if (metadataButton) {
    metadataButton.addEventListener("click", async () => {
      if (!electionId) return;
      GV.setBusy(metadataButton, true);
      try {
        await GV.downloadFromServer(
          "/api/v1/admin/tokens/export?election_id=" + encodeURIComponent(electionId),
          "token-metadata-" + electionId + "-" + fileSuffix() + ".csv",
          {},
          "Gagal mengunduh metadata token."
        );
        GV.toast("Metadata token diunduh. Berkas ini tidak memuat nilai token.", "success");
      } catch (error) {
        GV.toast(error.message, "error");
      } finally {
        GV.setBusy(metadataButton, false);
      }
    });
  }

  const copyButton = document.querySelector("[data-copy-tokens]");
  if (copyButton) {
    copyButton.addEventListener("click", async () => {
      if (!lastTokens.length) return;
      const copied = await GV.copyText(lastTokens.join("\n"));
      GV.toast(copied ? "Seluruh token disalin ke clipboard." : "Clipboard tidak tersedia di peramban ini.", copied ? "success" : "error");
    });
  }

  const downloadButton = document.querySelector("[data-download-tokens]");
  if (downloadButton) {
    downloadButton.addEventListener("click", () => {
      if (!lastTokens.length) return;
      const rows = [["Token", "ID Pemilihan", "Berlaku Hingga"]];
      lastTokens.forEach((token) => {
        rows.push([token, electionId || "", GV.formatDateTime(lastExpiry)]);
      });
      const csv =
        "\uFEFF" +
        rows
          .map((row) => row.map(csvEscape).join(","))
          .join("\r\n");
      GV.downloadFile("token-pemilih-" + (electionId || "pemilihan") + "-" + fileSuffix() + ".csv", csv);
      GV.toast("CSV berisi nilai token diunduh. Simpan dengan aman.", "success");
    });
  }

  if (printButton) {
    printButton.addEventListener("click", async () => {
      if (!lastTokens.length) return;
      GV.setBusy(printButton, true);
      try {
        await GV.downloadFromServer(
          "/api/v1/admin/tokens/print",
          "kartu-token-" + (electionId || "pemilihan") + "-" + fileSuffix() + ".pdf",
          {
            method: "POST",
            body: {
              election_id: Number(electionId || 0),
              tokens: lastTokens,
              expires_at: lastExpiry,
            },
          },
          "Gagal membuat PDF kartu token."
        );
        GV.toast("PDF kartu token diunduh. Periksa jumlah halaman dan kartu sebelum mencetak.", "success");
      } catch (error) {
        GV.toast(error.message, "error");
      } finally {
        GV.setBusy(printButton, false);
      }
    });
  }

  const complete = document.querySelector("[data-complete-generation]");
  if (complete) {
    complete.addEventListener("click", () => {
      lastTokens = [];
      window.location.reload();
    });
  }

  tokenChecks().forEach((checkbox) => checkbox.addEventListener("change", syncSelection));
  if (checkAll) {
    checkAll.addEventListener("change", () => {
      const checked = checkAll.checked;
      tokenChecks().forEach((checkbox) => {
        if (!checkbox.disabled) checkbox.checked = checked;
      });
      syncSelection();
    });
  }
  syncSelection();

  async function revoke(payload, message) {
    const confirmed = await GV.confirmAction({
      title: "Cabut token?",
      message,
      confirmLabel: "Ya, cabut token",
      danger: true,
    });
    if (!confirmed) return;

    try {
      const data = await GV.api("/api/v1/admin/tokens/revoke", {
        method: "POST",
        body: Object.assign({ election_id: Number(electionId || 0) }, payload),
      });
      GV.toast((data.revoked || 0) + " token dicabut.", "success");
      window.setTimeout(() => window.location.reload(), 420);
    } catch (error) {
      GV.toast(error.message, "error");
    }
  }

  if (revokeSelected) {
    revokeSelected.addEventListener("click", () => {
      const ids = selectedIDs();
      if (!ids.length) return;
      revoke({ ids }, "Cabut " + ids.length + " token yang dipilih? Token yang dicabut tidak dapat digunakan lagi.");
    });
  }

  if (revokeAll) {
    revokeAll.addEventListener("click", () => {
      const available = Number(document.querySelector('[data-stat="tokens-available"]')?.textContent || 0);
      revoke(
        { all: true },
        "Cabut seluruh token yang masih tersedia" + (available ? " (" + available + " token)" : "") + "? Nilai token yang sudah dibagikan tidak akan berlaku lagi."
      );
    });
  }
})();