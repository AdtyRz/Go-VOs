(() => {
  "use strict";

  const GV = window.GV;
  const form = document.getElementById("school-form");
  const errorBox = document.getElementById("school-form-error");
  const save = document.getElementById("school-save");
  const saveTop = document.getElementById("school-save-top");
  const logoError = document.getElementById("logo-form-error");
  const logoInput = document.getElementById("school-logo-file");
  const logoUpload = document.querySelector("[data-logo-upload]");
  const logoRemove = document.querySelector("[data-logo-remove]");
  const logoPreview = document.getElementById("school-logo-preview");
  let logoPreviewURL = "";

  function paintColour(inputId) {
    const input = document.getElementById(inputId);
    const output = document.querySelector('[data-color-output="' + inputId + '"]');
    if (!input) return;
    const value = input.value || "#000000";
    if (output) output.textContent = value;
    try {
      document.documentElement.style.setProperty(
        inputId === "school-primary" ? "--brand-primary" : "--brand-secondary",
        value
      );
    } catch (error) {
      
    }
  }

  GV.qsa('input[type="color"]').forEach((input) => {
    paintColour(input.id);
    input.addEventListener("input", () => paintColour(input.id));
  });

  if (form) {
    form.addEventListener("submit", async (event) => {
      event.preventDefault();
      GV.clearError(errorBox);

      const themeInput = document.querySelector('input[name="theme"]:checked');
      const payload = {
        name: document.getElementById("school-name")?.value.trim() || "",
        tagline: document.getElementById("school-tagline")?.value.trim() || "",
        address: document.getElementById("school-address")?.value.trim() || "",
        email: document.getElementById("school-email")?.value.trim() || "",
        phone: document.getElementById("school-phone")?.value.trim() || "",
        website: document.getElementById("school-website")?.value.trim() || "",
        footer_text: document.getElementById("school-footer")?.value.trim() || "",
        theme: themeInput ? themeInput.value : "",
        primary_color: document.getElementById("school-primary")?.value || "",
        secondary_color: document.getElementById("school-secondary")?.value || "",
      };
      if (!payload.name) {
        GV.setError(errorBox, "Nama sekolah wajib diisi.");
        return;
      }
      if (!payload.theme) {
        GV.setError(errorBox, "Pilih salah satu tema visual.");
        return;
      }

      GV.setBusy(save, true);
      GV.setBusy(saveTop, true);
      try {
        await GV.api("/api/v1/admin/school", { method: "PUT", body: payload });
        GV.toast("Identitas sekolah disimpan. Memuat ulang tampilan...", "success");
        window.setTimeout(() => window.location.reload(), 520);
      } catch (error) {
        GV.setError(errorBox, error.message);
        GV.setBusy(save, false);
        GV.setBusy(saveTop, false);
      }
    });
  }

  if (logoInput) {
    logoInput.addEventListener("change", () => {
      GV.clearError(logoError);
      const file = logoInput.files && logoInput.files[0];
      if (!file) {
        if (logoUpload) logoUpload.disabled = true;
        return;
      }
      if (file.size > 3 * 1024 * 1024) {
        GV.setError(logoError, "Ukuran logo maksimal 3 MB.");
        logoInput.value = "";
        if (logoUpload) logoUpload.disabled = true;
        return;
      }
      if (!/^image\/(jpeg|png|webp)$/.test(file.type)) {
        GV.setError(logoError, "Logo harus berupa JPG, PNG, atau WebP.");
        logoInput.value = "";
        if (logoUpload) logoUpload.disabled = true;
        return;
      }
      if (logoPreviewURL) URL.revokeObjectURL(logoPreviewURL);
      logoPreviewURL = URL.createObjectURL(file);
      if (logoPreview) {
        let image = logoPreview.querySelector("img");
        if (!image) {
          image = document.createElement("img");
          image.alt = "Pratinjau logo sekolah";
          image.width = 120;
          image.height = 120;
          logoPreview.replaceChildren(image);
        }
        image.src = logoPreviewURL;
      }
      if (logoUpload) logoUpload.disabled = false;
    });
  }

  if (logoUpload) {
    logoUpload.addEventListener("click", async () => {
      if (!logoInput || !logoInput.files || !logoInput.files[0]) {
        GV.setError(logoError, "Pilih file logo terlebih dahulu.");
        return;
      }
      GV.clearError(logoError);
      GV.setBusy(logoUpload, true);
      try {
        const data = new FormData();
        data.append("logo", logoInput.files[0]);
        await GV.api("/api/v1/admin/school/logo", { method: "POST", body: data });
        GV.toast("Logo sekolah diperbarui. Memuat ulang tampilan...", "success");
        window.setTimeout(() => window.location.reload(), 520);
      } catch (error) {
        GV.setError(logoError, error.message);
        GV.setBusy(logoUpload, false);
      }
    });
  }

  if (logoRemove) {
    logoRemove.addEventListener("click", async () => {
      const confirmed = await GV.confirmAction({
        title: "Hapus logo sekolah?",
        message: "Logo akan dihapus dari halaman siswa, panel panitia, dan laporan PDF.",
        confirmLabel: "Ya, hapus logo",
        danger: true,
      });
      if (!confirmed) return;
      GV.setBusy(logoRemove, true);
      try {
        await GV.api("/api/v1/admin/school/logo", { method: "DELETE" });
        GV.toast("Logo sekolah dihapus.", "success");
        window.setTimeout(() => window.location.reload(), 420);
      } catch (error) {
        GV.setError(logoError, error.message);
        GV.setBusy(logoRemove, false);
      }
    });
  }
})();