;(function () {
  "use strict"

  const picker = document.querySelector("[data-role=tag-picker]")
  if (!picker) return

  const control = picker.querySelector("[data-role=tag-picker-control]")
  const chipsContainer = picker.querySelector("[data-role=tag-picker-chips]")
  const searchInput = picker.querySelector("[data-role=tag-picker-search]")
  const dropdown = picker.querySelector("[data-role=tag-picker-dropdown]")
  const options = Array.from(
    dropdown.querySelectorAll("[data-role=tag-picker-option]"),
  )
  const groups = Array.from(
    dropdown.querySelectorAll("[data-role=tag-picker-group]"),
  )

  const selected = new Set()
  let focusedIdx = -1

  Array.from(picker.querySelectorAll("input[name='tags']")).forEach((input) => {
    selected.add(input.value)
  })

  function open() {
    picker.setAttribute("aria-expanded", "true")
  }

  function close() {
    picker.setAttribute("aria-expanded", "false")
    setFocused(-1)
  }

  function isOpen() {
    return picker.getAttribute("aria-expanded") === "true"
  }

  function toggle(optEl) {
    const id = optEl.dataset.id
    if (selected.has(id)) {
      deselect(id)
    } else {
      selectTag(id)
    }
  }

  function selectTag(id) {
    selected.add(id)
    const optEl = dropdown.querySelector(
      `[data-role=tag-picker-option][data-id="${id}"]`,
    )
    if (!optEl) return
    optEl.setAttribute("aria-selected", "true")

    // Add hidden input
    const input = document.createElement("input")
    input.type = "hidden"
    input.name = "tags"
    input.value = id
    input.setAttribute("data-tag-input", id)
    picker.appendChild(input)

    // Add chip
    const chip = document.createElement("span")
    chip.className = `tag-picker__chip${optEl.dataset.isMedia === "true" ? " tag-picker__chip--media" : ""}`
    chip.setAttribute("data-role", "tag-picker-chip")
    chip.dataset.id = id
    chip.append(optEl.dataset.tag)
    const removeBtn = document.createElement("button")
    removeBtn.type = "button"
    removeBtn.className = "tag-picker__chip-remove"
    removeBtn.setAttribute("data-action", "tag-picker-chip-remove")
    removeBtn.setAttribute("aria-label", `Remove ${optEl.dataset.tag}`)
    removeBtn.textContent = "\u00d7"
    removeBtn.addEventListener("click", (e) => {
      e.stopPropagation()
      deselect(id)
      searchInput.focus()
    })
    chip.append(removeBtn)
    chipsContainer.appendChild(chip)
  }

  function deselect(id) {
    selected.delete(id)
    const optEl = dropdown.querySelector(
      `[data-role=tag-picker-option][data-id="${id}"]`,
    )
    if (optEl) optEl.setAttribute("aria-selected", "false")

    // Remove hidden input
    const input = picker.querySelector(`input[data-tag-input="${id}"]`)
    if (input) input.remove()

    // Remove chip
    const chip = chipsContainer.querySelector(
      `[data-role=tag-picker-chip][data-id="${id}"]`,
    )
    if (chip) chip.remove()
  }

  function getVisibleOptions() {
    return options.filter((o) => !o.hidden)
  }

  function setFocused(idx) {
    const visible = getVisibleOptions()
    if (focusedIdx >= 0 && focusedIdx < visible.length) {
      visible[focusedIdx].classList.remove("tag-picker__option--focused")
    }
    focusedIdx = idx
    if (idx >= 0 && idx < visible.length) {
      visible[idx].classList.add("tag-picker__option--focused")
      visible[idx].scrollIntoView({ block: "nearest" })
      searchInput.setAttribute(
        "aria-activedescendant",
        `tag-opt-${visible[idx].dataset.id}`,
      )
    } else {
      searchInput.removeAttribute("aria-activedescendant")
    }
  }

  function filterOptions() {
    const q = searchInput.value.toLowerCase().trim()
    options.forEach((opt) => {
      if (!q) {
        opt.hidden = false
        return
      }
      opt.hidden = !opt.dataset.tag.toLowerCase().includes(q)
    })

    // Hide empty groups
    groups.forEach((g) => {
      const hasVisible = Array.from(
        g.querySelectorAll("[data-role=tag-picker-option]"),
      ).some((o) => !o.hidden)
      g.hidden = !hasVisible
    })

    // Auto-focus exact match or sole remaining option
    const visible = getVisibleOptions()
    if (q && visible.length === 1) {
      setFocused(0)
    } else if (q) {
      const exactIdx = visible.findIndex(
        (o) => o.dataset.tag.toLowerCase() === q,
      )
      setFocused(exactIdx)
    } else {
      setFocused(-1)
    }
  }

  // Wire up pre-rendered chip remove buttons
  Array.from(
    chipsContainer.querySelectorAll("[data-action=tag-picker-chip-remove]"),
  ).forEach((btn) => {
    const chip = btn.closest("[data-role=tag-picker-chip]")
    const id = chip.dataset.id
    btn.addEventListener("click", (e) => {
      e.stopPropagation()
      deselect(id)
      searchInput.focus()
    })
  })

  // Prevent dropdown from catching tab focus
  dropdown.tabIndex = -1

  // Set option IDs for ARIA
  options.forEach((opt) => {
    opt.id = `tag-opt-${opt.dataset.id}`
  })

  // Events

  control.addEventListener("click", () => {
    searchInput.focus()
    if (!isOpen()) open()
  })

  searchInput.addEventListener("focus", () => {
    if (!isOpen()) open()
  })

  searchInput.addEventListener("input", () => {
    filterOptions()
    if (!isOpen()) open()
  })

  searchInput.addEventListener("keydown", (e) => {
    const visible = getVisibleOptions()
    if (e.key === "ArrowDown") {
      e.preventDefault()
      if (!isOpen()) {
        open()
        return
      }
      setFocused(Math.min(focusedIdx + 1, visible.length - 1))
    } else if (e.key === "ArrowUp") {
      e.preventDefault()
      if (focusedIdx > 0) setFocused(focusedIdx - 1)
    } else if (e.key === "Enter") {
      e.preventDefault()
      if (focusedIdx >= 0 && focusedIdx < visible.length) {
        toggle(visible[focusedIdx])
      }
      searchInput.value = ""
      filterOptions()
    } else if (e.key === "Escape") {
      close()
      searchInput.blur()
    } else if (e.key === "Backspace" && searchInput.value === "") {
      // Remove last selected chip
      const chips = chipsContainer.querySelectorAll(
        "[data-role=tag-picker-chip]",
      )
      if (chips.length > 0) {
        const lastChip = chips[chips.length - 1]
        deselect(lastChip.dataset.id)
      }
    }
  })

  searchInput.addEventListener("blur", () => {
    close()
  })

  options.forEach((opt) => {
    opt.addEventListener("mousedown", (e) => {
      e.preventDefault()
    })
    opt.addEventListener("click", (e) => {
      e.preventDefault()
      toggle(opt)
      searchInput.focus()
    })
  })

  // Close on outside click
  document.addEventListener("click", (e) => {
    if (!picker.contains(e.target)) {
      close()
    }
  })
})()
