;(function () {
  "use strict"

  document.addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-action=hide-tag]")
    if (!btn) return

    const tagId = btn.dataset.tagId
    const hidden = btn.dataset.hidden.trim() === "true"
    const url = `/tags/${tagId}${hidden ? "/unhide" : "/hide"}`

    const res = await fetch(url, { method: "POST" })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    const data = await res.json()
    if (!data?.ok) return
    btn.dataset.hidden = hidden ? "false" : "true"
    btn.textContent = hidden ? "hide" : "unhide"
  })
})()
