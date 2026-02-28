;(function () {
  "use strict"

  document.addEventListener("click", async (e) => {
    const btn = e.target.closest("[data-action=vote]")
    if (!btn || btn.hasAttribute("data-vote-disabled")) return

    const storyId = btn.dataset.storyId
    if (!storyId) return
    const voted = btn.dataset.voted === "true"
    const url = `/stories/${storyId}${voted ? "/unvote" : "/upvote"}`

    const res = await fetch(url, { method: "POST" })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    const data = await res.json()
    if (!data?.ok) return
    const score = document.querySelector(
      `[data-role=vote-score][data-story-id="${storyId}"]`,
    )
    if (score) score.textContent = data.upvotes
    btn.dataset.voted = voted ? "false" : "true"
    btn.classList.toggle("vote-btn--active")
  })
})()
