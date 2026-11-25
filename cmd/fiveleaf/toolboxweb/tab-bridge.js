(() => {
  if (window.parent === window || window.mygo || (location.protocol !== 'fiveleaf-toolbox:' && !location.hostname.endsWith('.fiveleaf-toolbox.localhost'))) return
  const channel = new MessageChannel()
  const pending = new Map()
  const listeners = new Set()
  let next = 0
  let switching = false

  channel.port1.onmessage = ({ data }) => {
    if (data.event === 'toolbox-theme') {
      for (const listener of listeners) listener(data.value)
      return
    }
    const result = pending.get(data.id)
    if (!result) return
    pending.delete(data.id)
    if (data.error !== undefined) result.reject(new Error(data.error))
    else result.resolve(data.value)
  }
  function call(method, ...args) {
    const id = ++next
    const response = new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
    channel.port1.postMessage({ id, method, args })
    if (method === 'Toolbox.OpenTools') return response.then(value => {
      switching = true
      setTimeout(() => { switching = false }, 0)
      return value
    })
    return response
  }
  const close = () => switching ? Promise.resolve() : call('Toolbox.Close')
  const on = (event, listener) => {
    if (event === 'toolbox-theme') listeners.add(listener)
    return () => listeners.delete(listener)
  }
  Object.defineProperty(window, 'mygo', { value: Object.freeze({ call, on, window: Object.freeze({ close }) }) })
  window.parent.postMessage('fiveleaf-toolbox-connect', '*', [channel.port2])

  window.addEventListener('focus', () => call('Toolbox.Focus').catch(() => {}))
  window.addEventListener('keydown', event => {
    if (event.defaultPrevented || event.isComposing || !(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey) return
    const action = { k: 'Toolbox.OpenTools', w: 'Toolbox.Close' }[event.key.toLowerCase()]
    if (!action) return
    event.preventDefault()
    event.stopImmediatePropagation()
    call(action).catch(console.error)
  }, true)
})()
