;(function () {
  "use strict"

  // Comment voting (upvote/unvote)
  document.addEventListener("click", async function (e) {
    var btn = e.target.closest("[data-action=comment-vote]")
    if (!btn) return

    var commentId = btn.dataset.commentId
    var voted = btn.dataset.voted === "true"
    var url = "/comments/" + commentId + (voted ? "/unvote" : "/upvote")

    var res = await fetch(url, { method: "POST" })
    if (res.status === 401) {
      window.location.href = "/login"
      return
    }
    var data = await res.json()
    if (!data || !data.ok) return

    var score = document.querySelector(
      '[data-role=vote-score][data-comment-id="' + commentId + '"]',
    )
    if (score) score.textContent = data.score
    btn.dataset.voted = voted ? "false" : "true"
    btn.classList.toggle("vote-btn--active")
  })

  // Reply button
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action=comment-reply]")
    if (!btn) return

    // Remove any existing reply form
    var existing = document.querySelector("[data-role=reply-form]")
    if (existing) existing.remove()

    var commentId = btn.dataset.commentId
    var storyCode = btn.dataset.storyCode
    var subtree = document.getElementById("comment-" + commentId)
    if (!subtree) return
    // Insert inside the li.comments_subtree, after the .comment div
    var li = subtree.closest(".comments_subtree")
    if (!li) return

    var form = document.createElement("form")
    form.method = "POST"
    form.action = "/x/" + storyCode + "/comments"
    form.className = "comment-reply-form"
    form.setAttribute("data-role", "reply-form")

    var hidden = document.createElement("input")
    hidden.type = "hidden"
    hidden.name = "parent_id"
    hidden.value = commentId
    form.appendChild(hidden)

    var textarea = document.createElement("textarea")
    textarea.name = "body"
    textarea.className = "field-input"
    textarea.rows = 4
    textarea.placeholder = "Write a reply..."
    textarea.required = true
    textarea.maxLength = 10000
    form.appendChild(textarea)

    var actions = document.createElement("div")
    actions.className = "comment-reply-actions"

    var submit = document.createElement("button")
    submit.type = "submit"
    submit.className = "btn"
    submit.textContent = "Reply"
    actions.appendChild(submit)

    var cancel = document.createElement("button")
    cancel.type = "button"
    cancel.className = "btn btn--secondary"
    cancel.textContent = "Cancel"
    cancel.addEventListener("click", function () {
      form.remove()
    })
    actions.appendChild(cancel)

    form.appendChild(actions)
    // Insert after the .comment div but inside the <li>
    subtree.after(form)
    textarea.focus()
  })

  // Edit toggle
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action=comment-edit-toggle]")
    if (!btn) return

    var commentId = btn.dataset.commentId
    var form = document.querySelector(
      '[data-role=comment-edit-form][data-comment-id="' + commentId + '"]',
    )
    if (form) {
      form.hidden = !form.hidden
      if (!form.hidden) {
        form.querySelector("textarea").focus()
      }
    }
  })

  // Edit cancel
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-action=comment-edit-cancel]")
    if (!btn) return

    var commentId = btn.dataset.commentId
    var form = document.querySelector(
      '[data-role=comment-edit-form][data-comment-id="' + commentId + '"]',
    )
    if (form) form.hidden = true
  })

  // Delete confirmation
  document.addEventListener("submit", function (e) {
    var form = e.target.closest("[data-role=comment-delete-form]")
    if (!form) return

    if (!confirm("Delete this comment? This cannot be undone.")) {
      e.preventDefault()
    }
  })
})()
