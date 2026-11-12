(() => {
  "use strict";

  const form = document.getElementById("login-form");
  if (!form) return;

  const errorBox = document.getElementById("login-error");
  const submit = document.getElementById("login-submit");
  const meta = document.querySelector('meta[name="csrf-token"]');
  const csrf = (meta && meta.content) || document.body.dataset.csrf || "";

  const toggle = document.querySelector("[data-toggle-password]");
  const password = document.getElementById("login-password");
  const toggleLabel = document.querySelector("[data-password-label]");
  if (toggle && password) {
    toggle.addEventListener("click", () => {
      const showing = password.type === "text";
      password.type = showing ? "password" : "text";
      toggle.setAttribute("aria-pressed", showing ? "false" : "true");
      if (toggleLabel) toggleLabel.textContent = showing ? "Tampilkan" : "Sembunyikan";
      password.focus({ preventScroll: true });
    });
  }

  const showError = (message) => {
    if (!errorBox) return;
    errorBox.textContent = message;
    errorBox.hidden = false;
  };

  const clearError = () => {
    if (!errorBox) return;
    errorBox.textContent = "";
    errorBox.hidden = true;
  };

  const setBusy = (busy) => {
    if (!submit) return;
    submit.setAttribute("aria-busy", busy ? "true" : "false");
    submit.disabled = busy;
  };

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();

    const data = new FormData(form);
    const login = String(data.get("login") || "").trim();
    const secret = String(data.get("password") || "");
    if (!login || !secret) {
      showError("Masukkan username dan password.");
      return;
    }

    setBusy(true);
    try {
      const response = await fetch("/api/v1/admin/login", {
        method: "POST",
        credentials: "same-origin",
        headers: {
          Accept: "application/json",
          "Content-Type": "application/json",
          "X-CSRF-Token": csrf,
        },
        body: JSON.stringify({ login, password: secret }),
      });

      let payload = null;
      try {
        payload = await response.json();
      } catch (error) {
        payload = null;
      }

      if (!response.ok || !payload || payload.ok !== true) {
        if (response.status === 429) {
          showError("Terlalu banyak percobaan login. Tunggu beberapa menit lalu coba lagi.");
        } else {
          showError(
            (payload && payload.error && payload.error.message) ||
              "Username atau password salah."
          );
        }
        setBusy(false);
        return;
      }

      const redirect = (payload.data && payload.data.redirect) || "/admin";
      window.location.assign(redirect);
    } catch (error) {
      showError("Tidak dapat terhubung ke server. Periksa koneksi Anda.");
      setBusy(false);
    }
  });
})();