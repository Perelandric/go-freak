
; freak.ctors = new Map

freak._getCtor = function (idName) {

  var ctor = freak.ctors.get(idName)
  if (ctor != null) {
    return ctor
  }

  const [id, name] = idName.split("-")

  const loader = freak.loaders.get(id)
  if (loader == null) {
    console.log("expected JS loader for:", id)
    return null
  }

  const ctors = loader(freak)
  if (ctors == null) {
    console.log("expected one or more JS constructors for:", id)
    return null
  }

  freak.loaders.delete(id)

  for (const [n, c] of Object.entries(ctors)) {
    if (n === name) {
      ctor = c
    }

    freak.ctors.set(id + "-" + n, c)
  }

  if (ctor == null) {
    console.log("expected JS constructor for:", id, name)
  }

  return ctor
}

document.addEventListener("DOMContentLoaded", e => {
  for (const el of document.querySelectorAll("[data-freak-js]")) {
    const ctor = freak._getCtor(el.dataset.freakJs)

    if (ctor != null) {
      new ctor(el)
    }
  }
}, { once: true });



freak.Base = class Base {

  static withParent(parent, element) {

    // TODO: Need to have subclass to set up handlers. How?

    const b = new freak.Base(element)
    b._parent = parent
    const lc = parent.lastChild
    if (lc != null) {
      b._previousSibling = lc
      lc._nextSibling = b
    }
    parent._children.push(b)
    return b
  }

  static fromRoot(root) {
    return freak.Base._setChildren(new freak.Base(root), root.children)
  }

  static _setChildren(parent, children) {
    for (const ch of children) {
      if (!(ch instanceof HTMLElement)) continue

      if ("subDom" in ch.dataset) {
        freak.Base._setChildren(freak.Base.withParent(parent, ch), ch.children)
      } else {
        freak.Base._setChildren(parent, ch.children)
      }
    }
    return parent
  }

  static _get_element(base) {
    return base._element
  }

  static get _prefix() {return "_on_"}

  // TODO: this needs to do more to insert itself into this sub-dom
  constructor(_element) {
    this._element = _element
    this._children = []
    this._parent = null
    this._previousSibling = null
    this._nextSibling = null

    if (this.constructor._event_types == null) {

      // Store list of event listener types on the ctor
      this.constructor._event_types = 
        (function getTypes(obj, types) {
          if (!obj) return types

          types = types.concat(
            Object.getOwnPropertyNames(obj)
              .filter(n => n.startsWith(freak.Base._prefix))
              .map(n => n.slice(freak.Base._prefix.length))
          )
          
          return getTypes(Object.getPrototypeOf(obj), types)
        }(this, []));
    }

    this.constructor._event_types
      .forEach(n => _element.addEventListener(n, this))
  }

  handleEvent(event) {
    this[freak.Base._prefix + event.type](event)
  }

  remove() {
    if (this._parent) {
      const ch = this._parent._children
      const idx = ch.indexOf(this)
      if (idx !== -1) {
        ch.splice(idx, 1)
      }
      this._parent = null
    }

    const prev = this._previousSibling
    const next = this._nextSibling
    if (next != null) {
      next._previousSibling = prev
      this._nextSibling = null
    }
    if (prev != null) {
      prev._nextSibling = next
      this._previousSibling = null
    }

    this._element.remove()

    return this
  }

  // We need to implement a full sub-dom, so that there is no using the
  // native DOM API. This means all modifications, relocations, querying,
  // and so on gets done with this API.
  findFirst(...fns) {
    for (const ch of this._children) {
      if (fns.every(fn => fn(ch))) {
        return ch
      }

      const res = ch.findFirst(...fns)
      if (res != null) {
        return res
      }
    }
    return null
  }

  _findLimit(limit, res, ...fns) {
    for (const ch of this._children) {
      if (fns.every(fn => fn(ch)) && limit === res.push(ch)) {
        break
      }

      ch._findLimit(limit, res, ...fns)
      if (limit === res.length) {
        break
      }
    }
    return res
  }

  findLimit(limit, ...fns) {
    const _limit = Math.max(0, ~~limit)
    return _limit === 0 ? [] : this._findLimit(_limit, [], ...fns)
  }

  findAll(...fns) {
    return this._findLimit(Infinity, [], ...fns)
  }

  _insert_before(toInsert, before) {
    const parent = this

    toInsert.remove()
    parent._element.insertBefore(
      toInsert._element,
      before ? before._element || null : null,
    )

    if (before == null) {
      toInsert._previousSibling = parent.lastChild
      toInsert._nextSibling = null
      parent._children = parent._children.concat(toInsert)

    } else {
      const ch = parent._children
      const targIdx = ch.indexOf(before) + 1
      parent._children = ch.slice(0, targIdx).concat(toInsert, ch.slice(targIdx))

      const after = before._nextSibling

      toInsert._previousSibling = before
      toInsert._nextSibling = after

      if (after != null) {
        after._previousSibling = toInsert
      }
      before._nextSibling = toInsert
    }

    toInsert._parent = parent

    return toInsert
  }

  // TODO: These should not depend on a _parent being present
  placeBefore(target) {
    return target._parent && 
            target._parent._insert_before(this, target) || 
            null
  }
  placeAfter(target) {
    return target._parent && 
            target._parent._insert_before(this, target._nextSibling) ||
            null
  }

  prependTo(parent) {
    return parent._insert_before(this, parent.firstChild)
  }
  appendTo(parent) {
    return parent._insert_before(this, null)
  }

  get previousSibling() {
    return this._previousSibling
  }
  get nextSibling() {
    return this._nextSibling
  }
  get parent() {
    return this._parent
  }
  get firstChild() {
    return this._children.length !== 0 ? this._children[0] : null
  }
  get lastChild() {
    const len = this._children.length
    return len !== 0 ? this._children[len - 1] : null
  }
  get id() {
    return this._element.id
  }
  get dataset() {
    return this._element.dataset
  }
  get class() {
    return this._element.classList
  }
}

freak.FormElementBase = class FormElementBase extends freak.Base {
  constructor(_element) {
    super(_element)
  }

  get value() {
    return this._element.value
  }
}
