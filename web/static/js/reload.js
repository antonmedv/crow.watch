;(function () {
  function morph(from, to) {
    if (from.nodeType !== to.nodeType) {
      from.replaceWith(to)
      return
    }
    if (
      from.nodeType === Node.TEXT_NODE ||
      from.nodeType === Node.COMMENT_NODE
    ) {
      if (from.nodeValue !== to.nodeValue) from.nodeValue = to.nodeValue
      return
    }
    if (from.nodeType === Node.ELEMENT_NODE) {
      if (from.tagName !== to.tagName) {
        from.replaceWith(to)
        return
      }
      syncAttributes(from, to)
      syncChildren(from, to)
      return
    }
    from.replaceWith(to)
  }

  function syncAttributes(from, to) {
    for (const { name } of Array.from(from.attributes)) {
      if (!to.hasAttribute(name)) from.removeAttribute(name)
    }
    for (const { name, value } of Array.from(to.attributes)) {
      if (from.getAttribute(name) !== value) from.setAttribute(name, value)
    }
  }

  function syncChildren(from, to) {
    const fromKids = Array.from(from.childNodes)
    const toKids = Array.from(to.childNodes)
    const commonLen = Math.min(fromKids.length, toKids.length)
    for (let i = 0; i < commonLen; i++) {
      morph(fromKids[i], toKids[i])
    }
    for (let i = fromKids.length - 1; i >= toKids.length; i--) {
      from.removeChild(fromKids[i])
    }
    for (let i = commonLen; i < toKids.length; i++) {
      from.appendChild(toKids[i])
    }
  }

  let delay = 100
  function poll(isUp = false) {
    fetch(`/__dev/reload${isUp ? "?is_up" : ""}`)
      .then(function (res) {
        if (res.status === 204) {
          delay = 100
          poll()
          return
        }
        return res.json()
      })
      .then(function (data) {
        if (!data) return
        delay = 100
        if (data.kind === "css") {
          document
            .querySelectorAll('link[rel="stylesheet"]')
            .forEach(function (link) {
              if (link.href.indexOf("/static/") === -1) return
              const url = link.href.replace(/[?&]_dev=\d+/, "")
              link.href =
                url + (url.indexOf("?") > -1 ? "&" : "?") + "_dev=" + Date.now()
            })
          poll()
        } else if (data.kind === "tmpl") {
          fetch(location.href)
            .then(function (r) {
              return r.text()
            })
            .then(function (html) {
              const doc = new DOMParser().parseFromString(html, "text/html")
              const newMain = doc.querySelector("html")
              if (newMain) {
                morph(document.querySelector("html"), newMain)
              }
              poll()
            })
            .catch(function () {
              poll()
            })
        } else {
          location.reload()
        }
      })
      .catch(function () {
        delay = Math.min(delay * 2, 3000)
        setTimeout(() => poll(true), delay)
      })
  }
  poll()
})()
