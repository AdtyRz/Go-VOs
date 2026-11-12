(() => {
  "use strict";

  const dialog = document.getElementById("candidate-dialog");
  const form = document.getElementById("candidate-form");
  if (!dialog || !form) return;

  const GV = window.GV;
  const errorBox = document.getElementById("candidate-form-error");
  const submit = document.getElementById("candidate-submit");
  const title = document.getElementById("candidate-dialog-title");
  const photoInput = document.getElementById("candidate-photo");
  const photoPreview = document.getElementById("candidate-photo-preview");
  const photoImg = document.getElementById("candidate-photo-img");
  const photoPlaceholder = document.getElementById("candidate-photo-placeholder");
  const removePhoto = document.getElementById("candidate-remove-photo");
  const electionId = document.body.dataset.electionId;
  let editingId = null;
  let previewURL = "";

  function clearPreview() {
    if (previewURL) {
      URL.revokeObjectURL(previewURL);
      previewURL = "";
    }
    if (photoImg) {
      photoImg.hidden = true;
      photoImg.removeAttribute("src");
    }
    if (photoPlaceholder) photoPlaceholder.hidden = false;
  }

  function showPreview(src) {
    if (!photoImg || !photoPlaceholder || !src) return;
    photoImg.src = src;
    photoImg.hidden = false;
    photoPlaceholder.hidden = true;
  }

  function resetForm() {
    form.reset();
    editingId = null;
    if (removePhoto) removePhoto.value = "false";
    if (photoInput) photoInput.value = "";
    clearPreview();
    GV.clearError(errorBox);
    const active = document.getElementById("candidate-active");
    if (active) active.checked = true;
    if (dialog) dialog.dataset.mode = "create";
    if (title) title.textContent = "Tambah pasangan calon";
  }

  function fillForm(card) {
    editingId = card.dataset.candidateId || null;
    if (!editingId) return;
    const number = document.getElementById("candidate-number");
    const chairman = document.getElementById("candidate-chairman");
    const vice = document.getElementById("candidate-vice");
    const vision = document.getElementById("candidate-vision");
    const mission = document.getElementById("candidate-mission");
    const active = document.getElementById("candidate-active");
    if (number) number.value = card.dataset.number || "";
    if (chairman) chairman.value = card.dataset.chairman || "";
    if (vice) vice.value = card.dataset.vice || "";
    if (vision) vision.value = card.dataset.vision || "";
    if (mission) mission.value = card.dataset.mission || "";
    if (active) active.checked = card.dataset.active === "true";
    if (removePhoto) removePhoto.value = "false";
    if (photoInput) photoInput.value = "";
    clearPreview();
    if (card.dataset.photo) showPreview(card.dataset.photo);
    GV.clearError(errorBox);
    if (title) title.textContent = "Ubah pasangan calon";
    if (dialog) dialog.dataset.mode = "edit";
  }

  GV.qsa("[data-candidate-create]").forEach((button) => {
    button.addEventListener("click", () => {
      resetForm();
      GV.openDialog(dialog);
      document.getElementById("candidate-number")?.focus({ preventScroll: true });
    });
  });

  GV.qsa("[data-candidate-edit]").forEach((button) => {
    button.addEventListener("click", () => {
      const card = button.closest("[data-candidate-id]");
      if (!card) return;
      resetForm();
      fillForm(card);
      GV.openDialog(dialog);
    });
  });

  GV.qsa("[data-candidate-delete]").forEach((button) => {
    button.addEventListener("click", async () => {
      const card = button.closest("[data-candidate-id]");
      if (!card) return;
      const id = card.dataset.candidateId;
      const chairman = card.dataset.chairman || "pasangan calon ini";
      const confirmed = await GV.confirmAction({
        title: "Hapus pasangan calon?",
        message:
          "Profil " +
          chairman +
          " akan dihapus permanen. Tindakan ini tidak dapat dibatalkan dan hanya tersedia sebelum pemilihan berjalan.",
        confirmLabel: "Ya, hapus",
        danger: true,
      });
      if (!confirmed) return;

      button.setAttribute("aria-busy", "true");
      button.disabled = true;
      try {
        await GV.api(
          "/api/v1/admin/candidates/" + encodeURIComponent(id) + "?election_id=" + encodeURIComponent(electionId || ""),
          { method: "DELETE" }
        );
        GV.toast("Pasangan calon dihapus.", "success");
        window.setTimeout(() => window.location.reload(), 380);
      } catch (error) {
        GV.toast(error.message, "error");
        button.removeAttribute("aria-busy");
        button.disabled = false;
      }
    });
  });

  if (photoInput) {
    photoInput.addEventListener("change", () => {
      const file = photoInput.files && photoInput.files[0];
      if (!file) return;
      if (file.size > 3 * 1024 * 1024) {
        GV.setError(errorBox, "Ukuran foto maksimal 3 MB.");
        photoInput.value = "";
        return;
      }
      if (!/^image\/(jpeg|png|webp)$/.test(file.type)) {
        GV.setError(errorBox, "Foto harus berupa JPG, PNG, atau WebP.");
        photoInput.value = "";
        return;
      }
      clearPreview();
      previewURL = URL.createObjectURL(file);
      showPreview(previewURL);
      if (removePhoto) removePhoto.value = "false";
      GV.clearError(errorBox);
    });
  }

  const clearPhoto = document.getElementById("candidate-photo-clear");
  if (clearPhoto) {
    clearPhoto.addEventListener("click", () => {
      if (photoInput) photoInput.value = "";
      clearPreview();
      if (removePhoto) removePhoto.value = "true";
      GV.clearError(errorBox);
    });
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    GV.clearError(errorBox);

    const required = ["candidate-number", "candidate-chairman", "candidate-vice", "candidate-vision", "candidate-mission"];
    for (const id of required) {
      const field = document.getElementById(id);
      if (field && !String(field.value).trim()) {
        field.focus();
        GV.setError(errorBox, "Lengkapi seluruh data wajib pasangan calon.");
        return;
      }
    }

    const data = new FormData(form);
    if (!data.get("election_id") && electionId) data.set("election_id", electionId);

    const editing = Boolean(editingId);
    let path = "/api/v1/admin/candidates";
    if (editing) {
      path += "/" + encodeURIComponent(editingId);
    }
    if (electionId) path += "?election_id=" + encodeURIComponent(electionId);

    GV.setBusy(submit, true);
    try {
      await GV.api(path, { method: editing ? "PUT" : "POST", body: data });
      GV.toast(editing ? "Profil pasangan calon diperbarui." : "Pasangan calon ditambahkan.", "success");
      window.setTimeout(() => window.location.reload(), 420);
    } catch (error) {
      GV.setError(errorBox, error.message);
      GV.setBusy(submit, false);
    }
  });
})();