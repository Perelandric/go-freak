
freak.Base = class Base {
  static withParent(parent, element) {

    // TODO: Need to have subclass to set up handlers. How?

    var b = new freak.Base(element)
    b._parent = parent
    var lc = parent.lastChild
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
    for (var i = 0; i < children.length; i++) {
      var ch = children[i]

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

  // TODO: this needs to do more to insert itself into this sub-dom
  constructor(_element) {
    this._element = _element
    this._children = []
    this._parent = null
    this._previousSibling = null
    this._nextSibling = null
    
    var thisCtor = this.constructor

    thisCtor._event_types
      .forEach(function(n) { _element.addEventListener(n, this) }, this)

    this._handlers = thisCtor._handlers
  }

  handleEvent(event) { this._handlers[event.type](event) }

  remove() {
    if (this._parent) {
      const ch = this._parent._children
      const idx = ch.indexOf(this)
      if (idx !== -1) {
        ch.splice(idx, 1)
      }
      this._parent = null
    }

    var prev = this._previousSibling
    var next = this._nextSibling
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
  findFirst(fns) {
    return _findFirst(this, Array.slice.call(argumets))

    function _findFirst(el, fns) {
      for (var i = 0; i < this._children.length; i++) {
        var ch = this._children[i]
        if (fns.every(function(fn) { return fn(ch) })) {
          return ch
        }

        const res = _findFirst(ch, fns)
        if (res != null) {
          return res
        }
      }
      return null
    }
  }

  _findLimit(limit, res, fns) {
    for (var i = 0; i < this._children.length; i++) {
      var ch = this._children[i]
      if (fns.every(function(fn) { return fn(ch) }) && limit === res.push(ch)) {
        break
      }

      ch._findLimit(limit, res, fns)
      if (limit === res.length) {
        break
      }
    }
    return res
  }

  findLimit(limit, fns) {
    var _limit = Math.max(0, ~~limit)
    return _limit === 0 ? [] : 
      this._findLimit(limit, [], Array.slice.call(arguments, 1))
  }

  findAll(fns) {
    return this._findLimit(Infinity, [], array.slice.call(arguments))
  }

  _insert_before(toInsert, before) {
    var parent = this

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
      var ch = parent._children
      var targIdx = ch.indexOf(before) + 1
      parent._children = ch.slice(0, targIdx).concat(toInsert, ch.slice(targIdx))

      var after = before._nextSibling

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


freak._getCtor = function (idName) {

  var ctor = freak.ctors.get(idName)
  if (ctor != null) {
    return ctor
  }

  var parts = idName.split("-")
  var id = parts[0]
  var name = parts[1]

  var loader = freak.loaders.get(id)
  if (loader == null) {
    console.log("expected JS loader for:", id)
    return null
  }

  var ctors = loader(freak)
  if (ctors == null) {
    console.log("expected JS constructors for:", id)
    return null
  }

  freak.loaders.delete(id)

  for (var n in ctors) {
    var c = ctors[n]
    if (n === name) {
      ctor = c
    }

    freak._prepare_ctor(c)
    freak.ctors.set(id + "-" + n, c)
  }

  if (ctor == null) {
    console.log("expected JS constructor for:", id, name)
  }

  return ctor
}

freak._prepare_ctor = function(ctor) {

  var prefix = "_on_"

  // Store list of event listener types on the ctor
  ctor._event_types = 
    (function getTypes(obj, types) {
      if (!obj) 
        return types

      types = types.concat(
        Object.getOwnPropertyNames(obj)
          .filter(function(n) { return n.startsWith(prefix) })
          .map(function(n) { return n.slice(prefix.length) })
      )
      
      return getTypes(Object.getPrototypeOf(obj), types)
    }(ctor.prototype, []));

  // Store event handlers on separate object
  ctor._handlers = Object.create(null)
  ctor._event_types.forEach(function(name) {
    ctor._handlers[name] = ctor.prototype[prefix + name]
  })
}


document.addEventListener("DOMContentLoaded", function(e) {
  var els = document.querySelectorAll("[data-freak-js]")
  for (var i = 0; i < els.length; i++) {
    var el = els[i]

    const ctor = freak._getCtor(el.dataset.freakJs)

    if (ctor != null) {
      new ctor(el)
    }
  }
}, { once: true });