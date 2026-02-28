;(function () {
  "use strict"

  // Close all flag dropdowns
  function closeAllDropdowns() {
    document.querySelectorAll("[data-role=flag-menu]").forEach(function (p) {
      p.hidden = true
      p.classList.remove(
        "flag-dropdown__menu--above",
        "flag-dropdown__menu--right",
      )
    })
  }

  // Position dropdown: flip vertically/horizontally if it overflows the viewport
  function positionDropdown(menu) {
    menu.classList.remove(
      "flag-dropdown__menu--above",
      "flag-dropdown__menu--right",
    )
    menu.hidden = false
    var rect = menu.getBoundingClientRect()
    if (rect.bottom > window.innerHeight) {
      menu.classList.add("flag-dropdown__menu--above")
    }
    if (rect.right > window.innerWidth) {
      menu.classList.add("flag-dropdown__menu--right")
    }
  }

  // Open story flag dropdown
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action=story-flag]")
    if (!btn) return
    e.stopPropagation()
    var menu = btn.nextElementSibling
    if (!menu) return
    var wasHidden = menu.hidden
    closeAllDropdowns()
    if (wasHidden) {
      positionDropdown(menu)
    }
  })

  // Open comment flag dropdown
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action=comment-flag]")
    if (!btn) return
    e.stopPropagation()
    var dropdown = btn.closest("[data-role=flag-dropdown]")
    if (!dropdown) return
    var menu = dropdown.querySelector("[data-role=flag-menu]")
    if (!menu) return
    var wasHidden = menu.hidden
    closeAllDropdowns()
    if (wasHidden) {
      positionDropdown(menu)
    }
  })

  // Close dropdowns on outside click
  document.addEventListener("click", function (e) {
    if (!e.target.closest("[data-role=flag-dropdown]")) {
      closeAllDropdowns()
    }
  })

  // Story flag reason selection
  document.addEventListener("click", async function (e) {
    var option = e.target.closest("[data-action=flag-option]")
    if (!option) return
    var dropdown = option.closest("[data-role=flag-dropdown]")
    if (!dropdown) return

    // Determine if this is a story or comment flag
    var storyBtn = dropdown.querySelector("[data-action=story-flag]")
    var commentBtn = dropdown.querySelector("[data-action=comment-flag]")

    if (storyBtn) {
      var storyId = storyBtn.dataset.storyId
      var reason = option.dataset.reason
      var res = await fetch("/stories/" + storyId + "/flag", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: reason }),
      })
      if (res.status === 401) {
        window.location.href = "/login"
        return
      }
      var data = await res.json()
      if (data && data.ok) {
        // Replace the dropdown with an unflag button
        var parent = dropdown.parentNode
        var unflagBtn = document.createElement("button")
        unflagBtn.className = "story-item__action story-unflag-btn"
        unflagBtn.setAttribute("data-action", "story-unflag")
        unflagBtn.dataset.storyId = storyId
        unflagBtn.textContent = "unflag"
        parent.replaceChild(unflagBtn, dropdown)
      }
      closeAllDropdowns()
      return
    }

    if (commentBtn) {
      var commentId = commentBtn.dataset.commentId
      var reason = option.dataset.reason
      var res = await fetch("/comments/" + commentId + "/flag", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: reason }),
      })
      if (res.status === 401) {
        window.location.href = "/login"
        return
      }
      var data = await res.json()
      if (data && data.ok) {
        // Update score
        var score = document.querySelector(
          '[data-role=vote-score][data-comment-id="' + commentId + '"]',
        )
        if (score) score.textContent = data.score
        // Replace the dropdown with an unflag button
        var parent = dropdown.parentNode
        var unflagBtn = document.createElement("button")
        unflagBtn.className = "comment__action comment-unflag-btn"
        unflagBtn.setAttribute("data-action", "comment-unflag")
        unflagBtn.dataset.commentId = commentId
        unflagBtn.textContent = "unflag"
        parent.replaceChild(unflagBtn, dropdown)
      }
      closeAllDropdowns()
      return
    }
  })

  // Story unflag
  document.addEventListener("click", async function (e) {
    var btn = e.target.closest("[data-action=story-unflag]")
    if (!btn) return

    var storyId = btn.dataset.storyId
    var res = await fetch("/stories/" + storyId + "/unflag", {
      method: "POST",
    })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    var data = await res.json()
    if (data && data.ok) {
      // Reload to restore dropdown (simpler than rebuilding it)
      window.location.reload()
    }
  })

  // Comment unflag
  document.addEventListener("click", async function (e) {
    var btn = e.target.closest("[data-action=comment-unflag]")
    if (!btn) return

    var commentId = btn.dataset.commentId
    var res = await fetch("/comments/" + commentId + "/unflag", {
      method: "POST",
    })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    var data = await res.json()
    if (data && data.ok) {
      var score = document.querySelector(
        '[data-role=vote-score][data-comment-id="' + commentId + '"]',
      )
      if (score) score.textContent = data.score
      // Reload to restore dropdown
      window.location.reload()
    }
  })

  // Story hide
  document.addEventListener("click", async function (e) {
    var btn = e.target.closest("[data-action=story-hide]")
    if (!btn) return

    var storyId = btn.dataset.storyId
    var res = await fetch("/stories/" + storyId + "/hide", {
      method: "POST",
    })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    var data = await res.json()
    if (data && data.ok) {
      // On listing pages, remove the <li> from the list; on story page, reload
      var listItem = btn.closest("[data-role=story-item]")
      if (listItem) {
        listItem.remove()
      } else {
        window.location.reload()
      }
    }
  })

  // Story unhide
  document.addEventListener("click", async function (e) {
    var btn = e.target.closest("[data-action=story-unhide]")
    if (!btn) return

    var storyId = btn.dataset.storyId
    var res = await fetch("/stories/" + storyId + "/unhide", {
      method: "POST",
    })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    var data = await res.json()
    if (data && data.ok) {
      window.location.reload()
    }
  })
})()
