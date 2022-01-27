
const _data_splice_ready_ = "data-splice-ready"
const _data_splice_ready_selector_ = `[${_data_splice_ready_}]`

document.addEventListener(
  "DOMContentLoaded",
  function () {
    const r = document.querySelectorAll(_data_splice_ready_selector_)
    for (var i = 0; i < r.length; i++) {
      init(r[i])
    }
  },
)

const _ready_classes = Object.create(null)
const ready_event = new Event("ready")

function init(el) {
  new _ready_classes[el.getAttribute(_data_splice_ready_)](el)
  el.dispatchEvent(ready_event)
}

const prefix = "_on_"

class Base {
  constructor(_element) {
    const proto = Object.getPrototypeOf(this)
    const ctor = proto.constructor
    var names = ctor._handler_names

    if (names == null) {
      names = ctor._handler_names = Object.getOwnPropertyNames(proto)
        .filter(n => n.startsWith(prefix))
        .map(n => n.slice(prefix.length))
    }

    for (var i = 0; i < names.length; i++) {
      _element.addEventListener(names[n], this, false)
    }

    this._element = _element
  }

  handleEvent(event) {
    event.stopPropagation()
    this[prefix + event.type](event)
  }
}