(() => {
  const methods = new Set(['Toolbox.Copy', 'Toolbox.Theme', 'Toolbox.SetDetail', 'Toolbox.ManagePlugins', 'Toolbox.OpenTools', 'Toolbox.Close', 'Toolbox.Focus'])
  let connection

  window.addEventListener('message', event => {
    const frame = document.querySelector('iframe')
    if (!frame || event.source !== frame.contentWindow || event.data !== 'fiveleaf-toolbox-connect' || !event.ports[0]) return
    const source = new URL(frame.src)
    if (event.origin !== source.origin && event.origin !== `${source.protocol}//${source.host}`) return
    connection?.close()
    const port = event.ports[0]
    connection = port
    port.onmessage = async ({ data }) => {
      if (!data || !Number.isSafeInteger(data.id) || !Array.isArray(data.args)) return
      try {
        if (!methods.has(data.method)) throw new Error(`Unknown Toolbox call ${data.method}`)
        const value = await window.mygo.call(data.method, ...data.args)
        port.postMessage({ id: data.id, value })
      } catch (error) {
        port.postMessage({ id: data.id, error: String(error?.message ?? error) })
      }
    }
  })

  window.mygo.on('toolbox-theme', value => connection?.postMessage({ event: 'toolbox-theme', value }))
})()
