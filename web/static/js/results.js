(() => {
  "use strict";

  const electionId = document.body.dataset.electionId;
  if (!electionId) return;

  const start = () => {
    window.GV.qsa("[data-export-results]").forEach((button) => {
      button.addEventListener("click", async () => {
        const format = button.dataset.exportResults;
        if (format !== "csv" && format !== "pdf") return;
        const suffix = new Date().toISOString().slice(0, 10).replace(/-/g, "");
        window.GV.setBusy(button, true);
        try {
          await window.GV.downloadFromServer(
            "/api/v1/admin/results/export?election_id=" + encodeURIComponent(electionId) + "&format=" + format,
            "hasil-pemilihan-" + electionId + "-" + suffix + "." + format,
            {},
            "Gagal mengunduh hasil pemilihan."
          );
          window.GV.toast("Berkas hasil pemilihan diunduh.", "success");
        } catch (error) {
          window.GV.toast(error.message, "error");
        } finally {
          window.GV.setBusy(button, false);
        }
      });
    });
    window.GV.connectLiveResults(electionId);
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start, { once: true });
  } else {
    start();
  }
})();
