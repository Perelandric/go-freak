
; freak.ctors = new Map

freak.getCtor = function (idName) {

  var ctor = freak.ctors.get(idName)
  if (ctor != null) {
    return ctor
  }

  const [id, name] = idName.split("-")

  const obj = freak.loaders.get(id)
  if (obj == null) {
    console.log("expected JS loader for:", id)
    return null
  }

  ctor = obj[name]
  if (ctor == null) {
    console.log("expected JS constructor for:", idName)
    return null
  }

  freak.loaders.delete(id)
  for (const [n, c] of Object.entries(obj)) {
    freak.ctors.set(id + "-" + n, c)
  }

  return ctor
}

document.addEventListener("DOMContentLoaded", e => {
  for (const el of document.querySelectorAll("[data-freak-js]")) {
    const ctor = freak.getCtor(el.dataset.freakJs)

    if (ctor == null) {
      return
    }

    new ctor(freak)
  }
}, { once: true });