
; freak.ctors = new Map

freak.getCtor = function (id) {
  var ctor = freak.ctors.get(id)
  if (ctor != null) {
    return ctor
  }

  ctor = freak.loaders.get(id)
  if (ctor == null) {
    console.log("expected JS loader for:", id)
  } else {
    freak.loaders.delete(id)
    freak.ctors.set(id, ctor)
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