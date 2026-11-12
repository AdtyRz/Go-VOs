(() => {
  "use strict";

  const electionId = document.body.dataset.electionId;
  if (!electionId) return;

  const start = () => {
    window.GV.connectLiveResults(electionId);
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start, { once: true });
  } else {
    start();
  }
})();